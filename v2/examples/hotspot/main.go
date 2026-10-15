// Command hotspot shows the receive and retransmit path of a DMR hotspot or
// repeater: it decodes captured bursts with injected bit errors, applies the
// MMDVMHost acceptance check, reassembles the embedded LC, regenerates clean
// bursts from the corrected LC and AMBE+2 frames, and rewrites the target of
// a Voice LC Header with MMDVMHostSeal.
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

func loadBursts(path string) ([][33]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%33 != 0 {
		return nil, fmt.Errorf("%s: %d bytes is not a whole number of bursts", path, len(data))
	}
	bursts := make([][33]byte, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		bursts = append(bursts, [33]byte(data[i:i+33]))
	}
	return bursts, nil
}

func flip(p *[33]byte, i int) { p[i/8] ^= 0x80 >> (i % 8) }

// addRFErrors flips one bit per FEC layer that the burst carries: a BPTC bit
// of a data burst, the first bit of vocoder slot 0, which is Golay coded,
// and an EMB bit of an embedded signalling burst.
func addRFErrors(p *[33]byte) (burstFEC, ambe int) {
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

func receive(captured [][33]byte) (rxStats, error) {
	var s rxStats
	var rx layer2.VoiceReceiver
	cc := uint8(colorCode)
	for _, clean := range captured {
		p := clean
		f, a := addRFErrors(&p)
		s.injFEC += f
		s.injAMBE += a
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(p, layer2.DecodeOptions{ColorCode: &cc}); err != nil {
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
			raw := layer2.RawBurst(p)
			payload, _ := raw.BPTCPayload()
			s.checked++
			if layer2.MMDVMHostAccepts(b.SlotType.DataType, payload) {
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

func regenerate(lc pdu.FullLinkControl, frames []vocoder.AMBEFrame) ([][33]byte, error) {
	tx, err := layer2.NewVoiceTransmitter(lc, colorCode, layer2.SyncSourceBS, vocoder.EncodeAMBE72(vocoder.AMBESilence()))
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

func checkRegenerated(bursts [][33]byte, captured [][33]byte, frames []vocoder.AMBEFrame) (clean, framesOK bool, identical int) {
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

// rewriteTarget sets the target address of a Voice LC Header payload in
// place and reseals it, as a hotspot does when it maps a talkgroup.
func rewriteTarget(header [33]byte, target uint32) (before, after, sameAsCodeword bool, decoded uint32, err error) {
	p, _ := layer2.RawBurst(header).BPTCPayload()
	lc := pdu.LCBytes(p[:9])
	_, source, _ := lc.Addresses()
	if !lc.SetAddresses(target, source) {
		err = fmt.Errorf("cannot set the target of a %s LC", lc.FLCO())
		return
	}
	copy(p[:9], lc[:])
	before = layer2.MMDVMHostAccepts(enums.DataTypeVoiceLCHeader, p)
	if err = layer2.MMDVMHostSeal(enums.DataTypeVoiceLCHeader, &p); err != nil {
		return
	}
	after = layer2.MMDVMHostAccepts(enums.DataTypeVoiceLCHeader, p)
	sameAsCodeword = pdu.VoiceLCHeaderCodeword(lc) == p
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

// run processes voice.bin from dataDir and prints the results to w. The AMBE
// FEC corrects 3 more errors than are injected because burst 20 of the
// capture has them on the air, so its regenerated burst differs from the
// capture. The capture also lost a burst A at burst 49, which shifts the
// regenerated superframes after it.
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
	fmt.Fprintf(w, "rewrite target %d -> %d: accepted before seal %t, after seal %t, equals VoiceLCHeaderCodeword %t\n",
		target, decoded, before, after, sameCW)
	return nil
}
