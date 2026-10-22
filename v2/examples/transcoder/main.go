// Command transcoder checks the AMBE+2 FEC of a captured call, rebuilds it, and runs a Codec2 round trip.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"
)

type call struct {
	lc     *pdu.FullLinkControl
	cc     enums.ColorCode
	source layer2.SyncSource
	slots  [][9]byte
	frames []vocoder.AMBEFrame
	voice  int
	embLCs int
}

func readBursts(path string) ([]layer2.RawBurst, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%layer2.RawBurstSize != 0 {
		return nil, fmt.Errorf("%s: %d bytes is not a multiple of %d", path, len(data), layer2.RawBurstSize)
	}
	bursts := make([]layer2.RawBurst, len(data)/layer2.RawBurstSize)
	for i := range bursts {
		copy(bursts[i][:], data[i*layer2.RawBurstSize:])
	}
	return bursts, nil
}

func receive(bursts []layer2.RawBurst) (call, error) {
	var c call
	var rx layer2.VoiceReceiver
	b := layer2.NewBurst()
	for i, raw := range bursts {
		if err := b.DecodeFromBytes(raw); err != nil {
			return c, fmt.Errorf("burst %d: %w", i, err)
		}
		_, ok, err := rx.Receive(b, nil)
		if err != nil {
			return c, fmt.Errorf("burst %d: %w", i, err)
		}
		if ok {
			c.embLCs++
		}
		switch b.Kind {
		case layer2.KindData:
			if lc, isLC := b.Data.(*pdu.FullLinkControl); isLC && b.SlotType.DataType == enums.DataTypeVoiceLCHeader {
				l := *lc
				c.lc, c.cc = &l, b.SlotType.ColorCode
			}
		case layer2.KindVoice:
			if s, _, found := layer2.SyncSourceOf(b.SyncPattern); found {
				c.source = s
			}
			c.voice++
			frames, _ := b.AMBEFrames()
			c.frames = append(c.frames, frames[:]...)
			c.slots = append(c.slots, b.VoiceSlots[:]...)
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if c.lc == nil {
		return c, errors.New("no Voice LC Header")
	}
	return c, nil
}

func transmit(lc pdu.FullLinkControl, cc enums.ColorCode, src layer2.SyncSource, silence [9]byte, add func(*layer2.VoiceTransmitter) ([]layer2.RawBurst, error)) ([]layer2.RawBurst, error) {
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: cc, Source: src, Silence: silence})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	bursts, err := add(tx)
	if err != nil {
		return nil, err
	}
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(append(out, bursts...), tail...), nil
}

func ambe(w io.Writer, dataDir string) (*pdu.FullLinkControl, error) {
	bursts, err := readBursts(filepath.Join(dataDir, "voice.bin"))
	if err != nil {
		return nil, err
	}
	c, err := receive(bursts)
	if err != nil {
		return nil, err
	}
	dst, src, _ := c.lc.Addresses()
	_, _ = fmt.Fprintf(w, "capture: %d bursts, %d voice bursts, %d AMBE frames, %d embedded LCs, %d -> %d\n",
		len(bursts), c.voice, len(c.frames), c.embLCs, src, dst)

	corrected, bad, clean := 0, 0, 0
	for _, slot := range c.slots {
		info, res := vocoder.DecodeAMBE72(slot)
		corrected += res.ErrorsCorrected
		if res.Uncorrectable {
			bad++
		}
		if s, err := vocoder.EncodeAMBE72(info); err == nil && s == slot {
			clean++
		}
	}
	_, _ = fmt.Fprintf(w, "AMBE72 FEC: %d bits corrected, %d uncorrectable frames, %d of %d frames re-encode unchanged\n",
		corrected, bad, clean, len(c.slots))

	slot, err := vocoder.EncodeAMBE72(c.frames[0])
	if err != nil {
		return nil, err
	}
	for _, k := range []int{0, 30, 60} {
		slot[k/8] ^= 0x80 >> (k % 8)
	}
	info, res := vocoder.DecodeAMBE72(slot)
	_, _ = fmt.Fprintf(w, "AMBE72 with 3 flipped bits: %d corrected, frame restored %t\n", res.ErrorsCorrected, info == c.frames[0])

	rebuilt, err := transmit(*c.lc, c.cc, c.source, vocoder.AMBESilenceSlot(),
		func(tx *layer2.VoiceTransmitter) ([]layer2.RawBurst, error) { return tx.AddFrames(c.frames...) })
	if err != nil {
		return nil, err
	}
	back, err := receive(rebuilt)
	if err != nil {
		return nil, err
	}
	same := len(back.frames) >= len(c.frames)
	for i := 0; same && i < len(c.frames); i++ {
		same = back.frames[i] == c.frames[i]
	}
	_, _ = fmt.Fprintf(w, "rebuilt: %d bursts, %d embedded LCs, AMBE frames equal %t\n", len(rebuilt), back.embLCs, same)
	return c.lc, nil
}

