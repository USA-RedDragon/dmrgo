package dmrd_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestHBRPSpecExamples(t *testing.T) {
	t.Parallel()

	token, _ := hex.DecodeString("cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85")
	tests := []struct {
		p    string
		want dmrd.Control
	}{
		{"RPTL00040178", dmrd.Control{Command: dmrd.CommandRPTL, ID: 0x00040178}},
		{"MSTNAK00040178", dmrd.Control{Command: dmrd.CommandMSTNAK, ID: 0x00040178}},
		{"MSTACK000401780A7ED498", dmrd.Control{Command: dmrd.CommandMSTACK, ID: 0x00040178, Salt: 0x0A7ED498, HasSalt: true}},
		{"RPTK00040178cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85", dmrd.Control{Command: dmrd.CommandRPTK, ID: 0x00040178, Token: [32]byte(token)}},
		{"MSTACK00040178", dmrd.Control{Command: dmrd.CommandMSTACK, ID: 0x00040178}},
		{"MSTPING00040178", dmrd.Control{Command: dmrd.CommandMSTPING, ID: 0x00040178}},
		{"RPTPONG00040178", dmrd.Control{Command: dmrd.CommandRPTPONG, ID: 0x00040178}},
		{"MSTCL00040178", dmrd.Control{Command: dmrd.CommandMSTCL, ID: 0x00040178}},
		{"RPTCL00040178", dmrd.Control{Command: dmrd.CommandRPTCL, ID: 0x00040178}},
		{"RPTSBKN01234567", dmrd.Control{Command: dmrd.CommandRPTSBKN, ID: 0x01234567}},
		{"RPTRSSI01234567:1-120.0", dmrd.Control{Command: dmrd.CommandRPTRSSI, ID: 0x01234567, Text: []byte(":1-120.0")}},
		{"RPTINTR01234567:2", dmrd.Control{Command: dmrd.CommandRPTINTR, ID: 0x01234567, Text: []byte(":2")}},
	}
	for _, tt := range tests {
		got, err := dmrd.DecodeControl([]byte(tt.p), dmrd.DialectHBRP)
		if err != nil || !equalControl(got, tt.want) {
			t.Errorf("%s: %+v %v", tt.p, got, err)
			continue
		}
		out, err := got.AppendBinary(nil, dmrd.DialectHBRP)
		if err != nil || !bytes.EqualFold(out, []byte(tt.p)) {
			t.Errorf("%s: encode %q %v", tt.p, out, err)
		}
	}
}

func equalControl(a, b dmrd.Control) bool {
	return a.Command == b.Command && a.ID == b.ID && a.Salt == b.Salt && a.HasSalt == b.HasSalt && a.Token == b.Token && bytes.Equal(a.Text, b.Text)
}

