package pdu_test

import (
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	fecrefcrc "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func FuzzTalkerAlias(f *testing.F) {
	talkerAliasSeeds(func(alias string, format, order uint8, extra []byte) { f.Add(alias, format, order, extra) })
	f.Fuzz(fuzzTalkerAlias)
}

func talkerAliasSeeds(add func(alias string, format, order uint8, extra []byte)) {
	add("N0CALL", uint8(layer3.TalkerAliasSevenBit), uint8(0), []byte{})
	add("Jacob McSwain KI5VMF", uint8(layer3.TalkerAliasISOEightBit), uint8(0x1B), []byte{})
	add("Grüße aus Texas 73", uint8(layer3.TalkerAliasUTF8), uint8(0x27), []byte{})
	add("日本語のエイリアス", uint8(layer3.TalkerAliasUTF16), uint8(0x93), []byte{})
	add("", uint8(layer3.TalkerAliasUTF8), uint8(0), []byte{0x10, 0x00, 0xC0, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46})
}

func fuzzTalkerAlias(t *testing.T, alias string, format, order uint8, extra []byte) {
	tf := layer3.TalkerAliasDataFormat(format % 4)
	lcs, err := pdu.EncodeTalkerAlias(alias, tf)
	var a pdu.TalkerAliasAssembler
	for i := 0; i+9 <= len(extra); i += 9 {
		lc := pdu.DecodeFullLinkControlBytes([9]byte(extra[i : i+9]))
		_, _, _ = a.Add(&lc)
	}
	if err != nil {
		return
	}
	want := alias
	if !utf8.ValidString(alias) && tf != layer3.TalkerAliasUTF8 {
		want = string([]rune(alias))
	}
	decoded := make([]pdu.FullLinkControl, len(lcs))
	for i := range lcs {
		lcs[i].BurstDataType = enums.DataTypeVoiceLCHeader
		decoded[i] = pdu.DecodeFullLinkControl(must(lcs[i].Encode()), enums.DataTypeVoiceLCHeader)
	}
	blocks := make([]*pdu.FLCTalkerAliasBlock, 0, len(decoded)-1)
	for _, lc := range decoded[1:] {
		blk, ok := lc.AsTalkerAliasBlock()
		if !ok {
			t.Fatalf("EncodeTalkerAlias(%q, %s) block is %s", alias, tf, lc.String())
		}
		blocks = append(blocks, &blk)
	}
	header, ok := decoded[0].AsTalkerAliasHeader()
	if !ok {
		t.Fatalf("EncodeTalkerAlias(%q, %s) header is %s", alias, tf, decoded[0].String())
	}
	got, err := pdu.TalkerAlias(&header, blocks...)
	if tf == layer3.TalkerAliasUTF8 && !utf8.ValidString(alias) {
		if !errors.Is(err, pdu.ErrTalkerAliasInvalidUTF8) {
			t.Fatalf("TalkerAlias of invalid UTF-8 %q: %q %v", alias, got, err)
		}
		return
	}
	if err != nil || got != want {
		t.Fatalf("TalkerAlias(EncodeTalkerAlias(%q, %s)) = %q %v", alias, tf, got, err)
	}
	if len(blocks) > 0 {
		if _, err := pdu.TalkerAlias(&header, blocks[:len(blocks)-1]...); !errors.Is(err, pdu.ErrTalkerAliasIncomplete) {
			t.Fatalf("TalkerAlias without the last block of %q: %v", alias, err)
		}
	}

	a.Reset()
	var done bool
	for i := range 2 * len(decoded) {
		lc := &decoded[(i+int(order))%len(decoded)]
		s, ok, err := a.Add(lc)
		if err != nil {
			t.Fatalf("assembler on %q: %v", alias, err)
		}
		if ok {
			if done || s != want {
				t.Fatalf("assembler on %q gives %q again %t", alias, s, done)
			}
			done = true
		}
	}
	if !done {
		t.Fatalf("assembler never completes %q", alias)
	}
}

func FuzzSysCode(f *testing.F) {
	sysCodeSeeds(func(v uint16, model uint8, net, site uint16, par uint8, id uint32) {
		f.Add(v, model, net, site, par, id)
	})
	f.Fuzz(fuzzSysCode)
}

func sysCodeSeeds(add func(v uint16, model uint8, net, site uint16, par uint8, id uint32)) {
	add(uint16(0x8E21), uint8(2), uint16(5), uint16(0x55), uint8(1), uint32(3120101))
	add(uint16(0xFFFF), uint8(4), uint16(0x3FF), uint16(0x1000), uint8(4), uint32(1<<24))
}

func fuzzSysCode(t *testing.T, v uint16, model uint8, net, site uint16, par uint8, id uint32) {
	var s pdu.SysCode
	s.Unpack(v)
	if p, err := s.Pack(); err != nil || p != v {
		t.Fatalf("Pack(Unpack(%04x)) = %04x %v", v, p, err)
	}
	var c pdu.SysCode
	if err := c.Unpack14(v); (err == nil) != (v>>14 == 0) {
		t.Fatalf("Unpack14(%04x): %v", v, err)
	}
	if v>>14 == 0 {
		if p, err := c.Pack14(); err != nil || p != v || c.Par != 0 {
			t.Fatalf("Pack14(Unpack14(%04x)) = %04x %v", v, p, err)
		}
	}

	in := pdu.SysCode{Model: pdu.NetworkModel(model), Net: net, Site: site, Par: par}
	netBits, nerr := in.Model.NetBits()
	siteBits, serr := in.Model.SiteBits()
	fits := model < 4 && nerr == nil && serr == nil && netBits+siteBits == 12 &&
		int(net) < 1<<netBits && int(site) < 1<<siteBits && par < 4
	p, err := in.Pack()
	if (err == nil) != fits {
		t.Fatalf("Pack(%+v) = %v, fits %t", in, err, fits)
	}
	if fits {
		var back pdu.SysCode
		back.Unpack(p)
		if back != in {
			t.Fatalf("Unpack(Pack(%+v)) = %+v", in, back)
		}
		var cp pdu.ShortLCCSysParms
		var pp pdu.ShortLCPSysParms
		noErr(cp.SetSysCode(in))
		noErr(pp.SetSysCode(in))
		in.Par = 0
		if got := must(cp.SysCode()); got != in {
			t.Fatalf("C_SYS_Parms SysCode %+v, want %+v", got, in)
		}
		if got := must(pp.SysCode()); got != in {
			t.Fatalf("P_SYS_Parms SysCode %+v, want %+v", got, in)
		}
	}

	h, err := pdu.AddressHash(id)
	if (err == nil) != (id < 1<<24) {
		t.Fatalf("AddressHash(%x): %v", id, err)
	}
	if err == nil {
		var b [24]unpacked.Bit
		unpacked.PutUint(b[:], uint64(id))
		if want := fecrefcrc.CRC8(b[:]); h != want {
			t.Fatalf("AddressHash(%06x) = %02x, reference %02x", id, h, want)
		}
	}
}
