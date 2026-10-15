package parse_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const header = `package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

var _ bit.Bit
var _ enums.FLCO
`

func parseSrc(t *testing.T, src string) ([]parse.PDUStruct, error) {
	t.Helper()
	return parse.ParseSource("test.go", header+src)
}

func mustParse(t *testing.T, src string) []parse.PDUStruct {
	t.Helper()
	pdus, err := parseSrc(t, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return pdus
}

func TestParseRejectsMalformed(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "crc_mask not hex",
			src: `// dmr:crc crc_ccitt
// dmr:crc_mask 0xZZZZ
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "invalid dmr:crc_mask",
		},
		{
			name: "crc_mask without 0x",
			src: `// dmr:crc crc_ccitt
// dmr:crc_mask A5A5
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "must start with 0x",
		},
		{
			name: "crc_mask too wide",
			src: `// dmr:crc crc_ccitt
// dmr:crc_mask 0x1A5A5
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "invalid dmr:crc_mask",
		},
		{
			name: "crc_mask without crc",
			src: `// dmr:crc_mask 0xA5A5
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "dmr:crc_mask without dmr:crc",
		},
		{
			name: "input_size not a number",
			src: `// dmr:input_size abc
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "invalid dmr:input_size",
		},
		{
			name: "input_size zero",
			src: `// dmr:input_size 0
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "invalid dmr:input_size",
		},
		{
			name: "input_size smaller than fields",
			src: `// dmr:input_size 4
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "smaller than the field range",
		},
		{
			name: "unknown crc algorithm",
			src: `// dmr:crc crc_32
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "unknown CRC algorithm",
		},
		{
			name: "unknown directive",
			src: `// dmr:fecc golay_20_8_7
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "unknown directive",
		},
		{
			name: "directive missing argument",
			src: `// dmr:fec
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "missing argument",
		},
		{
			name: "when value not a number",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	K  uint8 ` + "`dmr:\"bits:8-15\"`" + `
	A  *Sub  ` + "`dmr:\"bits:16-23,dispatch:Op=1,when:K==abc\"`" + `
}`,
			want: "must be true, false, or a decimal integer",
		},
		{
			name: "when missing operator",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:16-23,dispatch:Op=1,when:K\"`" + `
}`,
			want: "missing operator",
		},
		{
			name: "when bool value on integer field",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	K  uint8 ` + "`dmr:\"bits:8-15\"`" + `
	A  *Sub  ` + "`dmr:\"bits:16-23,dispatch:Op=1,when:K==true\"`" + `
}`,
			want: "does not match type",
		},
		{
			name: "when on unknown field",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:16-23,dispatch:Op=1,when:Nope==1\"`" + `
}`,
			want: "unknown field",
		},
		{
			name: "when without dispatch",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7,when:Op==1\"`" + `
}`,
			want: "when: requires dispatch",
		},
		{
			name: "reversed range",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:7-2\"`" + `
}`,
			want: "reversed range",
		},
		{
			name: "reversed extra range",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0+7-2\"`" + `
}`,
			want: "reversed range",
		},
		{
			name: "non-numeric range",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:a-2\"`" + `
}`,
			want: "invalid bit range",
		},
		{
			name: "tagged embedded field",
			src: `type Inner struct{}
type T struct {
	Inner ` + "`dmr:\"bits:0-7\"`" + `
	A uint8 ` + "`dmr:\"bits:8-15\"`" + `
}`,
			want: "embedded field",
		},
		{
			name: "removed from: modifier",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-7,enum,from:enums.FLCOFromInt\"`" + `
}`,
			want: "unrecognized modifier",
		},
		{
			name: "stride does not divide width",
			src: `type T struct {
	A [3]Sub ` + "`dmr:\"bits:0-99,delegate,stride:32\"`" + `
}`,
			want: "does not divide",
		},
		{
			name: "stride element count mismatch",
			src: `type T struct {
	A [2]Sub ` + "`dmr:\"bits:0-95,delegate,stride:32\"`" + `
}`,
			want: "does not hold 3 elements",
		},
		{
			name: "stride without delegate",
			src: `type T struct {
	A [1]byte ` + "`dmr:\"bits:0-2,raw,stride:1\"`" + `
}`,
			want: "stride requires delegate",
		},
		{
			name: "invalid stride",
			src: `type T struct {
	A [3]Sub ` + "`dmr:\"bits:0-95,delegate,stride:x\"`" + `
}`,
			want: "invalid stride",
		},
		{
			name: "dispatch ranges differ",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
	B  *Sub  ` + "`dmr:\"bits:8-23,dispatch:Op=2\"`" + `
}`,
			want: "differs from",
		},
		{
			name: "dispatch switch fields differ",
			src: `type T struct {
	Op  uint8 ` + "`dmr:\"bits:0-3\"`" + `
	Op2 uint8 ` + "`dmr:\"bits:4-7\"`" + `
	A   *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
	B   *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op2=2\"`" + `
}`,
			want: "switches on",
		},
		{
			name: "dispatch on unknown field",
			src: `type T struct {
	A *Sub ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
}`,
			want: "unknown field",
		},
		{
			name: "dispatch non-pointer",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  Sub   ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
}`,
			want: "must be a pointer",
		},
		{
			name: "dispatch missing value",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=\"`" + `
}`,
			want: "dispatch modifier must be",
		},
		{
			name: "crc range overlaps field",
			src: `// dmr:crc crc_ccitt
// dmr:input_size 96
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
	B uint8 ` + "`dmr:\"bits:80-87\"`" + `
}`,
			want: "overlaps the CRC",
		},
		{
			name: "crc_8 range overlaps field",
			src: `// dmr:crc crc_8
// dmr:input_size 36
type T struct {
	A uint32 ` + "`dmr:\"bits:0-28\"`" + `
}`,
			want: "overlaps the CRC",
		},
		{
			name: "noptr delegate wider than type",
			src: `type Small uint8
type T struct {
	A Small ` + "`dmr:\"bits:0-11,delegate,noptr\"`" + `
}`,
			want: "narrower than the 12-bit range",
		},
		{
			name: "noptr without delegate",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-7,noptr\"`" + `
}`,
			want: "noptr requires delegate",
		},
		{
			name: "uint8 wider than type",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-11\"`" + `
}`,
			want: "narrower than the 12-bit range",
		},
		{
			name: "named byte type wider than type",
			src: `type Opcode byte
type T struct {
	A Opcode ` + "`dmr:\"bits:0-8\"`" + `
}`,
			want: "narrower than the 9-bit range",
		},
		{
			name: "integer wider than 64 bits",
			src: `type T struct {
	A uint64 ` + "`dmr:\"bits:0-64\"`" + `
}`,
			want: "wider than 64 bits",
		},
		{
			name: "raw array shorter than range",
			src: `type T struct {
	A [1]byte ` + "`dmr:\"bits:0-8,raw\"`" + `
}`,
			want: "raw field of 9 bits must be [2]byte, got [1]byte",
		},
		{
			name: "raw array longer than range",
			src: `type T struct {
	A [2]byte ` + "`dmr:\"bits:0-7,raw\"`" + `
}`,
			want: "raw field of 8 bits must be [1]byte, got [2]byte",
		},
		{
			name: "raw bit array",
			src: `type T struct {
	A [8]bit.Bit ` + "`dmr:\"bits:0-7,raw\"`" + `
}`,
			want: "raw field of 8 bits must be [1]byte, got [8]bit.Bit",
		},
		{
			name: "packed array shorter than range",
			src: `type T struct {
	A [1]byte ` + "`dmr:\"bits:0-15,packed\"`" + `
}`,
			want: "does not match range width",
		},
		{
			name: "packed array longer than range",
			src: `type T struct {
	A [3]byte ` + "`dmr:\"bits:0-15,packed\"`" + `
}`,
			want: "does not match range width",
		},
		{
			name: "packed slice",
			src: `type T struct {
	A []byte ` + "`dmr:\"bits:0-15,packed\"`" + `
}`,
			want: "packed field must be [N]byte",
		},
		{
			name: "bool with range",
			src: `type T struct {
	A bool ` + "`dmr:\"bits:0-3\"`" + `
}`,
			want: "single bit",
		},
		{
			name: "bit on non-bool",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bit:0\"`" + `
}`,
			want: "requires a bool",
		},
		{
			name: "signed on unsigned type",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-7,signed\"`" + `
}`,
			want: "signed requires a signed integer",
		},
		{
			name: "unknown modifier",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-7,bogus\"`" + `
}`,
			want: "unrecognized modifier",
		},
		{
			name: "unknown semantic type",
			src: `type T struct {
	A float64 ` + "`dmr:\"bits:0-7,type:altitude\"`" + `
}`,
			want: "unknown semantic type",
		},
		{
			name: "latitude on integer",
			src: `type T struct {
	A int ` + "`dmr:\"bits:0-23,type:latitude\"`" + `
}`,
			want: "latitude/longitude fields must be",
		},
		{
			name: "longitude on float32",
			src: `type T struct {
	A float32 ` + "`dmr:\"bits:0-24,type:longitude\"`" + `
}`,
			want: "latitude/longitude fields must be float64",
		},
		{
			name: "non-contiguous raw",
			src: `type T struct {
	A [1]byte ` + "`dmr:\"bits:3+12-15,raw\"`" + `
}`,
			want: "raw fields must be contiguous",
		},
		{
			name: "unrecognized bit specifier",
			src: `type T struct {
	A uint8 ` + "`dmr:\"byte:0\"`" + `
}`,
			want: "unrecognized bit specifier",
		},
		{
			name: "empty tag",
			src: `type T struct {
	A uint8 ` + "`dmr:\"\"`" + `
}`,
			want: "unrecognized bit specifier",
		},
		{
			name: "uncovered bits",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:0-3\"`" + `
	B bool  ` + "`dmr:\"bit:5\"`" + `
	C uint8 ` + "`dmr:\"bits:9-10\"`" + `
}`,
			want: "uncovered bits 4,6-8: add a field, the CRC or a dmr:const",
		},
		{
			name: "uncovered bits before the CRC",
			src: `// dmr:crc crc_8
// dmr:input_size 16
type T struct {
	A uint8 ` + "`dmr:\"bits:0-6\"`" + `
}`,
			want: "uncovered bits 7:",
		},
		{
			name: "const without value",
			src: `// dmr:const 0-3
type T struct {
	A uint8 ` + "`dmr:\"bits:4-7\"`" + `
}`,
			want: "want S-E[+S-E]=Value",
		},
		{
			name: "const value too wide",
			src: `// dmr:const 0-3=16
type T struct {
	A uint8 ` + "`dmr:\"bits:4-7\"`" + `
}`,
			want: "value 16 does not fit in 4 bits",
		},
		{
			name: "const value not a name",
			src: `// dmr:const 0-3=a-b
type T struct {
	A uint8 ` + "`dmr:\"bits:4-7\"`" + `
}`,
			want: "is not an integer or a constant name",
		},
		{
			name: "const overlaps field",
			src: `// dmr:const 0-4=1
type T struct {
	A uint8 ` + "`dmr:\"bits:4-7\"`" + `
}`,
			want: "A and dmr:const overlap at bit 4",
		},
		{
			name: "fec without a FEC field",
			src: `// dmr:fec golay_20_8_7
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "need a dmr:\"-\" FEC fec.Result field",
		},
		{
			name: "FEC field without fec or crc",
			src: `type T struct {
	FEC fec.Result ` + "`dmr:\"-\"`" + `
	A   uint8      ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "field FEC needs dmr:fec or dmr:crc",
		},
		{
			name: "FEC field of another type",
			src: `// dmr:crc crc_8
// dmr:input_size 16
type T struct {
	FEC uint8 ` + "`dmr:\"-\"`" + `
	A   uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "field FEC must be a dmr:\"-\" fec.Result",
		},
		{
			name: "const beyond input size",
			src: `// dmr:const 8-9=1
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "dmr:const bits 8-9 extend beyond input size 8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSrc(t, tt.src)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestParseValidStruct(t *testing.T) {
	pdus := mustParse(t, `type Opcode byte

// dmr:crc crc_ccitt
// dmr:crc_mask 0xA5A5
// dmr:input_size 96
// T is a test PDU (ETSI TS 102 361-1 clause 1).
type T struct {
	FEC      fec.Result `+"`dmr:\"-\"`"+`
	DataType uint8  `+"`dmr:\"-\"`"+`
	LB       bool   `+"`dmr:\"bit:0\"`"+`
	Op       Opcode `+"`dmr:\"bits:2-7\"`"+`
	Split    uint8  `+"`dmr:\"bits:1+8-11\"`"+`
	Rsvd     uint8  `+"`dmr:\"bits:12-15\"`"+`
	A        *Sub   `+"`dmr:\"bits:16-79,dispatch:Op=1|2\"`"+`
}`)
	if len(pdus) != 1 {
		t.Fatalf("got %d structs, want 1", len(pdus))
	}
	p := pdus[0]
	if p.InputSize != 96 || !p.HasInputSize {
		t.Errorf("InputSize = %d, want 96", p.InputSize)
	}
	if p.CRC == nil || p.CRC.Algorithm != "crc_ccitt" || p.CRC.Mask != 0xA5A5 || !p.CRC.HasMask {
		t.Errorf("CRC = %+v", p.CRC)
	}
	if p.SpecRef != "ETSI TS 102 361-1 clause 1" {
		t.Errorf("SpecRef = %q", p.SpecRef)
	}
	op := p.FieldByName("Op")
	if op.Kind != parse.FieldUint || op.BaseType != "byte" {
		t.Errorf("Op kind=%v base=%q", op.Kind, op.BaseType)
	}
	split := p.FieldByName("Split")
	if split.BitWidth != 5 || len(split.ExtraBitRanges) != 1 {
		t.Errorf("Split = %+v", split)
	}
	a := p.FieldByName("A")
	if a.Kind != parse.FieldDispatch || len(a.DispatchValues) != 2 {
		t.Errorf("A = %+v", a)
	}
	if !p.FieldByName("DataType").Skip {
		t.Error("DataType should be skipped")
	}
}

func TestParseInputSizeDefaultsToFieldRange(t *testing.T) {
	pdus := mustParse(t, `type T struct {
	A uint8 `+"`dmr:\"bits:0-6\"`"+`
	R uint8 `+"`dmr:\"bits:7-8\"`"+`
	B bool  `+"`dmr:\"bit:9\"`"+`
}`)
	if pdus[0].InputSize != 10 {
		t.Errorf("InputSize = %d, want 10", pdus[0].InputSize)
	}
}

func TestParseRejectsOverlap(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "range and bit",
			src: `type T struct {
	Len int  ` + "`dmr:\"bits:2-7\"`" + `
	MSB bool ` + "`dmr:\"bit:7\"`" + `
}`,
			want: "fields Len and MSB overlap at bit 7",
		},
		{
			name: "non-contiguous segment",
			src: `type T struct {
	A uint8 ` + "`dmr:\"bits:3+12-15\"`" + `
	B uint8 ` + "`dmr:\"bits:8-12\"`" + `
}`,
			want: "fields A and B overlap at bit 12",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseAllowsIntentionalOverlap(t *testing.T) {
	mustParse(t, `type T struct {
	Format uint8 `+"`dmr:\"bits:4-7\"`"+`
	Blocks uint8 `+"`dmr:\"bits:2-3+12-15,no_encode\"`"+`
	Other  uint8 `+"`dmr:\"bits:2-3\"`"+`
	A      *Sub  `+"`dmr:\"bits:0-79,dispatch:Format=1\"`"+`
	B      *Sub  `+"`dmr:\"bits:0-79,dispatch:Format=2\"`"+`
}`)
}

func TestParseWhenOnSkipFieldRequiresInput(t *testing.T) {
	body := `type T struct {
	Mode bool  ` + "`dmr:\"-\"`" + `
	Op   uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A    *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1,when:Mode==true\"`" + `
	B    *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1,when:Mode==false\"`" + `
}`
	_, err := parseSrc(t, body)
	if err == nil || !strings.Contains(err.Error(), "not a dmr:input") {
		t.Fatalf("expected dmr:input error, got %v", err)
	}

	pdus := mustParse(t, "// dmr:input Mode\n"+body)
	if got := pdus[0].Inputs; len(got) != 1 || got[0] != "Mode" {
		t.Errorf("Inputs = %v, want [Mode]", got)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "input on decoded field",
			src: `// dmr:input A
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "must name a dmr:\"-\" field",
		},
		{
			name: "input on unknown field",
			src: `// dmr:input Nope
type T struct {
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "must name a dmr:\"-\" field",
		},
		{
			name: "duplicate input",
			src: `// dmr:input M N M
type T struct {
	M bool  ` + "`dmr:\"-\"`" + `
	N bool  ` + "`dmr:\"-\"`" + `
	A uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "duplicate dmr:input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseRejectsBadFallbackAndDispatchIf(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "fallback not raw",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	R  uint8 ` + "`dmr:\"bits:8-15,fallback\"`" + `
}`,
			want: "fallback requires raw",
		},
		{
			name: "fallback without dispatch",
			src: `type T struct {
	Op uint8      ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,fallback\"`" + `
}`,
			want: "without dispatch fields",
		},
		{
			name: "fallback range differs",
			src: `type T struct {
	Op uint8      ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub       ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-11,raw,fallback\"`" + `
}`,
			want: "differs from dispatch range",
		},
		{
			name: "two fallbacks",
			src: `type T struct {
	Op uint8      ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub       ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,fallback\"`" + `
	S  [1]byte ` + "`dmr:\"bits:8-15,raw,fallback\"`" + `
}`,
			want: "multiple fallback fields",
		},
		{
			name: "dispatch_if without dispatch",
			src: `// dmr:dispatch_if Op==0
type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "without dispatch fields",
		},
		{
			name: "dispatch_if on unset field",
			src: `// dmr:dispatch_if M==true
type T struct {
	M  bool  ` + "`dmr:\"-\"`" + `
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
}`,
			want: "not a dmr:input",
		},
		{
			name: "dispatch_if bad value",
			src: `// dmr:dispatch_if Op==x
type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
}`,
			want: "must be true, false, or a decimal integer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseSpecRefUsesFirstCitation(t *testing.T) {
	pdus := mustParse(t, `// T is the first PDU (ETSI TS 102 361-1
// clause 9.1.6). It uses the FLCO (ETSI TS 102 361-1 Table 9.21).
type T struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if pdus[0].SpecRef != "ETSI TS 102 361-1 clause 9.1.6" {
		t.Errorf("SpecRef = %q", pdus[0].SpecRef)
	}
}

// TestParseV2Sources parses every tagged struct of the v2 module, so that a
// tag error such as an uncovered bit fails here as well as in go generate.
func TestParseV2Sources(t *testing.T) {
	root := filepath.Join("..", "..", "..", "v2")
	structs := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			return nil
		}
		pdus, err := parse.ParseFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
		}
		structs += len(pdus)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if structs == 0 {
		t.Fatal("found no tagged structs")
	}
}

func TestParseIgnoresUntaggedStructs(t *testing.T) {
	pdus := mustParse(t, `type T struct {
	A uint8
}`)
	if len(pdus) != 0 {
		t.Errorf("got %d structs, want 0", len(pdus))
	}
}
