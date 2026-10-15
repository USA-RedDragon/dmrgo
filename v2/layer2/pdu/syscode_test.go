package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSysCode_PackUnpack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code pdu.SysCode
		want uint16
	}{
		{pdu.SysCode{Model: pdu.NetworkModelTiny, Net: 0x1FF, Site: 0x7, Par: 1}, 0b00_111111111_111_01},
		{pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 0x55, Site: 0x0A, Par: 2}, 0b01_1010101_01010_10},
		{pdu.SysCode{Model: pdu.NetworkModelLarge, Net: 0x9, Site: 0xA5, Par: 3}, 0b10_1001_10100101_11},
		{pdu.SysCode{Model: pdu.NetworkModelHuge, Net: 0x2, Site: 0x3FF, Par: 0}, 0b11_10_1111111111_00},
	}
	for _, tt := range tests {
		got, err := tt.code.Pack()
		if err != nil || got != tt.want {
			t.Errorf("%+v: Pack = %016b, %v; want %016b", tt.code, got, err, tt.want)
		}
		if back := pdu.UnpackSysCode(got); back != tt.code {
			t.Errorf("UnpackSysCode(%016b) = %+v", got, back)
		}
		got14, err := tt.code.Pack14()
		if err != nil || got14 != tt.want>>2 {
			t.Errorf("%+v: Pack14 = %014b, %v", tt.code, got14, err)
		}
		noPar := tt.code
		noPar.Par = 0
		if back := pdu.UnpackSysCode14(got14); back != noPar {
			t.Errorf("UnpackSysCode14(%014b) = %+v", got14, back)
		}
	}
}

func TestSysCode_Overflow(t *testing.T) {
	t.Parallel()
	for _, c := range []pdu.SysCode{
		{Model: 4},
		{Model: pdu.NetworkModelTiny, Net: 0x200},
		{Model: pdu.NetworkModelTiny, Site: 8},
		{Model: pdu.NetworkModelSmall, Net: 0x80},
		{Model: pdu.NetworkModelLarge, Site: 0x100},
		{Model: pdu.NetworkModelHuge, Net: 4},
		{Model: pdu.NetworkModelHuge, Site: 0x400},
		{Par: 4},
	} {
		if _, err := c.Pack(); err == nil {
			t.Errorf("%+v: expected error", c)
		}
	}
}

func TestSysCode_ShortLCSysParms(t *testing.T) {
	t.Parallel()
	code := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 0x42, Site: 0x13}
	var c pdu.ShortLCCSysParms
	if err := c.SetSysCode(code); err != nil {
		t.Fatal(err)
	}
	if c.NetID() != code.Net || c.SiteID() != code.Site || c.SysCode() != code {
		t.Errorf("C_SYS_Parms: net %d site %d code %+v", c.NetID(), c.SiteID(), c.SysCode())
	}
	slc := pdu.ShortLC{SLCO: enums.SLCOCSysParms, CSysParms: &c}
	bits, err := pdu.EncodeShortLC(&slc)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := pdu.DecodeShortLC(bits)
	if got.CSysParms == nil || got.CSysParms.SysCode() != code {
		t.Errorf("decoded %s", got.String())
	}

	var p pdu.ShortLCPSysParms
	if err := p.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelHuge, Net: 3, Site: 0x2AA, Par: 3}); err != nil {
		t.Fatal(err)
	}
	if p.NetID() != 3 || p.SiteID() != 0x2AA || p.SysCode().Par != 0 {
		t.Errorf("P_SYS_Parms: net %d site %d", p.NetID(), p.SiteID())
	}
	if err := p.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelHuge, Net: 4}); err == nil {
		t.Error("expected error")
	}
	if err := c.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelTiny, Site: 9}); err == nil {
		t.Error("expected error")
	}
}

func TestSysCode_AlohaField(t *testing.T) {
	t.Parallel()
	code := pdu.SysCode{Model: pdu.NetworkModelLarge, Net: 5, Site: 200, Par: 2}
	v, err := code.Pack()
	if err != nil {
		t.Fatal(err)
	}
	a := pdu.AlohaPDU{SysIdentCode: v}
	if pdu.UnpackSysCode(a.SysIdentCode) != code {
		t.Errorf("got %+v", pdu.UnpackSysCode(a.SysIdentCode))
	}
	if pdu.NetworkModelTiny.NetBits() != 9 || pdu.NetworkModelHuge.SiteBits() != 10 {
		t.Error("model widths")
	}
}
