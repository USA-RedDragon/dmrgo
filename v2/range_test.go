package dmrgo_test

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func isRangeError(err error, typ, field string, n int) bool {
	var rangeErr *dmrgo.FieldRangeError
	return errors.As(err, &rangeErr) && rangeErr.Type == typ && rangeErr.Field == field && rangeErr.Bits == n
}

type paddingRejectCase struct {
	name       string
	bits       int
	typ, field string
	run        func(t *testing.T, p []byte) error
}

func TestEncodeRejectsPaddingBits(t *testing.T) {
	t.Parallel()

	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	cases := []paddingRejectCase{
		{"layer2.CACH.Encode", 17, "CACH", "Payload", func(_ *testing.T, p []byte) error {
			c := layer2.CACH{Payload: [3]byte(p)}
			_, err := c.Encode()
			return err
		}},
		{"pdu.MBCContinuation.Encode", 95, "MBCContinuation", "Data", func(_ *testing.T, p []byte) error {
			m := pdu.MBCContinuation{Data: [12]byte(p)}
			_, err := m.Encode()
			return err
		}},
		{"pdu.UnifiedSingleBlockData.Encode", 76, "UnifiedSingleBlockData", "Payload", func(_ *testing.T, p []byte) error {
			u := pdu.UnifiedSingleBlockData{Payload: [10]byte(p)}
			_, err := u.Encode()
			return err
		}},
		{"vocoder.EncodeAMBE72", 49, "vocoder", "AMBEFrame", func(_ *testing.T, p []byte) error {
			_, err := vocoder.EncodeAMBE72(vocoder.AMBEFrame(p))
			return err
		}},
		{"layer2.BuildVoiceBurst", 49, "vocoder", "AMBEFrame", func(_ *testing.T, p []byte) error {
			var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
			frames[2] = vocoder.AMBEFrame(p)
			_, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.VoiceTransmitter.AppendFrames", 49, "vocoder", "AMBEFrame", func(_ *testing.T, p []byte) error {
			tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc}))
			_, err := tx.AppendFrames(nil, vocoder.AMBEFrame(p))
			return err
		}},
		{"voice.Transmitter.Frame", 49, "vocoder", "AMBEFrame", func(t *testing.T, p []byte) error {
			tx := must(voice.NewTransmitter(&voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()}))
			if err := tx.Start(0, voice.Call{Group: true, Target: 91, Source: 3191868}, nil); err != nil {
				t.Fatal(err)
			}
			return tx.Frame(vocoder.AMBEFrame(p))
		}},
	}
	r := rand.New(rand.NewPCG(3, 4))
	for _, c := range cases {
		n := (c.bits + 7) / 8
		p := make([]byte, n)
		for i := range p {
			p[i] = byte(r.Uint32())
		}
		p[n-1] &^= byte(1)<<(8*n-c.bits) - 1
		if err := c.run(t, p); err != nil {
			t.Fatalf("%s: no padding bit set: %v", c.name, err)
		}
		for i := c.bits; i < 8*n; i++ {
			q := append([]byte(nil), p...)
			q[i/8] |= 0x80 >> (i % 8)
			if err := c.run(t, q); !isRangeError(err, c.typ, c.field, c.bits) {
				t.Errorf("%s: padding bit %d set: error %v, want the FieldRangeError of %s.%s", c.name, i, err, c.typ, c.field)
			}
		}
	}
}

func TestDecodeAMBE72WritesZeroPadding(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(5, 6))
	for range 2000 {
		var slot [9]byte
		for i := range slot {
			slot[i] = byte(r.Uint32())
		}
		f, _ := vocoder.DecodeAMBE72(slot)
		if f[6]&0x7F != 0 {
			t.Fatalf("DecodeAMBE72(%x) = %x, whose padding bits are not 0", slot, f)
		}
		if _, err := vocoder.EncodeAMBE72(f); err != nil {
			t.Fatalf("EncodeAMBE72 of a decoded frame: %v", err)
		}
	}
}

