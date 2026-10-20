// Command bridge receives a voice call, rewrites its talkgroup and source ID, and sends it again.
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
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	newTarget = 91
	newSource = 3100999
)

func main() {
	defaultDir := filepath.Join("layer2", "testdata")
	if _, file, _, ok := runtime.Caller(0); ok {
		defaultDir = filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
	}
	dataDir := flag.String("data", defaultDir, "directory with the burst captures")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, dataDir string) error {
	if err := bridgeVoice(w, filepath.Join(dataDir, "voice.bin")); err != nil {
		return err
	}
	return bridgeSMS(w, filepath.Join(dataDir, "d-sms.bin"))
}

func loadBursts(path string) ([]layer2.RawBurst, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw)%33 != 0 {
		return nil, fmt.Errorf("%s: %d bytes is not a whole number of bursts", path, len(raw))
	}
	out := make([]layer2.RawBurst, 0, len(raw)/33)
	for i := 0; i < len(raw); i += 33 {
		out = append(out, [33]byte(raw[i:i+33]))
	}
	return out, nil
}

type inbound struct {
	lc        pdu.FullLinkControl
	colorCode enums.ColorCode
	slots     [][9]byte
}

func receive(bursts []layer2.RawBurst) (inbound, error) {
	var in inbound
	var rx layer2.VoiceReceiver
	haveLC := false
	for i, p := range bursts {
		b, err := layer2.NewBurstFromBytes(p)
		if err != nil {
			return in, fmt.Errorf("burst %d: %w", i, err)
		}
		if _, _, err := rx.Receive(b); err != nil {
			return in, fmt.Errorf("burst %d: %w", i, err)
		}
		switch b.Kind {
		case layer2.KindVoice:
			in.slots = append(in.slots, b.VoiceSlots[:]...)
		case layer2.KindData:
			lc, ok := b.Data.(*pdu.FullLinkControl)
			if ok && !haveLC && b.SlotType.DataType == enums.DataTypeVoiceLCHeader && !b.FEC.PDU.Uncorrectable {
				in.lc, in.colorCode, haveLC = *lc, b.SlotType.ColorCode, true
			}
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if !haveLC {
		return in, errors.New("no usable Voice LC Header")
	}
	return in, nil
}

func transmit(in inbound) ([]layer2.RawBurst, error) {
	silence := vocoder.AMBESilenceSlot()
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: in.lc, ColorCode: in.colorCode, Source: layer2.SyncSourceBS, Silence: silence})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	bursts, err := tx.AddSlots(in.slots...)
	if err != nil {
		return nil, err
	}
	out = append(out, bursts...)
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(out, tail...), nil
}

type verified struct {
	lcBursts, lcAccepted  int
	embedded, embeddedNew int
	slots                 [][9]byte
	ambeUncorrectable     int
}

func verify(bursts []layer2.RawBurst) (verified, error) {
	var v verified
	rx := layer2.VoiceReceiver{Strict: true}
	for i, p := range bursts {
		b, err := layer2.NewBurstFromBytes(p)
		if err != nil {
			return v, fmt.Errorf("rewritten burst %d: %w", i, err)
		}
		lc, ok, err := rx.Receive(b)
		if err != nil {
			return v, fmt.Errorf("rewritten burst %d: %w", i, err)
		}
		if ok {
			v.embedded++
			if t, s, _ := lc.Addresses(); lc.FLCO == enums.FLCOGroupVoiceChannelUser && t == newTarget && s == newSource {
				v.embeddedNew++
			}
		}
		if b.Kind == layer2.KindVoice {
			v.slots = append(v.slots, b.VoiceSlots[:]...)
			if _, res := b.AMBEFrames(); res.Uncorrectable {
				v.ambeUncorrectable++
			}
			continue
		}
		flc, isLC := b.Data.(*pdu.FullLinkControl)
		if !isLC {
			continue
		}
		v.lcBursts++
		raw := p
		payload, _ := raw.BPTCPayload()
		t, s, _ := flc.Addresses()
		if flc.FLCO == enums.FLCOGroupVoiceChannelUser && t == newTarget && s == newSource && !b.FEC.PDU.HasErrors() && layer2.MMDVMHostAccepts(b.SlotType.DataType, payload) {
			v.lcAccepted++
		}
	}
	return v, nil
}