func bin(tag string, v ...uint32) []byte {
	p := []byte(tag)
	for _, x := range v {
		p = append(p, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	}
	return p
}

func TestMMDVMControl(t *testing.T) {
	t.Parallel()

	var token [32]byte
	for i := range token {
		token[i] = byte(i)
	}
	tests := []struct {
		p    []byte
		want dmrd.Control
	}{
		{bin("RPTL", 3149999), dmrd.Control{Command: dmrd.CommandRPTL, ID: 3149999}},
		{bin("RPTACK", 0xDEADBEEF), dmrd.Control{Command: dmrd.CommandRPTACK, ID: 0xDEADBEEF}},
		{append(bin("RPTK", 3149999), token[:]...), dmrd.Control{Command: dmrd.CommandRPTK, ID: 3149999, Token: token}},
		{bin("RPTPING", 3149999), dmrd.Control{Command: dmrd.CommandRPTPING, ID: 3149999}},
		{bin("MSTPONG", 3149999), dmrd.Control{Command: dmrd.CommandMSTPONG, ID: 3149999}},
		{bin("RPTCL", 3149999), dmrd.Control{Command: dmrd.CommandRPTCL, ID: 3149999}},
		{bin("MSTCL", 3149999), dmrd.Control{Command: dmrd.CommandMSTCL, ID: 3149999}},
		{bin("MSTNAK", 3149999), dmrd.Control{Command: dmrd.CommandMSTNAK, ID: 3149999}},
		{bin("RPTSBKN", 3149999), dmrd.Control{Command: dmrd.CommandRPTSBKN, ID: 3149999}},
		{append(bin("RPTO", 3149999), "StartRef=4013;"...), dmrd.Control{Command: dmrd.CommandRPTO, ID: 3149999, Text: []byte("StartRef=4013;")}},
		{append(bin("RPTG", 3149999), "+30.1234-097.1234"...), dmrd.Control{Command: dmrd.CommandRPTG, ID: 3149999, Text: []byte("+30.1234-097.1234")}},
	}
	for _, tt := range tests {
		got, err := dmrd.DecodeControl(tt.p, dmrd.DialectMMDVM)
		if err != nil || !equalControl(got, tt.want) {
			t.Errorf("%q: %+v %v", tt.p, got, err)
			continue
		}
		out, err := got.AppendBinary(nil, dmrd.DialectMMDVM)
		if err != nil || !bytes.Equal(out, tt.p) {
			t.Errorf("%q: encode %q %v", tt.p, out, err)
		}
	}
}

func TestLocalControl(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		p    string
		want dmrd.Control
	}{
		{"DMRP", dmrd.Control{Command: dmrd.CommandDMRP}},
		{"DMRB", dmrd.Control{Command: dmrd.CommandDMRB}},
		{"DMRT\x01\x81", dmrd.Control{Command: dmrd.CommandDMRT, Text: []byte{1, 0x81}}},
	} {
		got, err := dmrd.DecodeControl([]byte(tt.p), dmrd.DialectLocal)
		if err != nil || !equalControl(got, tt.want) {
			t.Errorf("%q: %+v %v", tt.p, got, err)
			continue
		}
		if out, err := got.AppendBinary(nil, dmrd.DialectLocal); err != nil || string(out) != tt.p {
			t.Errorf("%q: encode %q %v", tt.p, out, err)
		}
	}
	if _, err := dmrd.DecodeControl([]byte("DMRPX"), dmrd.DialectLocal); !errors.Is(err, dmrd.ErrLength) {
		t.Errorf("DMRPX: %v", err)
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		p    string
		d    dmrd.Dialect
		want dmrd.Command
	}{
		{"RPTCL0000", dmrd.DialectMMDVM, dmrd.CommandRPTCL},
		{"RPTC0000", dmrd.DialectMMDVM, dmrd.CommandRPTC},
		{"RPTPING", dmrd.DialectMMDVM, dmrd.CommandRPTPING},
		{"RPTPONG", dmrd.DialectMMDVM, dmrd.CommandUnknown},
		{"RPTPONG", dmrd.DialectHBRP, dmrd.CommandRPTPONG},
		{"RPTACK", dmrd.DialectHBRP, dmrd.CommandUnknown},
		{"MSTACK", dmrd.DialectHBRP, dmrd.CommandMSTACK},
		{"DMRA", dmrd.DialectHBRP, dmrd.CommandUnknown},
		{"DMRA", dmrd.DialectLocal, dmrd.CommandDMRA},
		{"DMRD", dmrd.DialectLocal, dmrd.CommandDMRD},
		{"DMRP", dmrd.DialectMMDVM, dmrd.CommandUnknown},
		{"BCKA", dmrd.DialectMMDVM, dmrd.CommandBCKA},
		{"DMRE", dmrd.DialectHBRP, dmrd.CommandDMRE},
		{"RPT", dmrd.DialectMMDVM, dmrd.CommandUnknown},
		{"RPTL", dmrd.DialectUnknown, dmrd.CommandUnknown},
		{"RPTC0000", dmrd.DialectUnknown, dmrd.CommandUnknown},
		{"DMRA", dmrd.DialectUnknown, dmrd.CommandUnknown},
		{"DMRD", dmrd.DialectUnknown, dmrd.CommandDMRD},
		{"BCKA", dmrd.DialectUnknown, dmrd.CommandBCKA},
		{"DMRD", dmrd.Dialect(9), dmrd.CommandUnknown},
	}
	for _, tt := range tests {
		if got := dmrd.Classify([]byte(tt.p), tt.d); got != tt.want {
			t.Errorf("%s %v: %v, want %v", tt.p, tt.d, got, tt.want)
		}
	}
}

