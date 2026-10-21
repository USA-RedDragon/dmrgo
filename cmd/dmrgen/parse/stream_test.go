package parse_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

func TestParseRejectsBadStreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"unknown kind", "// dmr:stream\ntype T struct {\n A uint8 " + tag("u7") + "\n}", "unknown stream kind"},
		{"narrow type", "// dmr:stream\ntype T struct {\n A uint8 " + tag("u16") + "\n}", "narrower than 2 octets"},
		{"bytes type", "// dmr:stream\ntype T struct {\n A [3]byte " + tag("lv8") + "\n}", "need []byte"},
		{"array size", "// dmr:stream\ntype T struct {\n A [3]byte " + tag("bytes:4") + "\n}", "needs [4]byte"},
		{"rest not last", "// dmr:stream\ntype T struct {\n A []byte " + tag("rest") + "\n B uint8 " + tag("u8") + "\n}", "only conditional fields may follow"},
		{"if before", "// dmr:stream\ntype T struct {\n A uint8 " + tag("u8,if:B") + "\n B uint8 " + tag("u8") + "\n}", "must name a field before it"},
		{"token outside tokens", "// dmr:stream\ntype T struct {\n A uint8 " + tag("u8,token:0x22") + "\n}", "token needs dmr:tokens"},
		{"token without Has", "// dmr:tokens\ntype T struct {\n A uint8 " + tag("u8,token:0x22") + "\n}", "needs a dmr:\"-\" bool field HasA"},
		{"duplicate token", "// dmr:tokens\ntype T struct {\n A bool " + tag("flag,token:0x22") + "\n B bool " + tag("flag,token:0x22") + "\n}", "same token"},
		{"two lengths", "// dmr:stream\ntype T struct {\n A uint16 " + tag("u16,length") + "\n B uint16 " + tag("u16,length") + "\n}", "at most one length"},
		{"length signed", "// dmr:stream\ntype T struct {\n A int16 " + tag("i16,length") + "\n}", "length cannot be signed"},
		{"scale type", "// dmr:stream\ntype T struct {\n A int32 " + tag("i32,scale:90/31") + "\n}", "scale needs a float64"},
		{"float varint type", "// dmr:stream\ntype T struct {\n A uint8 " + tag("ufloatvar") + "\n}", "float varint needs float64"},
		{"text on int", "// dmr:stream\ntype T struct {\n A uint8 " + tag("u8,utf16le") + "\n}", "need a byte string kind"},
		{"bound path", "// dmr:stream\n// dmr:bound A.Len\ntype T struct {\n A uint8 " + tag("u8") + "\n}", "must name a field of a frame field"},
		{"len expr", "// dmr:stream\ntype T struct {\n A []byte " + tag("len:1x") + "\n}", "is not a field path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v does not contain %q", err, tt.want)
			}
		})
	}
}

func TestParseStream(t *testing.T) {
	t.Parallel()

	pdus := mustParse(t, "// dmr:stream trailing\ntype T struct {\n L uint16 "+tag("u16,length")+"\n A uint32 "+tag("uintvar")+
		"\n F float64 "+tag("i32,scale:90/31")+"\n S []byte "+tag("lv8,utf16le")+"\n O []byte "+tag("len:A*4-20,if:A==7|8")+"\n R []byte "+tag("rest")+"\n}")
	st := pdus[0].Stream
	if st == nil || !st.Trailing || len(st.Fields) != 6 {
		t.Fatalf("stream %+v", st)
	}
	f := st.Fields
	switch {
	case f[0].SKind != parse.StreamInt || f[0].Octets != 2 || f[0].Bound != parse.BoundAfter:
		t.Errorf("L %+v", f[0])
	case f[1].SKind != parse.StreamUintvar:
		t.Errorf("A %+v", f[1])
	case f[2].SKind != parse.StreamFixed || !f[2].SignedInt || f[2].ScaleNum != 90 || f[2].ScaleExp != 31:
		t.Errorf("F %+v", f[2])
	case f[3].SKind != parse.StreamBytes || f[3].Length != parse.LengthPrefix || f[3].Octets != 1 || f[3].Text != parse.TextUTF16LE:
		t.Errorf("S %+v", f[3])
	case f[4].Expr == nil || f[4].Expr.Path != "A" || f[4].Expr.Mul != 4 || f[4].Expr.Add != -20 || f[4].If == nil || len(f[4].If.Values) != 2:
		t.Errorf("O %+v", f[4])
	case f[5].Length != parse.LengthRest:
		t.Errorf("R %+v", f[5])
	}
}
