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
		var back pdu.SysCode
		if back.Unpack(got); back != tt.code {
			t.Errorf("Unpack(%016b) = %+v", got, back)
		}
		got14, err := tt.code.Pack14()
		if err != nil || got14 != tt.want>>2 {
			t.Errorf("%+v: Pack14 = %014b, %v", tt.code, got14, err)
		}
		noPar := tt.code
		noPar.Par = 0
		if err := back.Unpack14(got14); err != nil || back != noPar {
			t.Errorf("Unpack14(%014b) = %+v, %v", got14, back, err)
		}
		before := back
		if err := back.Unpack14(got14 | 0x4000); !isRangeError(err, "SysCode", "Unpack14", 14) || back != before {
			t.Errorf("Unpack14 of 15 bits: %+v, %v", back, err)
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
	if must(c.NetID()) != code.Net || must(c.SiteID()) != code.Site || must(c.SysCode()) != code {
		t.Errorf("C_SYS_Parms: net %d site %d code %+v", must(c.NetID()), must(c.SiteID()), must(c.SysCode()))
	}
	slc := pdu.ShortLC{SLCO: enums.SLCOCSysParms, CSysParms: &c}
	bits, err := slc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got := pdu.DecodeShortLC(bits)
	if got.CSysParms == nil || must(got.CSysParms.SysCode()) != code {
		t.Errorf("decoded %s", got.String())
	}

	var p pdu.ShortLCPSysParms
	if err := p.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelHuge, Net: 3, Site: 0x2AA, Par: 3}); err != nil {
		t.Fatal(err)
	}
	if must(p.NetID()) != 3 || must(p.SiteID()) != 0x2AA || must(p.SysCode()).Par != 0 {
		t.Errorf("P_SYS_Parms: net %d site %d", must(p.NetID()), must(p.SiteID()))
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
	var got pdu.SysCode
	if got.Unpack(a.SysIdentCode); got != code {
		t.Errorf("got %+v", got)
	}
	if must(pdu.NetworkModelTiny.NetBits()) != 9 || must(pdu.NetworkModelHuge.SiteBits()) != 10 {
		t.Error("model widths")
	}
}

func TestSysCode_ShortLCGettersRejectOutOfRange(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, typ, field string
		bits             int
		get              func() (pdu.SysCode, error)
	}{
		{"C MODEL", "ShortLCCSysParms", "MODEL", 2, (&pdu.ShortLCCSysParms{MODEL: 4}).SysCode},
		{"C NetSiteRaw", "ShortLCCSysParms", "NetSiteRaw", 12, (&pdu.ShortLCCSysParms{NetSiteRaw: 0x1000}).SysCode},
		{"P MODEL", "ShortLCPSysParms", "MODEL", 2, (&pdu.ShortLCPSysParms{MODEL: 4}).SysCode},
		{"P NetSiteRaw", "ShortLCPSysParms", "NetSiteRaw", 12, (&pdu.ShortLCPSysParms{NetSiteRaw: 0x1000}).SysCode},
	} {
		if _, err := tt.get(); !isRangeError(err, tt.typ, tt.field, tt.bits) {
			t.Errorf("%s: %v, want the FieldRangeError of %s.%s", tt.name, err, tt.typ, tt.field)
		}
	}
}

func TestSysCode_NetSiteGettersRejectOutOfRange(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, typ, field string
		bits             int
		get              func() (uint16, error)
	}{
		{"C NetID MODEL", "ShortLCCSysParms", "MODEL", 2, (&pdu.ShortLCCSysParms{MODEL: 4}).NetID},
		{"C SiteID NetSiteRaw", "ShortLCCSysParms", "NetSiteRaw", 12, (&pdu.ShortLCCSysParms{NetSiteRaw: 0x1FFF}).SiteID},
		{"P NetID NetSiteRaw", "ShortLCPSysParms", "NetSiteRaw", 12, (&pdu.ShortLCPSysParms{NetSiteRaw: 0x1FFF}).NetID},
		{"P SiteID MODEL", "ShortLCPSysParms", "MODEL", 2, (&pdu.ShortLCPSysParms{MODEL: 7}).SiteID},
	} {
		if _, err := tt.get(); !isRangeError(err, tt.typ, tt.field, tt.bits) {
			t.Errorf("%s: %v, want the FieldRangeError of %s.%s", tt.name, err, tt.typ, tt.field)
		}
	}
	if _, err := pdu.NetworkModel(4).NetBits(); !isRangeError(err, "pdu", "NetworkModel", 2) {
		t.Errorf("NetBits of model 4: %v", err)
	}
	if _, err := pdu.NetworkModel(7).SiteBits(); !isRangeError(err, "pdu", "NetworkModel", 2) {
		t.Errorf("SiteBits of model 7: %v", err)
	}
}
