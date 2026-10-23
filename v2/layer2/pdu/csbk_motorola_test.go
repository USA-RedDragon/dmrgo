package pdu_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	motoRadioCheckRequest = "a41000002febf630b43c0637"
	motoRadioCheckAnswer  = "a41000802febf630b43cadce"
	motoCallAlert         = "9f10000030b43c2febf62dbe"
	motoCallAlertAck      = "a0109f002febf630b43cb2f5"
)

func motoBytes(t *testing.T, s string) [12]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 12 {
		t.Fatalf("vector %q: %v", s, err)
	}
	return [12]byte(b)
}

func TestMotorolaRadioCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		vector         string
		answer         bool
		target, source uint32
	}{
		{"request", motoRadioCheckRequest, false, 3191868, 3140598},
		{"answer", motoRadioCheckAnswer, true, 3140598, 3191868},
	}
	for _, tt := range tests {
		raw := motoBytes(t, tt.vector)
		c := pdu.DecodeCSBK(raw)
		if c.FEC.Uncorrectable || c.FID != enums.FIDMotorola || c.Opcode != pdu.CSBKMotorolaRadioCheck || !c.LastBlock {
			t.Fatalf("%s: decoded %+v", tt.name, c)
		}
		p, ok := c.AsMotorolaRadioCheck()
		if !ok || p.Answer != tt.answer || p.RequesterAddress != 3140598 || p.CheckedAddress != 3191868 {
			t.Fatalf("%s: payload %+v %t", tt.name, p, ok)
		}
		if tgt, src, ok := p.Addresses(); !ok || tgt != tt.target || src != tt.source {
			t.Errorf("%s: Addresses %d %d %t, want %d %d", tt.name, tgt, src, ok, tt.target, tt.source)
		}
		if _, ok := c.AsMotorolaCallAlert(); ok {
			t.Errorf("%s: AsMotorolaCallAlert ok", tt.name)
		}

		built := pdu.CSBK{LastBlock: true}
		if err := built.SetMotorolaRadioCheck(pdu.MotorolaRadioCheckPDU{Answer: tt.answer, RequesterAddress: 3140598, CheckedAddress: 3191868}); err != nil {
			t.Fatal(err)
		}
		if got := must(built.Encode()); got != raw {
			t.Errorf("%s: built %x, radio sent %x", tt.name, got, raw)
		}
	}
}

func TestMotorolaRadioCheckNil(t *testing.T) {
	t.Parallel()

	var p *pdu.MotorolaRadioCheckPDU
	if tgt, src, ok := p.Addresses(); ok || tgt != 0 || src != 0 {
		t.Errorf("nil Addresses %d %d %t", tgt, src, ok)
	}
}

func TestMotorolaCallAlert(t *testing.T) {
	t.Parallel()

	raw := motoBytes(t, motoCallAlert)
	c := pdu.DecodeCSBK(raw)
	p, ok := c.AsMotorolaCallAlert()
	if c.FEC.Uncorrectable || !ok || p.TargetAddress != 3191868 || p.SourceAddress != 3140598 {
		t.Fatalf("decoded %+v %+v %t", c, p, ok)
	}
	if _, ok := c.AsMotorolaCallAlertAck(); ok {
		t.Error("AsMotorolaCallAlertAck ok")
	}
	built := pdu.CSBK{LastBlock: true}
	if err := built.SetMotorolaCallAlert(pdu.MotorolaCallAlertPDU{TargetAddress: 3191868, SourceAddress: 3140598}); err != nil {
		t.Fatal(err)
	}
	if got := must(built.Encode()); got != raw {
		t.Errorf("built %x, radio sent %x", got, raw)
	}
}

func TestMotorolaCallAlertAck(t *testing.T) {
	t.Parallel()

	raw := motoBytes(t, motoCallAlertAck)
	c := pdu.DecodeCSBK(raw)
	p, ok := c.AsMotorolaCallAlertAck()
	if c.FEC.Uncorrectable || !ok || p.Request != 0x9F || p.TargetAddress != 3140598 || p.SourceAddress != 3191868 {
		t.Fatalf("decoded %+v %+v %t", c, p, ok)
	}
	alert := motoBytes(t, motoCallAlert)
	if p.Request != alert[0] {
		t.Errorf("Request %#x, want octet 0 of the call alert %#x", p.Request, alert[0])
	}
	built := pdu.CSBK{LastBlock: true}
	if err := built.SetMotorolaCallAlertAck(pdu.MotorolaCallAlertAckPDU{Request: 0x9F, TargetAddress: 3140598, SourceAddress: 3191868}); err != nil {
		t.Fatal(err)
	}
	if got := must(built.Encode()); got != raw {
		t.Errorf("built %x, radio sent %x", got, raw)
	}
}

