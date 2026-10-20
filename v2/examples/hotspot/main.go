// Command hotspot decodes captured bursts with bit errors, applies the MMDVMHost check and regenerates clean bursts.
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
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const colorCode = 1

func main() {
	def := filepath.Join("layer2", "testdata")
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

func loadBursts(path string) ([]layer2.RawBurst, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%33 != 0 {
		return nil, fmt.Errorf("%s: %d bytes is not a whole number of bursts", path, len(data))
	}
	bursts := make([]layer2.RawBurst, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		bursts = append(bursts, [33]byte(data[i:i+33]))
	}
	return bursts, nil
}

func flip(p *layer2.RawBurst, i int) { p[i/8] ^= 0x80 >> (i % 8) }

func addRFErrors(p *layer2.RawBurst) (burstFEC, ambe int) {
	var b layer2.Burst
	_ = b.DecodeFromBytes(*p)
	switch {
	case b.Kind == layer2.KindData:
		flip(p, 3)
		return 1, 0
	case b.HasEmbeddedSignalling():
		flip(p, 0)
		flip(p, 109)
		return 1, 1
	default:
		flip(p, 0)
		return 0, 1
	}
}

type rxStats struct {
	data, voice       int
	injFEC, injAMBE   int
	fec               fec.Result
	ambe              fec.Result
	embLCs, embMatch  int
	accepted, checked int
	header            *pdu.FullLinkControl
	frames            []vocoder.AMBEFrame
}

func receive(captured []layer2.RawBurst) (rxStats, error) {
	var s rxStats
	var rx layer2.VoiceReceiver
	cc := enums.ColorCode(colorCode)
	for _, clean := range captured {
		p := clean
		f, a := addRFErrors(&p)
		s.injFEC += f
		s.injAMBE += a
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(p, &layer2.DecodeOptions{ColorCode: &cc}); err != nil {
			return s, err
		}
		s.fec = s.fec.Combine(b.FEC.Aggregate())
		lc, ok, err := rx.Receive(&b)
		if err != nil {
			return s, err
		}
		if ok {
			s.embLCs++
			if s.header != nil && sameCall(&lc, s.header) {
				s.embMatch++
			}
		}
		switch b.Kind {
		case layer2.KindData:
			s.data++
			s.checked++
			if _, ok := p.MMDVMHostPayload(b.SlotType.DataType); ok {
				s.accepted++
			}
			if f, ok := b.Data.(*pdu.FullLinkControl); ok && b.SlotType.DataType == enums.DataTypeVoiceLCHeader {
				s.header = f
			}
		case layer2.KindVoice:
			s.voice++
			frames, res := b.AMBEFrames()
			s.ambe = s.ambe.Combine(res)
			s.frames = append(s.frames, frames[:]...)
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if s.header == nil {
		return s, errors.New("no Voice LC Header in the capture")
	}
	return s, nil
}

func sameCall(a, b *pdu.FullLinkControl) bool {
	at, as, aok := a.Addresses()
	bt, bs, bok := b.Addresses()
	return aok && bok && a.FLCO == b.FLCO && at == bt && as == bs
}

func regenerate(lc pdu.FullLinkControl, frames []vocoder.AMBEFrame) ([]layer2.RawBurst, error) {
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: colorCode, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	voice, err := tx.AddFrames(frames...)
	if err != nil {
		return nil, err
	}
	out = append(out, voice...)
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(out, tail...), nil
}

func checkRegenerated(bursts []layer2.RawBurst, captured []layer2.RawBurst, frames []vocoder.AMBEFrame) (clean, framesOK bool, identical int) {
	clean, framesOK = true, true
	var got []vocoder.AMBEFrame
	for i, p := range bursts {
		if i < len(captured) && p == captured[i] {
			identical++
		}
		b, err := layer2.NewBurstFromBytes(p)
		if err != nil || b.FEC.Aggregate().HasErrors() {
			clean = false
		}
		if b.Kind == layer2.KindVoice {
			f, res := b.AMBEFrames()
			if res.HasErrors() {
				clean = false
			}
			got = append(got, f[:]...)
		}
	}
	if len(got) < len(frames) {
		return clean, false, identical
	}
	for i := range frames {
		if got[i] != frames[i] {
			framesOK = false
		}
	}
	return clean, framesOK, identical
}

func rewriteTarget(header [33]byte, target uint32) (before, after, sameAsCodeword bool, decoded uint32, err error) {
	p, _ := layer2.RawBurst(header).BPTCPayload()
	lc := pdu.LCBytes(p[:9])
	_, source, _ := lc.Addresses()
	if err = lc.SetAddresses(target, source); err != nil {
		err = fmt.Errorf("cannot set the target of a %s LC: %w", lc.FLCO(), err)
		return
	}
	copy(p[:9], lc[:])
	before = layer2.MMDVMHostAccepts(enums.DataTypeVoiceLCHeader, p)
	if err = layer2.MMDVMHostSeal(enums.DataTypeVoiceLCHeader, &p); err != nil {
		return
	}
	after = layer2.MMDVMHostAccepts(enums.DataTypeVoiceLCHeader, p)
	cw, err := lc.Codeword(enums.DataTypeVoiceLCHeader)
	if err != nil {
		return
	}
	sameAsCodeword = cw == p
	out, err := layer2.BuildDataBurst(p, enums.DataTypeVoiceLCHeader, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return
	}
	b, err := layer2.NewBurstFromBytes(out)
	if err != nil {
		return
	}
	flc, ok := b.Data.(*pdu.FullLinkControl)
	if !ok || flc.FEC.HasErrors() {
		err = errors.New("rewritten header does not decode cleanly")
		return
	}
	decoded, _, _ = flc.Addresses()
	return
}

func run(w io.Writer, dataDir string) error {
	captured, err := loadBursts(filepath.Join(dataDir, "voice.bin"))
	if err != nil {
		return err
	}
	s, err := receive(captured)
	if err != nil {
		return err
	}
	target, source, _ := s.header.Addresses()
	fmt.Fprintf(w, "capture: %d bursts (%d data, %d voice), color code %d\n", len(captured), s.data, s.voice, colorCode)
	fmt.Fprintf(w, "call: %s %d -> %d\n", s.header.FLCO, source, target)
	fmt.Fprintf(w, "burst FEC: injected %d, corrected %d, uncorrectable %t\n", s.injFEC, s.fec.ErrorsCorrected, s.fec.Uncorrectable)
	fmt.Fprintf(w, "AMBE FEC: injected %d, corrected %d, uncorrectable %t\n", s.injAMBE, s.ambe.ErrorsCorrected, s.ambe.Uncorrectable)
	fmt.Fprintf(w, "mmdvmhost accepts: %d/%d data bursts\n", s.accepted, s.checked)
	fmt.Fprintf(w, "embedded LC: %d/%d match the header\n", s.embMatch, s.embLCs)

	regen, err := regenerate(*s.header, s.frames)
	if err != nil {
		return err
	}
	clean, framesOK, identical := checkRegenerated(regen, captured, s.frames)
	fmt.Fprintf(w, "regenerated: %d bursts from %d AMBE frames, clean %t, frames match %t, %d identical to capture\n",
		len(regen), len(s.frames), clean, framesOK, identical)

	before, after, sameCW, decoded, err := rewriteTarget(captured[0], 91)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "rewrite target %d -> %d: accepted before seal %t, after seal %t, equals LCBytes.Codeword %t\n",
		target, decoded, before, after, sameCW)
	return nil
}