func bridgeVoice(w io.Writer, path string) error {
	bursts, err := loadBursts(path)
	if err != nil {
		return err
	}
	in, err := receive(bursts)
	if err != nil {
		return err
	}
	oldTarget, oldSource, _ := in.lc.Addresses()
	fmt.Fprintf(w, "voice in: %d bursts, %d slots, %s %d -> %d\n", len(bursts), len(in.slots), in.lc.FLCO, oldSource, oldTarget)

	if err := toGroupCall(&in.lc); err != nil {
		return err
	}
	out, err := transmit(in)
	if err != nil {
		return err
	}
	v, err := verify(out)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "voice out: %d bursts, %d slots, %s %d -> %d\n", len(out), len(v.slots), in.lc.FLCO, newSource, newTarget)

	if v.lcBursts != 2 || v.lcAccepted != v.lcBursts {
		return fmt.Errorf("header and terminator: %d of %d carry the new IDs and pass MMDVMHostAccepts", v.lcAccepted, v.lcBursts)
	}
	fmt.Fprintf(w, "ok header and terminator carry the new IDs and pass MMDVMHostAccepts\n")
	if v.embedded == 0 || v.embeddedNew != v.embedded {
		return fmt.Errorf("embedded LC: %d of %d superframes carry the new IDs", v.embeddedNew, v.embedded)
	}
	fmt.Fprintf(w, "ok embedded LC carries the new IDs in %d/%d superframes\n", v.embeddedNew, v.embedded)
	if len(v.slots) < len(in.slots) || v.ambeUncorrectable != 0 {
		return fmt.Errorf("voice: %d slots out for %d in, %d bursts with uncorrectable AMBE", len(v.slots), len(in.slots), v.ambeUncorrectable)
	}
	for i := range in.slots {
		if v.slots[i] != in.slots[i] {
			return fmt.Errorf("voice slot %d changed", i)
		}
	}
	fmt.Fprintf(w, "ok %d voice slots forwarded unchanged, %d silence slots of padding\n", len(in.slots), len(v.slots)-len(in.slots))
	return nil
}

func toGroupCall(lc *pdu.FullLinkControl) error {
	if lc.FID != enums.FIDStandardized {
		return fmt.Errorf("cannot rewrite an LC of FID %s", lc.FID)
	}
	switch {
	case lc.UnitToUnit != nil:
		lc.GroupVoice = &pdu.FLCGroupVoice{ServiceOptions: lc.UnitToUnit.ServiceOptions}
		lc.UnitToUnit = nil
	case lc.GroupVoice == nil:
		return fmt.Errorf("cannot rewrite a %s LC", lc.FLCO)
	}
	lc.FLCO = enums.FLCOGroupVoiceChannelUser
	if err := lc.SetAddresses(newTarget, newSource); err != nil {
		return fmt.Errorf("cannot set addresses of a %s LC: %w", lc.FLCO, err)
	}
	return nil
}

func bridgeSMS(w io.Writer, path string) error {
	bursts, err := loadBursts(path)
	if err != nil {
		return err
	}
	var preambles, headers int
	for i, p := range bursts {
		b, err := layer2.NewBurstFromBytes(p)
		if err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
		switch d := b.Data.(type) {
		case *pdu.CSBK:
			if d.PreamblePDU == nil || d.SetAddresses(newTarget, newSource) != nil {
				continue
			}
			preambles++
		case *pdu.DataHeader:
			if d.SetAddresses(newTarget, newSource) != nil {
				continue
			}
			headers++
		default:
			continue
		}
		enc, err := b.Encode()
		if err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
		if err := checkSMSBurst(enc); err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
	}
	if preambles == 0 || headers == 0 {
		return fmt.Errorf("sms: %d CSBK preambles and %d data headers rewritten", preambles, headers)
	}
	fmt.Fprintf(w, "ok sms: %d CSBK preambles and %d data headers rewritten to %d -> %d\n", preambles, headers, newSource, newTarget)
	return nil
}

func checkSMSBurst(p [33]byte) error {
	b, err := layer2.NewBurstFromBytes(p)
	if err != nil {
		return err
	}
	raw := layer2.RawBurst(p)
	payload, _ := raw.BPTCPayload()
	if b.FEC.PDU.HasErrors() || !layer2.MMDVMHostAccepts(b.SlotType.DataType, payload) {
		return fmt.Errorf("rewritten %s does not check", b.SlotType.DataType)
	}
	var dst, src uint32
	var ok bool
	switch d := b.Data.(type) {
	case *pdu.CSBK:
		dst, src, ok = d.Addresses()
	case *pdu.DataHeader:
		dst, src, ok = d.Addresses()
	}
	if !ok || dst != newTarget || src != newSource {
		return fmt.Errorf("rewritten %s carries %d -> %d", b.SlotType.DataType, src, dst)
	}
	return nil
}
