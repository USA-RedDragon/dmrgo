// Command scanner finds bursts in a noisy BS outbound bit stream and follows the calls on both time slots.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	colorCode = 1
	lateEntry = 3
	junkBits  = 37
	berPPM    = 3000
)

func main() {
	def := ""
	if _, file, _, ok := runtime.Caller(0); ok {
		def = filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
	}
	dataDir := flag.String("data", def, "directory with the burst captures")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "scanner:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, dataDir string) error {
	ts1, err := loadBursts(filepath.Join(dataDir, "voice.bin"))
	if err != nil {
		return err
	}
	ts2, err := generatedCall()
	if err != nil {
		return err
	}
	stream, n, flipped, err := buildStream(ts1[lateEntry:], ts2)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "stream: %d bits, %d junk bits, %d bits flipped\n", n, junkBits, flipped)

	r := xorshift(2)
	f, err := framer.New(&framer.Config{Threshold: -1, CACH: true})
	if err != nil {
		return err
	}
	chans := [2]*channel{{name: "TS1"}, {name: "TS2"}}
	cc := enums.ColorCode(colorCode)
	opts := layer2.DecodeOptions{ColorCode: &cc}
	var b layer2.Burst
	left := n
	n, first := 0, int64(-1)
	for len(stream) > 0 {
		chunk := min(len(stream), 1+int(r.next()%63))
		k := min(8*chunk, left)
		left -= k
		for _, fb := range f.Write(stream[:chunk], k) {
			if first < 0 {
				first = fb.Offset
			}
			n++
			if fb.Channel != 1 && fb.Channel != 2 {
				continue
			}
			if err := b.DecodeFromBytesWith(fb.Data, &opts); err != nil {
				fmt.Fprintf(w, "TS%d burst at bit %d: %v\n", fb.Channel, fb.Offset, err)
				continue
			}
			chans[fb.Channel-1].receive(w, &b)
		}
		stream = stream[chunk:]
	}
	fmt.Fprintf(w, "framer: first burst at bit %d, %d bursts\n", first, n)
	for _, c := range chans {
		c.summary(w)
	}
	return nil
}

func loadBursts(path string) ([]layer2.RawBurst, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%layer2.RawBurstSize != 0 {
		return nil, fmt.Errorf("%s: %d bytes is not a whole number of bursts", path, len(data))
	}
	out := make([]layer2.RawBurst, len(data)/layer2.RawBurstSize)
	for i := range out {
		out[i] = layer2.RawBurst(data[i*layer2.RawBurstSize:])
	}
	return out, nil
}

