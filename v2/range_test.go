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

// must returns v, and panics when err is not nil.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// isRangeError reports whether err is the *dmrgo.FieldRangeError of field
// typ.field in n bits.
func isRangeError(err error, typ, field string, n int) bool {
	var rangeErr *dmrgo.FieldRangeError
	return errors.As(err, &rangeErr) && rangeErr.Type == typ && rangeErr.Field == field && rangeErr.Bits == n
}

// paddingRejectCase is an encoder whose input holds a field of bits that is
// not a whole number of bytes: run encodes the field p and returns the
// error, which for a set padding bit is the FieldRangeError of typ.field.
type paddingRejectCase struct {
	name       string
	bits       int
	typ, field string
	run        func(t *testing.T, p []byte) error
}

// TestEncodeRejectsPaddingBits checks that each encoder returns the
// *dmrgo.FieldRangeError of the field when one padding bit of its input is
// set, and no error when none is.
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

// TestDecodeAMBE72WritesZeroPadding checks that DecodeAMBE72 returns frames
// whose padding bits are 0, which EncodeAMBE72 takes.
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

// rangeCase is a call of an encoder written by hand with a value that does
// not fit its bits, which must return the FieldRangeError of typ.field in
// bits bits.
type rangeCase struct {
	name       string
	typ, field string
	bits       int
	run        func() error
}

// TestEncodeRejectsOutOfRangeValues checks that the encoders written by hand
// return the *dmrgo.FieldRangeError of a value that does not fit its bits.
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
			rangeCase{"pdu.SysCode.Pack", "SysCode", "Net", m.NetBits(), func() error {
				_, err := pdu.SysCode{Model: m, Net: 1 << m.NetBits()}.Pack()
				return err
			}},
			rangeCase{"pdu.SysCode.Pack", "SysCode", "Site", m.SiteBits(), func() error {
				_, err := pdu.SysCode{Model: m, Site: 1 << m.SiteBits()}.Pack()
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

// TestSettersRejectOutOfRangeValues checks that each setter returns the
// *dmrgo.FieldRangeError of a value that does not fit its bits, or of a set
// padding bit, and leaves its target as it was.
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