func sweep(n int) []int16 {
	pcm := make([]int16, n)
	ph := 0.0
	for i := range pcm {
		pcm[i] = int16(9000 * math.Sin(ph))
		ph += 2 * math.Pi * (300 + 600*float64(i)/float64(n)) / 8000
	}
	return pcm
}

func rms(pcm []int16) float64 {
	sum := 0.0
	for _, s := range pcm {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(max(len(pcm), 1)))
}

func codec2Call(w io.Writer, captured *pdu.FullLinkControl) error {
	enc, err := codec2.New(nil)
	if errors.Is(err, codec2.ErrUnavailable) {
		_, _ = fmt.Fprintln(w, "codec2 unavailable, skipping")
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = enc.Close() }()

	pcm := sweep(50 * enc.FrameSamples())
	slots := make([][9]byte, 0, 50)
	for off := 0; off < len(pcm); off += enc.FrameSamples() {
		s, err := enc.Encode(pcm[off : off+enc.FrameSamples()])
		if err != nil {
			return err
		}
		slots = append(slots, s)
	}

	dst, src, _ := captured.Addresses()
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, FID: enc.FID()}
	if err := lc.SetAddresses(dst, src); err != nil {
		return fmt.Errorf("cannot set addresses %d -> %d: %w", src, dst, err)
	}
	bursts, err := transmit(lc, 1, layer2.SyncSourceMS, enc.Silence(),
		func(tx *layer2.VoiceTransmitter) ([]layer2.RawBurst, error) { return tx.AddSlots(slots...) })
	if err != nil {
		return err
	}

	c, err := receive(bursts)
	if err != nil {
		return err
	}
	same := len(c.slots) >= len(slots)
	for i := 0; same && i < len(slots); i++ {
		same = c.slots[i] == slots[i]
	}
	rdst, rsrc, _ := c.lc.Addresses()
	_, _ = fmt.Fprintf(w, "codec2: %d frames -> %d bursts, rx FID %#x, %d -> %d, %d embedded LCs, slots equal %t\n",
		len(slots), len(bursts), uint8(c.lc.FID), rsrc, rdst, c.embLCs, same)

	dec, err := codec2.New(nil)
	if err != nil {
		return err
	}
	defer func() { _ = dec.Close() }()
	var out []int16
	for _, s := range c.slots {
		if out, err = dec.Decode(out, s); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(w, "codec2 decode: %d samples, audible %t\n", len(out), rms(out) > 500)
	return nil
}

func run(w io.Writer, dataDir string) error {
	lc, err := ambe(w, dataDir)
	if err != nil {
		return err
	}
	return codec2Call(w, lc)
}

func main() {
	def := "testdata"
	if _, file, _, ok := runtime.Caller(0); ok {
		def = filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
	}
	dataDir := flag.String("data", def, "directory with the burst captures")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