func TestClassifyConfigOfCallsignStartingWithL(t *testing.T) {
	t.Parallel()

	hbrp := []byte("RPTCLU1ABC  0004017B")
	hbrp = append(hbrp, bytes.Repeat([]byte(" "), 306-len(hbrp))...)
	if got := dmrd.Classify(hbrp, dmrd.DialectHBRP); got != dmrd.CommandRPTC {
		t.Errorf("HBRP RPTC of LU1ABC: %v, want RPTC", got)
	}
	if id, ok := dmrd.RepeaterID(hbrp, dmrd.DialectHBRP); !ok || id != 0x0004017B {
		t.Errorf("HBRP RPTC of LU1ABC: ID %d %v, want %d", id, ok, 0x0004017B)
	}
	if got := dmrd.Classify([]byte("RPTCL0004017B"), dmrd.DialectHBRP); got != dmrd.CommandRPTCL {
		t.Errorf("HBRP RPTCL: %v, want RPTCL", got)
	}
	mmdvm := append([]byte("RPTC"), 0x4C, 0x00, 0x01, 0x7B)
	mmdvm = append(mmdvm, bytes.Repeat([]byte(" "), 302-len(mmdvm))...)
	if got := dmrd.Classify(mmdvm, dmrd.DialectMMDVM); got != dmrd.CommandRPTC {
		t.Errorf("MMDVM RPTC whose ID starts with L: %v, want RPTC", got)
	}
}

func TestLoginSalt(t *testing.T) {
	t.Parallel()

	ack, _ := dmrd.DecodeControl([]byte("RPTACK\x0a\x7e\xd4\x98"), dmrd.DialectMMDVM)
	if s, ok := ack.LoginSalt(); !ok || s != 0x0A7ED498 || ack.HasSalt {
		t.Errorf("MMDVM RPTACK salt %x %v %+v", s, ok, ack)
	}
	mst, _ := dmrd.DecodeControl([]byte("MSTACK000401780A7ED498"), dmrd.DialectHBRP)
	if s, ok := mst.LoginSalt(); !ok || s != 0x0A7ED498 {
		t.Errorf("HBRP MSTACK salt %x %v", s, ok)
	}
	short, _ := dmrd.DecodeControl([]byte("MSTACK00040178"), dmrd.DialectHBRP)
	if _, ok := short.LoginSalt(); ok {
		t.Error("HBRP MSTACK without a salt")
	}
	if _, ok := (&dmrd.Control{Command: dmrd.CommandRPTPING, ID: 1}).LoginSalt(); ok {
		t.Error("RPTPING salt")
	}
	var nilc *dmrd.Control
	if _, ok := nilc.LoginSalt(); ok {
		t.Error("nil Control salt")
	}
	out, err := (&dmrd.Control{Command: dmrd.CommandRPTACK, ID: 5, Salt: 0x0A7ED498, HasSalt: true}).AppendBinary(nil, dmrd.DialectMMDVM)
	if err != nil || string(out) != "RPTACK\x0a\x7e\xd4\x98" {
		t.Errorf("MMDVM RPTACK with HasSalt %q %v", out, err)
	}
}

func TestRepeaterID(t *testing.T) {
	t.Parallel()

	const id = 3129100
	data, _ := (&dmrd.Data{Repeater: id, Slot: 1}).AppendBinary(nil)
	alias, _ := (&dmrd.TalkerAlias{Repeater: id}).AppendBinary(nil)
	pos, _ := (&dmrd.Position{Repeater: id}).AppendBinary(nil)
	var cfg dmrd.MMDVMConfig
	cfg.Reset()
	cfg.SetID(id)
	var hcfg dmrd.HBRPConfig
	hcfg.Reset()
	hcfg.SetID(id)
	ping, _ := (&dmrd.Control{Command: dmrd.CommandRPTPING, ID: id}).AppendBinary(nil, dmrd.DialectMMDVM)
	opts, _ := (&dmrd.Control{Command: dmrd.CommandRPTO, ID: id, Text: []byte("TS2=91;")}).AppendBinary(nil, dmrd.DialectMMDVM)
	login, _ := (&dmrd.Control{Command: dmrd.CommandRPTL, ID: id}).AppendBinary(nil, dmrd.DialectHBRP)
	ack, _ := (&dmrd.Control{Command: dmrd.CommandRPTACK, ID: id}).AppendBinary(nil, dmrd.DialectMMDVM)
	bcka, _ := (&dmrd.BridgeControl{Command: dmrd.CommandBCKA}).AppendBinary(nil)
	tests := []struct {
		name string
		p    []byte
		d    dmrd.Dialect
		ok   bool
	}{
		{"DMRD", data, dmrd.DialectMMDVM, true},
		{"DMRD local", data, dmrd.DialectLocal, true},
		{"DMRA", alias, dmrd.DialectMMDVM, true},
		{"DMRA local", alias, dmrd.DialectLocal, false},
		{"DMRG", pos, dmrd.DialectMMDVM, true},
		{"RPTC", cfg[:], dmrd.DialectMMDVM, true},
		{"RPTC HBRP", hcfg[:], dmrd.DialectHBRP, true},
		{"RPTPING", ping, dmrd.DialectMMDVM, true},
		{"RPTO", opts, dmrd.DialectMMDVM, true},
		{"RPTL HBRP", login, dmrd.DialectHBRP, true},
		{"RPTACK", ack, dmrd.DialectMMDVM, false},
		{"BCKA", bcka, dmrd.DialectMMDVM, false},
		{"short DMRD", data[:20], dmrd.DialectMMDVM, false},
		{"RPTL of HBRP in MMDVM", login, dmrd.DialectMMDVM, false},
		{"empty", nil, dmrd.DialectMMDVM, false},
	}
	for _, tt := range tests {
		got, ok := dmrd.RepeaterID(tt.p, tt.d)
		if ok != tt.ok || (ok && got != id) || (!ok && got != 0) {
			t.Errorf("%s: %d, %v; want ok %v", tt.name, got, ok, tt.ok)
		}
	}
}

func TestControlErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		p    []byte
		d    dmrd.Dialect
		want error
	}{
		{bin("DMRD", 1), dmrd.DialectMMDVM, dmrd.ErrCommand},
		{[]byte("RPTL0004"), dmrd.DialectMMDVM, nil},
		{[]byte("RPTL00040178"), dmrd.DialectMMDVM, dmrd.ErrLength},
		{[]byte("RPTL0004017"), dmrd.DialectHBRP, dmrd.ErrShort},
		{[]byte("RPTL0004017G"), dmrd.DialectHBRP, dmrd.ErrText},
		{[]byte("RPTO000"), dmrd.DialectMMDVM, dmrd.ErrShort},
		{[]byte("RPTRSSI0123456"), dmrd.DialectHBRP, dmrd.ErrShort},
		{[]byte("MSTACK0004017800"), dmrd.DialectHBRP, dmrd.ErrShort},
		{[]byte("nonsense"), dmrd.DialectMMDVM, dmrd.ErrCommand},
	}
	for _, tt := range tests {
		if _, err := dmrd.DecodeControl(tt.p, tt.d); !errors.Is(err, tt.want) {
			t.Errorf("%q: %v, want %v", tt.p, err, tt.want)
		}
	}
}

func TestControlEncodeMisuse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		c dmrd.Control
		d dmrd.Dialect
	}{
		{dmrd.Control{Command: dmrd.CommandMSTACK}, dmrd.DialectMMDVM},
		{dmrd.Control{Command: dmrd.CommandRPTACK}, dmrd.DialectHBRP},
		{dmrd.Control{Command: dmrd.CommandDMRD}, dmrd.DialectMMDVM},
		{dmrd.Control{Command: dmrd.CommandRPTL, Text: []byte("x")}, dmrd.DialectMMDVM},
		{dmrd.Control{Command: dmrd.CommandRPTL}, dmrd.DialectUnknown},
		{dmrd.Control{Command: dmrd.CommandRPTL}, dmrd.Dialect(9)},
	}
	for _, tt := range tests {
		if out, err := tt.c.AppendBinary([]byte{1}, tt.d); err == nil || !bytes.Equal(out, []byte{1}) {
			t.Errorf("%+v %v: %q %v", tt.c, tt.d, out, err)
		}
	}
}

func TestCommandAndDialectStrings(t *testing.T) {
	t.Parallel()

	if dmrd.CommandRPTSBKN.String() != "RPTSBKN" || dmrd.CommandUnknown.String() != "Unknown" || dmrd.Command(200).String() != "Command(200)" {
		t.Error("Command.String")
	}
	if dmrd.DialectHBRP.String() != "HBRP" || dmrd.Dialect(9).String() != "Dialect(9)" {
		t.Error("Dialect.String")
	}
	if dmrd.FrameVoiceSync.String() != "Voice Sync" || dmrd.FrameType(9).String() != "FrameType(9)" {
		t.Error("FrameType.String")
	}
}