func generatedCall() ([]layer2.RawBurst, error) {
	rotation, err := pdu.EncodeTalkerAlias("KF5XYZ Jacob", layer3.TalkerAliasSevenBit)
	if err != nil {
		return nil, err
	}
	var gps pdu.FullLinkControl
	if err := gps.SetGPSInfo(pdu.FLCGPSInfo{
		PositionError: layer3.PositionLessThan20M, Longitude: -97.7431, Latitude: 30.2672,
	}); err != nil {
		return nil, err
	}
	var lc pdu.FullLinkControl
	if err := lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 3100, SourceAddress: 3141592}); err != nil {
		return nil, err
	}
	rotation = append(rotation, gps)
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
		LC:        lc,
		ColorCode: colorCode,
		Source:    layer2.SyncSourceBS,
		Silence:   vocoder.AMBESilenceSlot(),
		EmbeddedLC: func(sf int) *pdu.FullLinkControl {
			if i := sf % (len(rotation) + 1); i > 0 {
				return &rotation[i-1]
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	frames := make([]vocoder.AMBEFrame, 200)
	for i := range frames {
		frames[i] = vocoder.AMBESilence()
	}
	voice, err := tx.AddFrames(frames...)
	if err != nil {
		return nil, err
	}
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(append(out, voice...), tail...), nil
}

func buildStream(ts1, ts2 []layer2.RawBurst) (stream []byte, n, flipped int, err error) {
	idle, err := layer2.BuildBurst(&pdu.PRFill{}, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return nil, 0, 0, err
	}
	bursts := max(len(ts1), len(ts2))
	stream = make([]byte, (junkBits+2*bursts*framer.OutboundPeriod+7)/8)
	put := func(v byte) {
		stream[n/8] |= v & 1 << (7 - n%8)
		n++
	}
	putBytes := func(b []byte) {
		for i := range len(b) * 8 {
			put(b[i/8] >> (7 - i%8))
		}
	}
	r := xorshift(1)
	for range junkBits {
		put(byte(r.next() & 1))
	}
	for i := range bursts {
		for tc, ts := range [2][]layer2.RawBurst{ts1, ts2} {
			burst := idle
			if i < len(ts) {
				burst = ts[i]
			}
			cach := layer2.CACH{TACT: pdu.TACT{AccessType: true, TDMAChannel: tc == 1, LCSS: enums.LCSSSingleFragment}}
			logical, err := cach.Encode()
			if err != nil {
				return nil, 0, 0, err
			}
			tx := layer2.CACHInterleave(logical)
			putBytes(tx[:])
			putBytes(burst[:])
		}
	}
	for i := junkBits; i < n; i++ {
		if r.next()%1_000_000 < berPPM {
			stream[i/8] ^= 0x80 >> (i % 8)
			flipped++
		}
	}
	return stream, n, flipped, nil
}

type channel struct {
	name      string
	rx        layer2.VoiceReceiver
	ta        pdu.TalkerAliasAssembler
	inCall    bool
	call      string
	gps       string
	voice     int
	positions [enums.VoiceBurstF + 1]int
	lcErrors  int
	ambe      fec.BERCalculator
}

func (c *channel) receive(w io.Writer, b *layer2.Burst) {
	if b.Kind == layer2.KindData {
		dt := b.SlotType.DataType
		if lc, ok := b.Data.(*pdu.FullLinkControl); ok && !b.FEC.PDU.Uncorrectable {
			if dt == enums.DataTypeVoiceLCHeader {
				c.start(w, lc, dt.String())
			}
			if dt == enums.DataTypeTerminatorWithLC {
				fmt.Fprintf(w, "%s call end: %s, %d voice bursts\n", c.name, describe(lc), c.voice)
				c.inCall = false
				c.ta.Reset()
			}
		}
	}
	lc, ok, err := c.rx.Receive(b)
	if err != nil && !errors.Is(err, layer2.ErrEmbeddedLCSequence) {
		c.lcErrors++
	}
	if b.Kind != layer2.KindVoice {
		return
	}
	if !c.inCall {
		fmt.Fprintf(w, "%s late entry at voice burst %s\n", c.name, b.VoiceBurst)
		c.inCall = true
	}
	c.voice++
	c.positions[b.VoiceBurst]++
	_, res := b.AMBEFrames()
	c.ambe.Add(res)
	if ok {
		c.embedded(w, &lc)
	}
}

func (c *channel) start(w io.Writer, lc *pdu.FullLinkControl, from string) {
	if d := describe(lc); d != c.call || !c.inCall {
		fmt.Fprintf(w, "%s call: %s (%s)\n", c.name, d, from)
		c.call = d
	}
	if !c.inCall {
		c.voice = 0
	}
	c.inCall = true
}

func (c *channel) embedded(w io.Writer, lc *pdu.FullLinkControl) {
	switch lc.FLCO {
	case enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser:
		if d := describe(lc); d != c.call {
			fmt.Fprintf(w, "%s call: %s (embedded LC)\n", c.name, d)
			c.call = d
		}
	case enums.FLCOGPSInfo:
		g, ok := lc.AsGPSInfo()
		if !ok {
			return
		}
		if pos := fmt.Sprintf("lat %.4f lon %.4f (%s)", g.Latitude, g.Longitude, g.PositionError.String()); pos != c.gps {
			fmt.Fprintf(w, "%s GPS: %s\n", c.name, pos)
			c.gps = pos
		}
	case enums.FLCOTalkerAliasHeader, enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3:
		alias, ok, err := c.ta.Add(lc)
		switch {
		case err != nil:
			fmt.Fprintf(w, "%s talker alias: %v\n", c.name, err)
		case ok:
			fmt.Fprintf(w, "%s talker alias: %q\n", c.name, alias)
		}
	case enums.FLCOTerminatorDataLinkControl:
	}
}

func describe(lc *pdu.FullLinkControl) string {
	dst, src, ok := lc.Addresses()
	if !ok {
		return lc.FLCO.String()
	}
	kind := "private"
	if _, ok := lc.AsGroupVoice(); ok {
		kind = "group"
	}
	return fmt.Sprintf("%s %d -> %d", kind, src, dst)
}

func (c *channel) summary(w io.Writer) {
	p := c.positions
	fmt.Fprintf(w, "%s: %d voice bursts (A=%d B=%d C=%d D=%d E=%d F=%d unknown=%d), %d embedded LC errors, AMBE BER %.2f%%\n",
		c.name, c.voice, p[enums.VoiceBurstA], p[enums.VoiceBurstB], p[enums.VoiceBurstC], p[enums.VoiceBurstD],
		p[enums.VoiceBurstE], p[enums.VoiceBurstF], p[enums.VoiceBurstUnknown], c.lcErrors, c.ambe.BER())
}

type xorshift uint64

func (x *xorshift) next() uint64 {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return uint64(*x)
}
