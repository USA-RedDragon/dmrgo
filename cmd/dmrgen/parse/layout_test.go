package parse_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

func tag(s string) string { return "`dmr:\"" + s + "\"`" }

func TestParseRejectsBadLayouts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"le outside a frame", "type T struct {\n A uint16 " + tag("bits:0-15,le") + "\n}", "require dmr:frame or dmr:view"},
		{"magic outside a frame", "// dmr:magic 0-7=\"A\"\ntype T struct {\n A uint8 " + tag("bits:8-15") + "\n}", "requires dmr:frame or dmr:view"},
		{"le not octets", "// dmr:frame 2\ntype T struct {\n R uint8 " + tag("bits:0-3") + "\n A uint16 " + tag("bits:4-15,le") + "\n}", "whole octets"},
		{"dec too wide for the type", "// dmr:frame 3\ntype T struct {\n A uint8 " + tag("bits:0-23,dec") + "\n}", "cannot hold 3 decimal digits"},
		{"hex too wide for the type", "// dmr:frame 3\ntype T struct {\n A uint8 " + tag("bits:0-23,hex") + "\n}", "cannot hold 3 hex digits"},
		{"dec on bool", "// dmr:frame 1\ntype T struct {\n A bool " + tag("bit:0") + "\n B uint8 " + tag("bits:1-7") + "\n}", ""},
		{"swap16 odd", "// dmr:frame 3\ntype T struct {\n A [3]byte " + tag("bits:0-23,packed,swap16") + "\n}", "even number"},
		{"text in a frame", "// dmr:frame 2\ntype T struct {\n A [2]byte " + tag("bits:0-15,packed,text") + "\n}", "text requires dmr:view"},
		{"nulzero without dec", "// dmr:frame 1\ntype T struct {\n A uint8 " + tag("bits:0-7,nulzero") + "\n}", "nulzero requires a dec"},
		{"bias with dec", "// dmr:frame 2\ntype T struct {\n A uint8 " + tag("bits:0-15,dec,bias:1") + "\n}", "bias cannot be combined"},
		{"two sizes without Size", "// dmr:frame 1 2\ntype T struct {\n A uint8 " + tag("bits:0-7") + "\n B uint8 " + tag("bits:8-15") + "\n}", "integer field Size"},
		{"field crosses a size", "// dmr:frame 1 2\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint16 " + tag("bits:0-15") + "\n}", "crosses the end of a 1-octet frame"},
		{"tail cut by a size", "// dmr:frame 1 3\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B [2]byte " + tag("bits:8-23,packed,tail") + "\n}", ""},
		{"tail not at the end", "// dmr:frame 1 3\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B [1]byte " + tag("bits:8-15,packed,tail") + "\n C uint8 " + tag("bits:16-23") + "\n}", "tail needs"},
		{"tail not packed", "// dmr:frame 1 3\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B uint16 " + tag("bits:8-23,tail") + "\n}", "tail needs"},
		{"field beyond the frame", "// dmr:frame 1\ntype T struct {\n A uint16 " + tag("bits:0-15") + "\n}", "beyond 1 octets"},
		{"uncovered octets", "// dmr:frame 2\ntype T struct {\n A uint8 " + tag("bits:0-7") + "\n}", "uncovered bits 8-15"},
		{"magic not text", "// dmr:frame 3\n// dmr:magic 0-15=AB\ntype T struct {\n A uint8 " + tag("bits:16-23") + "\n}", "not quoted text or 0x hex"},
		{"magic wrong length", "// dmr:frame 2\n// dmr:magic 0-15=\"ABC\"\ntype T struct {\n A uint8 " + tag("bits:16-23") + "\n}", "is 3 octets, the range is 2"},
		{"magic beyond the shortest", "// dmr:frame 1 3\n// dmr:magic 8-15=\"A\"\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B uint8 " + tag("bits:16-23") + "\n}", "not inside the shortest frame"},
		{"open frame", "// dmr:frame 1+\ntype T struct {\n A uint8 " + tag("bits:0-7") + "\n}", ""},
		{"open size not last", "// dmr:frame 1+ 2\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B uint8 " + tag("bits:8-15") + "\n}", "only the last size"},
		{"tail of an open frame", "// dmr:frame 1 3+\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B [2]byte " + tag("bits:8-23,packed,tail") + "\n}", "cannot end an open"},
		{"sizes not ascending", "// dmr:frame 2 1\ntype T struct {\n A uint8 " + tag("bits:0-7") + "\n}", "ascending"},
		{"two default sizes", "// dmr:frame 1* 2*\ntype T struct {\n Size uint8 " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n B uint8 " + tag("bits:8-15") + "\n}", "more than one default"},
		{"frame with crc", "// dmr:frame 3\n// dmr:crc crc_ccitt\ntype T struct {\n FEC fec.Result " + tag("-") + "\n A uint8 " + tag("bits:0-7") + "\n}", "cannot have dmr:fec or dmr:crc"},
		{"view without size", "// dmr:view V\ntype T struct {\n A uint8 " + tag("bits:0-7") + "\n}", "wants a type name and a size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseSrc(t, tt.src)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v does not contain %q", err, tt.want)
			}
		})
	}
}

func TestParseFrame(t *testing.T) {
	t.Parallel()

	pdus := mustParse(t, "// dmr:frame 6 8\n// dmr:magic 0-15=\"DM\"\ntype T struct {\n Size uint8 "+tag("-")+"\n A uint32 "+tag("bits:16-47,le")+"\n B uint16 "+tag("bits:48-63,dec")+"\n}")
	p := pdus[0]
	if p.Frame == nil || len(p.Frame.Sizes) != 2 || p.Frame.SizeField != "Size" || p.Frame.Default != 6 {
		t.Fatalf("frame %+v", p.Frame)
	}
	marked := mustParse(t, "// dmr:frame 6 8*\ntype T struct {\n Size uint8 "+tag("-")+"\n A uint32 "+tag("bits:0-31")+"\n B uint16 "+tag("bits:32-47")+"\n C uint16 "+tag("bits:48-63")+"\n}")
	if f := marked[0].Frame; f == nil || f.Default != 8 || len(f.Sizes) != 2 || f.Sizes[1] != 8 {
		t.Fatalf("marked frame %+v", f)
	}
	if p.InputSize != 64 {
		t.Fatalf("input size %d, want 64", p.InputSize)
	}
	if len(p.Magic) != 1 || string(p.Magic[0].Bytes) != "DM" || p.Magic[0].Start != 0 {
		t.Fatalf("magic %+v", p.Magic)
	}
	if a := p.FieldByName("A"); a.Encoding != parse.EncodingLE {
		t.Fatalf("A encoding %v", a.Encoding)
	}
	if b := p.FieldByName("B"); b.Encoding != parse.EncodingDecimal {
		t.Fatalf("B encoding %v", b.Encoding)
	}
}

func TestParseViewKeepsFieldDocs(t *testing.T) {
	t.Parallel()

	pdus := mustParse(t, "// dmr:view V 2\n// dmr:magic 0-7=0x52\ntype layout struct {\n // A is the value.\n A uint8 "+tag("bits:8-15,hex")+"\n}")
	p := pdus[0]
	if p.View == nil || p.View.Type != "V" || p.View.Size != 2 {
		t.Fatalf("view %+v", p.View)
	}
	if got := p.FieldByName("A").Doc; got != "A is the value." {
		t.Fatalf("doc %q", got)
	}
	if p.Magic[0].Bytes[0] != 0x52 {
		t.Fatalf("magic %x", p.Magic[0].Bytes)
	}
}