func TestMotorolaCSBKOtherFID(t *testing.T) {
	t.Parallel()

	raw := motoBytes(t, motoRadioCheckRequest)
	raw[1] = 0
	c := pdu.DecodeCSBK(raw)
	if _, ok := c.AsMotorolaRadioCheck(); ok {
		t.Error("standard FID: AsMotorolaRadioCheck ok")
	}
	var s pdu.CSBK
	if err := s.SetMotorolaCallAlert(pdu.MotorolaCallAlertPDU{TargetAddress: 1 << 24}); err == nil {
		t.Error("25-bit address: no error")
	}
	if s.Opcode != 0 || s.FID != 0 {
		t.Errorf("failed Set changed the CSBK: %+v", s)
	}
}

func TestMotorolaCSBKString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vector, name, view string
	}{
		{motoRadioCheckRequest, "Opcode: Motorola Radio Check (FID 0x10),", "MotorolaRadioCheckPDU{"},
		{motoRadioCheckAnswer, "Opcode: Motorola Radio Check (FID 0x10),", "MotorolaRadioCheckPDU{"},
		{motoCallAlert, "Opcode: Motorola Call Alert (FID 0x10),", "MotorolaCallAlertPDU{"},
		{motoCallAlertAck, "Opcode: Motorola Call Alert Ack (FID 0x10),", "MotorolaCallAlertAckPDU{"},
	}
	for _, tt := range tests {
		c := pdu.DecodeCSBK(motoBytes(t, tt.vector))
		if s := c.String(); !strings.Contains(s, tt.name) || !strings.Contains(s, tt.view) {
			t.Errorf("%s: %s", tt.vector, s)
		}
	}
}

func TestCSBKStringOpcodeNameFollowsFID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		op   pdu.CSBKOpcode
		fid  enums.FID
		name string
	}{
		{pdu.CSBKRandomAccess, 0, "Opcode: C_RAND PDU,"},
		{pdu.CSBKAckOutbound, 0, "Opcode: C_ACKD PDU,"},
		{pdu.CSBKMaintenance, 0, "Opcode: P_MAINT PDU,"},
		{pdu.CSBKPreamble, 0, "Opcode: Preamble PDU,"},
		{pdu.CSBKCapacityPlusSiteStatus, 0, "Opcode: Unknown CSBKOpcode: 00111110,"},
		{pdu.CSBKCapacityPlusNeighbors, 0, "Opcode: Unknown CSBKOpcode: 00111011,"},
		{pdu.CSBKCapacityPlusDataRevertAnnouncement, 0, "Opcode: Unknown CSBKOpcode: 00101001,"},
		{pdu.CSBKMaintenance, enums.FIDMotorola, "Opcode: Capacity Plus Data Revert Grant (FID 0x10),"},
		{pdu.CSBKPreamble, enums.FIDMotorola, "Opcode: Capacity Plus Preamble (FID 0x10),"},
		{pdu.CSBKCapacityPlusSiteStatus, enums.FIDMotorola, "Opcode: Capacity Plus Site Status (FID 0x10),"},
		{pdu.CSBKAloha, enums.FIDMotorola, "Opcode: Unknown CSBKOpcode: 00011001,"},
		{pdu.CSBKRandomAccess, enums.FIDHYTScienceTech2, "Opcode: Unknown CSBKOpcode: 00011111,"},
	}
	for _, tt := range tests {
		c := pdu.CSBK{Opcode: tt.op, FID: tt.fid}
		if s := c.String(); !strings.Contains(s, tt.name) {
			t.Errorf("opcode %#x FID %#x: %s", byte(tt.op), byte(tt.fid), s)
		}
		m := pdu.MBCHeader{Opcode: tt.op, FID: tt.fid}
		if s := m.String(); !strings.Contains(s, tt.name) {
			t.Errorf("MBC opcode %#x FID %#x: %s", byte(tt.op), byte(tt.fid), s)
		}
	}
}