type rangeCase struct {
	name       string
	typ, field string
	bits       int
	run        func() error
}

func TestEncodeRejectsOutOfRangeValues(t *testing.T) {
	t.Parallel()

	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	idle := must(layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS))
	cases := []rangeCase{
		{"layer2.BuildBurst", "SlotType", "ColorCode", 4, func() error {
			_, err := layer2.BuildBurst(&pdu.PRFill{}, 16, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.BuildDataBurst", "SlotType", "ColorCode", 4, func() error {
			_, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 16, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.BuildRate34DataBurst", "SlotType", "ColorCode", 4, func() error {
			_, err := layer2.BuildRate34DataBurst([18]byte{}, 16, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.BuildRate1DataBurst", "SlotType", "ColorCode", 4, func() error {
			_, err := layer2.BuildRate1DataBurst([24]byte{}, 16, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.BuildVoiceBurst", "EmbeddedSignalling", "ColorCode", 4, func() error {
			_, err := layer2.BuildVoiceBurst([layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{}, enums.VoiceBurstB,
				pdu.EmbeddedSignalling{ColorCode: 16}, [4]byte{}, layer2.SyncSourceBS)
			return err
		}},
		{"layer2.NewVoiceTransmitter", "SlotType", "ColorCode", 4, func() error {
			_, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 16})
			return err
		}},
		{"pdu.NewVoiceLCBytes", "ServiceOptions", "Reserved", 2, func() error {
			_, err := pdu.NewVoiceLCBytes(true, 91, 3191868, layer3.ServiceOptions{Reserved: 4})
			return err
		}},
		{"pdu.NewVoiceLCBytes", "ServiceOptions", "PriorityLevel", 2, func() error {
			_, err := pdu.NewVoiceLCBytes(true, 91, 3191868, layer3.ServiceOptions{PriorityLevel: 4})
			return err
		}},
		{"pdu.SysCode.Pack", "SysCode", "Model", 2, func() error {
			_, err := pdu.SysCode{Model: 4}.Pack()
			return err
		}},
		{"pdu.SysCode.Pack", "SysCode", "Par", 2, func() error {
			_, err := pdu.SysCode{Par: 4}.Pack()
			return err
		}},
		{"packet.NewResponse", "Response", "Class", 2, func() error {
			_, err := packet.NewResponse(&pdu.ConfirmedDataHeader{}, 1<<5|packet.ResponseACK, nil)
			return err
		}},
	}
	for _, m := range []pdu.NetworkModel{pdu.NetworkModelTiny, pdu.NetworkModelSmall, pdu.NetworkModelLarge, pdu.NetworkModelHuge} {
		cases = append(cases,
			rangeCase{"pdu.SysCode.Pack", "SysCode", "Net", must(m.NetBits()), func() error {
				_, err := pdu.SysCode{Model: m, Net: 1 << must(m.NetBits())}.Pack()
				return err
			}},
			rangeCase{"pdu.SysCode.Pack", "SysCode", "Site", must(m.SiteBits()), func() error {
				_, err := pdu.SysCode{Model: m, Site: 1 << must(m.SiteBits())}.Pack()
				return err
			}},
		)
	}
	for k := 3; k < 8; k++ {
		cases = append(cases, rangeCase{"layer2.Burst.Encode", "Burst", "BPTCReserved", 3, func() error {
			b := must(layer2.NewBurstFromBytes(idle))
			b.BPTCReserved = 1 << k
			_, err := b.Encode()
			return err
		}})
	}
	for _, c := range cases {
		if err := c.run(); !isRangeError(err, c.typ, c.field, c.bits) {
			t.Errorf("%s: error %v, want the FieldRangeError of %s.%s in %d bits", c.name, err, c.typ, c.field, c.bits)
		}
	}
	b := must(layer2.NewBurstFromBytes(idle))
	b.BPTCReserved = 7
	if _, err := b.Encode(); err != nil {
		t.Errorf("Burst.Encode with BPTCReserved 7: %v", err)
	}
}

func TestSettersRejectOutOfRangeValues(t *testing.T) {
	t.Parallel()

	idle := must(layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS))
	var padded [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	padded[1][6] = 0x01
	type setterCase struct {
		name       string
		typ, field string
		bits       int
		run        func() (unchanged bool, err error)
	}
	raw := func(set func(r *layer2.RawBurst) error) func() (bool, error) {
		return func() (bool, error) {
			r := idle
			err := set(&r)
			return r == idle, err
		}
	}
	cases := []setterCase{
		{"RawBurst.SetBPTCReserved", "RawBurst", "BPTCReserved", 3, raw(func(r *layer2.RawBurst) error { return r.SetBPTCReserved(8) })},
		{"RawBurst.SetSlotType", "SlotType", "ColorCode", 4, raw(func(r *layer2.RawBurst) error {
			return r.SetSlotType(pdu.SlotType{ColorCode: 16})
		})},
		{"RawBurst.SetEMB", "EmbeddedSignalling", "LCSS", 2, raw(func(r *layer2.RawBurst) error {
			return r.SetEMB(pdu.EmbeddedSignalling{LCSS: 4})
		})},
		{"RawBurst.SetStandaloneRCEMB", "EmbeddedSignalling", "ColorCode", 4, raw(func(r *layer2.RawBurst) error {
			return r.SetStandaloneRCEMB(pdu.EmbeddedSignalling{ColorCode: 16})
		})},
		{"RawBurst.SetAMBEFrames", "vocoder", "AMBEFrame", 49, raw(func(r *layer2.RawBurst) error { return r.SetAMBEFrames(padded) })},
		{"Burst.SetAMBEFrames", "vocoder", "AMBEFrame", 49, func() (bool, error) {
			var b layer2.Burst
			b.VoiceSlots[0][0] = 0x5A
			want := b.VoiceSlots
			err := b.SetAMBEFrames(padded)
			return b.VoiceSlots == want, err
		}},
		{"Rate12Data.SetConfirmed", "Rate12Data", "DBSN", 7, func() (bool, error) {
			var d pdu.Rate12Data
			err := d.SetConfirmed(128, [10]byte{1})
			return d == pdu.Rate12Data{}, err
		}},
		{"Rate34Data.SetConfirmed", "Rate34Data", "DBSN", 7, func() (bool, error) {
			var d pdu.Rate34Data
			err := d.SetConfirmed(128, [16]byte{1})
			return d == pdu.Rate34Data{}, err
		}},
		{"Rate1Data.SetConfirmed", "Rate1Data", "DBSN", 7, func() (bool, error) {
			var d pdu.Rate1Data
			err := d.SetConfirmed(128, [22]byte{1})
			return d == pdu.Rate1Data{}, err
		}},
		{"vocoder.PackAMBEVoice", "vocoder", "AMBEFrame", 49, func() (bool, error) {
			p, err := vocoder.PackAMBEVoice(padded)
			return p == [19]byte{}, err
		}},
		{"vocoder.PackAMBEVoiceFlags", "vocoder", "AMBEFrame", 49, func() (bool, error) {
			p, err := vocoder.PackAMBEVoiceFlags(padded, [3]bool{false, true, true})
			return p == [19]byte{}, err
		}},
	}
	for _, c := range cases {
		unchanged, err := c.run()
		if !isRangeError(err, c.typ, c.field, c.bits) {
			t.Errorf("%s: error %v, want the FieldRangeError of %s.%s in %d bits", c.name, err, c.typ, c.field, c.bits)
		}
		if !unchanged {
			t.Errorf("%s: changed its target on an error", c.name)
		}
	}
}

func TestSetAddressesErrors(t *testing.T) {
	t.Parallel()

	type setter struct {
		name string
		typ  string
		dst  string
		src  string
		set  func(target, source uint32) (unchanged bool, err error)
		none func() error
	}
	lcb := must(pdu.NewVoiceLCBytes(true, 91, 3191868, layer3.ServiceOptions{}))
	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	pre := pdu.CSBK{Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{TargetAddress: 1, SourceAddress: 2}}
	csbkb := pdu.CSBKBytes(must(pre.Encode()))
	hdr := pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{LLIDDestination: 1, LLIDSource: 2}}
	hdrb := pdu.DataHeaderBytes(must(hdr.Encode()))
	setters := []setter{
		{"LCBytes", "LCBytes", "TargetAddress", "SourceAddress", func(a, b uint32) (bool, error) {
			l := lcb
			err := l.SetAddresses(a, b)
			return l == lcb, err
		}, func() error {
			l := lcb
			noErr(l.SetFLCO(enums.FLCOTerminatorDataLinkControl))
			return l.SetAddresses(1, 2)
		}},
		{"FullLinkControl", "FullLinkControl", "TargetAddress", "SourceAddress", func(a, b uint32) (bool, error) {
			l := lc
			gv := *lc.GroupVoice
			l.GroupVoice = &gv
			err := l.SetAddresses(a, b)
			return gv == *lc.GroupVoice, err
		}, func() error {
			l := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser}
			return l.SetAddresses(1, 2)
		}},
		{"CSBK", "CSBK", "TargetAddress", "SourceAddress", func(a, b uint32) (bool, error) {
			c := pre
			p := *pre.PreamblePDU
			c.PreamblePDU = &p
			err := c.SetAddresses(a, b)
			return p == *pre.PreamblePDU, err
		}, func() error {
			c := pdu.CSBK{Opcode: pdu.CSBKPreamble}
			return c.SetAddresses(1, 2)
		}},
		{"CSBKBytes", "CSBKBytes", "TargetAddress", "SourceAddress", func(a, b uint32) (bool, error) {
			c := csbkb
			err := c.SetAddresses(a, b)
			return c == csbkb, err
		}, func() error {
			var c pdu.CSBKBytes
			c[1] = 0x10
			return c.SetAddresses(1, 2)
		}},
		{"DataHeader", "DataHeader", "LLIDDestination", "LLIDSource", func(a, b uint32) (bool, error) {
			h := hdr
			u := *hdr.UnconfirmedDataHeader
			h.UnconfirmedDataHeader = &u
			err := h.SetAddresses(a, b)
			return u == *hdr.UnconfirmedDataHeader, err
		}, func() error {
			var h pdu.DataHeader
			return h.SetAddresses(1, 2)
		}},
		{"DataHeaderBytes", "DataHeaderBytes", "LLIDDestination", "LLIDSource", func(a, b uint32) (bool, error) {
			h := hdrb
			err := h.SetAddresses(a, b)
			return h == hdrb, err
		}, func() error {
			h := hdrb
			h[0] = h[0]&0xF0 | byte(pdu.FormatProprietary)
			return h.SetAddresses(1, 2)
		}},
	}
	for _, s := range setters {
		if unchanged, err := s.set(1<<24, 2); !isRangeError(err, s.typ, s.dst, 24) || !unchanged {
			t.Errorf("%s: target over 24 bits: %v, unchanged %t", s.name, err, unchanged)
		}
		if unchanged, err := s.set(1, 1<<24); !isRangeError(err, s.typ, s.src, 24) || !unchanged {
			t.Errorf("%s: source over 24 bits: %v, unchanged %t", s.name, err, unchanged)
		}
		if _, err := s.set(5, 6); err != nil {
			t.Errorf("%s: %v", s.name, err)
		}
		if err := s.none(); !errors.Is(err, pdu.ErrNoAddresses) {
			t.Errorf("%s without address fields: %v, want pdu.ErrNoAddresses", s.name, err)
		}
	}
}

func noErr(err error) {
	if err != nil {
		panic(err)
	}
}
