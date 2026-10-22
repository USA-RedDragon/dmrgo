package parse_test

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const header = `package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

var _ enums.FLCO
`

func parseSrc(t *testing.T, src string) ([]parse.PDUStruct, error) {
	t.Helper()
	return parse.Source("test.go", header+src)
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
	t.Parallel()

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
			name: "removed when: modifier",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7,when:Op==1\"`" + `
}`,
			want: "unrecognized modifier",
		},
		{
			name: "removed dispatch: modifier",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	A  *Sub  ` + "`dmr:\"bits:8-15,dispatch:Op=1\"`" + `
}`,
			want: "unrecognized modifier",
		},
		{
			name: "removed fallback flag",
			src: `type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,fallback\"`" + `
}`,
			want: "unrecognized modifier",
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
			name: "raw bool array",
			src: `type T struct {
	A [8]bool ` + "`dmr:\"bits:0-7,raw\"`" + `
}`,
			want: "raw field of 8 bits must be [1]byte, got [8]bool",
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
			name: "fec without an FEC field",
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
			t.Parallel()

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
	t.Parallel()

	pdus := mustParse(t, `type Opcode byte

// dmr:crc crc_ccitt
// dmr:crc_mask 0xA5A5
// dmr:input_size 96
// dmr:dispatch_if DataType==0
// dmr:get Low uint8 bits:16-19,only:Op=3 the low bits of the payload
// dmr:variant A Sub Op=1|2
// dmr:variant B Sub Op=3 Low!=2 DataType==1
// dmr:input DataType
// T is a test PDU (ETSI TS 102 361-1 clause 1).
type T struct {
	FEC      fec.Result `+"`dmr:\"-\"`"+`
	DataType uint8  `+"`dmr:\"-\"`"+`
	LB       bool   `+"`dmr:\"bit:0\"`"+`
	Op       Opcode `+"`dmr:\"bits:2-7\"`"+`
	Split    uint8  `+"`dmr:\"bits:1+8-11\"`"+`
	Rsvd     uint8  `+"`dmr:\"bits:12-15\"`"+`
	Raw      [8]byte `+"`dmr:\"bits:16-79,raw,union\"`"+`
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
	if u := p.UnionField(); u == nil || u.Name != "Raw" || u.Kind != parse.FieldRaw {
		t.Errorf("UnionField = %+v", u)
	}
	want := []parse.Variant{
		{Name: "A", Type: "Sub", Conds: []parse.VariantCond{
			{Field: "Op", Values: []string{"1", "2"}},
			{Field: "DataType", Guard: &parse.Guard{Field: "DataType", Op: "==", Value: "0"}},
		}},
		{Name: "B", Type: "Sub", Conds: []parse.VariantCond{
			{Field: "Op", Values: []string{"3"}},
			{Field: "Low", Guard: &parse.Guard{Field: "Low", Op: "!=", Value: "2"}},
			{Field: "DataType", Guard: &parse.Guard{Field: "DataType", Op: "==", Value: "1"}},
		}},
	}
	if !reflect.DeepEqual(p.Variants, want) {
		t.Errorf("Variants = %+v, want %+v", p.Variants, want)
	}
	low := p.GetterByName("Low")
	if low == nil || !low.Getter || low.BitStart != 16 || low.BitEnd != 19 || low.OnlyField != "Op" ||
		low.Doc != "the low bits of the payload" || p.FieldByName("Low") != nil {
		t.Errorf("Low = %+v", low)
	}
	if !p.FieldByName("DataType").Skip {
		t.Error("DataType should be skipped")
	}
}

func TestParseInputSizeDefaultsToFieldRange(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseAllowsIntentionalOverlap(t *testing.T) {
	t.Parallel()

	mustParse(t, `type T struct {
	Low    uint8 `+"`dmr:\"bits:0-1\"`"+`
	Format uint8 `+"`dmr:\"bits:4-7\"`"+`
	Blocks uint8 `+"`dmr:\"bits:2-3+12-15,no_encode\"`"+`
	Other  uint8 `+"`dmr:\"bits:2-3\"`"+`
	Mid    uint8 `+"`dmr:\"bits:8-15\"`"+`
}`)
}

func TestParseVariantGuardOnSkipFieldRequiresInput(t *testing.T) {
	t.Parallel()

	body := `// dmr:variant A Sub Op=1 Mode==true
// dmr:variant B Sub Op=1 Mode==false
type T struct {
	Mode bool    ` + "`dmr:\"-\"`" + `
	Op   uint8   ` + "`dmr:\"bits:0-7\"`" + `
	Raw  [1]byte ` + "`dmr:\"bits:8-15,raw,union\"`" + `
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
	t.Parallel()

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
			t.Parallel()

			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseRejectsBadUnion(t *testing.T) {
	t.Parallel()

	const fields = `type T struct {
	M  bool    ` + "`dmr:\"bit:0\"`" + `
	Op uint8   ` + "`dmr:\"bits:1-7\"`" + `
	R  [2]byte ` + "`dmr:\"bits:8-23,raw,union\"`" + `
}`
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "union not raw",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	R  uint8 ` + "`dmr:\"bits:8-15,union\"`" + `
}`,
			want: "union requires raw",
		},
		{
			name: "union with no_encode",
			src: `// dmr:variant A Sub Op=1
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,union,no_encode\"`" + `
}`,
			want: "union and no_encode cannot be combined",
		},
		{
			name: "two unions",
			src: `// dmr:variant A Sub Op=1
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,union\"`" + `
	S  [1]byte ` + "`dmr:\"bits:16-23,raw,union\"`" + `
}`,
			want: "multiple union fields: R and S",
		},
		{
			name: "union without variant",
			src:  fields,
			want: "union field R without dmr:variant directives",
		},
		{
			name: "variant without union",
			src: `// dmr:variant A Sub Op=1
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw\"`" + `
}`,
			want: "need a union field",
		},
		{
			name: "dispatch_if without union",
			src: `// dmr:dispatch_if Op==0
type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "need a union field",
		},
		{
			name: "getter without union",
			src: `// dmr:get K uint8 bits:8-11 the kind
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw\"`" + `
}`,
			want: "need a union field",
		},
		{
			name: "unknown condition field",
			src:  "// dmr:variant A Sub Nope=1\n" + fields,
			want: `variant A: unknown field "Nope"`,
		},
		{
			name: "unknown guard field",
			src:  "// dmr:variant A Sub Op=1 Nope==1\n" + fields,
			want: `variant A: unknown field "Nope"`,
		},
		{
			name: "duplicate variant",
			src:  "// dmr:variant A Sub Op=1\n// dmr:variant A Sub Op=2\n" + fields,
			want: "duplicate variant A",
		},
		{
			name: "two conditions on one field",
			src:  "// dmr:variant A Sub Op=1 Op!=2\n" + fields,
			want: "variant A has two conditions on Op",
		},
		{
			name: "condition on the union field",
			src:  "// dmr:variant A Sub R=1\n" + fields,
			want: `field "R" is the union field`,
		},
		{
			name: "variant without condition",
			src:  "// dmr:variant A Sub\n" + fields,
			want: "want Name Type VariantCond",
		},
		{
			name: "unexported variant name",
			src:  "// dmr:variant a Sub Op=1\n" + fields,
			want: "bad name",
		},
		{
			name: "qualified variant type",
			src:  "// dmr:variant A pkg.Sub Op=1\n" + fields,
			want: "bad name",
		},
		{
			name: "condition without value",
			src:  "// dmr:variant A Sub Op\n" + fields,
			want: "condition must be Field=Value[|Value], Field==N or Field!=N",
		},
		{
			name: "condition with empty value",
			src:  "// dmr:variant A Sub Op=1|\n" + fields,
			want: "empty value",
		},
		{
			name: "guard value not a number",
			src:  "// dmr:variant A Sub Op==abc\n" + fields,
			want: "must be true, false, or a decimal integer",
		},
		{
			name: "bool guard on integer field",
			src:  "// dmr:variant A Sub Op==true\n" + fields,
			want: "value true does not match type uint8 of Op",
		},
		{
			name: "integer guard on bool field",
			src:  "// dmr:variant A Sub Op=1 M==1\n" + fields,
			want: "value 1 does not match type bool of M",
		},
		{
			name: "value list on bool field",
			src:  "// dmr:variant A Sub M=true\n" + fields,
			want: "bool field M needs M==true or M==false",
		},
		{
			name: "dispatch_if missing operator",
			src:  "// dmr:dispatch_if Op\n// dmr:variant A Sub Op=1\n" + fields,
			want: "missing operator",
		},
		{
			name: "dispatch_if bad value",
			src:  "// dmr:dispatch_if Op==x\n// dmr:variant A Sub M==true\n" + fields,
			want: "must be true, false, or a decimal integer",
		},
		{
			name: "dispatch_if on unknown field",
			src:  "// dmr:dispatch_if Nope==0\n// dmr:variant A Sub Op=1\n" + fields,
			want: `variant A: unknown field "Nope"`,
		},
		{
			name: "dispatch_if on unset field",
			src: `// dmr:dispatch_if N==true
// dmr:variant A Sub Op=1
type T struct {
	N  bool    ` + "`dmr:\"-\"`" + `
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,union\"`" + `
}`,
			want: "not a dmr:input",
		},
		{
			name: "getter outside the union",
			src:  "// dmr:get K uint8 bits:4-11 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K bits 4-11 are outside union field R",
		},
		{
			name: "getter extra range outside the union",
			src:  "// dmr:get K uint8 bits:8-9+24-25 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K bits 24-25 are outside union field R",
		},
		{
			name: "getter named like a field",
			src:  "// dmr:get Op uint8 bits:8-11 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter Op has the name of a field or getter",
		},
		{
			name: "two getters with one name",
			src:  "// dmr:get K uint8 bits:8-11 the kind\n// dmr:get K uint8 bits:12-15 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K has the name of a field or getter",
		},
		{
			name: "getter without description",
			src:  "// dmr:get K uint8 bits:8-11\n// dmr:variant A Sub Op=1\n" + fields,
			want: "want Name Type Tag Description",
		},
		{
			name: "signed getter type",
			src:  "// dmr:get K int bits:8-11 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K must be a bool or an unsigned integer",
		},
		{
			name: "raw getter",
			src:  "// dmr:get K [1]byte bits:8-15,raw the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K must be a bool or an unsigned integer",
		},
		{
			name: "getter with bias",
			src:  "// dmr:get K uint8 bits:8-15,bias:1 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "getter K takes no signed, encoding or bias modifier",
		},
		{
			name: "getter only: on unknown field",
			src:  "// dmr:get K uint8 bits:8-11,only:Nope=1 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: `getter K only: unknown field "Nope"`,
		},
		{
			name: "getter only: on later getter",
			src:  "// dmr:get K uint8 bits:8-11,only:L=1 the kind\n// dmr:get L uint8 bits:12-15 the length\n// dmr:variant A Sub Op=1\n" + fields,
			want: `getter K only: unknown field "L"`,
		},
		{
			name: "getter only: on bool field",
			src:  "// dmr:get K uint8 bits:8-11,only:M=1 the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "bool field M needs M==true or M==false",
		},
		{
			name: "getter only: without value",
			src:  "// dmr:get K uint8 bits:8-11,only:Op= the kind\n// dmr:variant A Sub Op=1\n" + fields,
			want: "only modifier must be",
		},
		{
			name: "bool getter with only:",
			src:  "// dmr:get K bool bit:8,only:Op=1 the flag\n// dmr:variant A Sub Op=1\n" + fields,
			want: "only: requires an integer field",
		},
		{
			name: "only: on a plain field",
			src: `type T struct {
	Op uint8 ` + "`dmr:\"bits:0-7\"`" + `
	K  uint8 ` + "`dmr:\"bits:8-15,only:Op=1\"`" + `
}`,
			want: "field K: only: is valid only on dmr:get",
		},
		{
			name: "variant in a frame",
			src: `// dmr:frame 2
// dmr:variant A Sub Op=1
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [1]byte ` + "`dmr:\"bits:8-15,raw,union\"`" + `
}`,
			want: "dmr:frame and dmr:view cannot have",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestParseSpecRefUsesFirstCitation(t *testing.T) {
	t.Parallel()

	pdus := mustParse(t, `// T is the first PDU (ETSI TS 102 361-1
// clause 9.1.6). It uses the FLCO (ETSI TS 102 361-1 Table 9.21).
type T struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if pdus[0].SpecRef != "ETSI TS 102 361-1 clause 9.1.6" {
		t.Errorf("SpecRef = %q", pdus[0].SpecRef)
	}
}

func TestParseV2Sources(t *testing.T) {
	t.Parallel()

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
		pdus, err := parse.File(path)
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
	t.Parallel()

	pdus := mustParse(t, `type T struct {
	A uint8
}`)
	if len(pdus) != 0 {
		t.Errorf("got %d structs, want 0", len(pdus))
	}
}

func TestSetUnexportedDecode(t *testing.T) {
	t.Parallel()

	const top = `// dmr:variant A Sub Op=1
type T struct {
	Op uint8   ` + "`dmr:\"bits:0-7\"`" + `
	R  [2]byte ` + "`dmr:\"bits:8-23,raw,union\"`" + `
}
`
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unknown variant type",
			src:  top,
			want: "struct T variant A: type Sub is not a PDU struct of the package",
		},
		{
			name: "variant type of another size",
			src: top + `type Sub struct {
	X uint8 ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "struct T variant A: type Sub has 8 bits, union field R has 16",
		},
		{
			name: "variant type with a CRC",
			src: top + `// dmr:crc crc_8
// dmr:input_size 16
type Sub struct {
	FEC fec.Result ` + "`dmr:\"-\"`" + `
	X   uint8      ` + "`dmr:\"bits:0-7\"`" + `
}`,
			want: "struct T variant A: type Sub must be a plain PDU struct",
		},
		{
			name: "variant type with an input",
			src: top + `// dmr:input M
type Sub struct {
	M bool   ` + "`dmr:\"-\"`" + `
	X uint16 ` + "`dmr:\"bits:0-15\"`" + `
}`,
			want: "struct T variant A: type Sub must be a plain PDU struct",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pdus := mustParse(t, tt.src)
			err := parse.SetUnexportedDecode(pdus)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("got error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSetUnexportedDecodeMarksVariantTypes(t *testing.T) {
	t.Parallel()

	pdus := mustParse(t, `// dmr:variant A Sub Op=1
// dmr:variant B Sub Op=2
type T struct {
	Op uint8   `+"`dmr:\"bits:0-7\"`"+`
	R  [2]byte `+"`dmr:\"bits:8-23,raw,union\"`"+`
}

type Sub struct {
	X uint16 `+"`dmr:\"bits:0-15\"`"+`
}

type Other struct {
	Y uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if err := parse.SetUnexportedDecode(pdus); err != nil {
		t.Fatal(err)
	}
	for _, p := range pdus {
		if !p.UnexportedDecode["Sub"] || p.UnexportedDecode["Other"] || p.UnexportedDecode["T"] {
			t.Errorf("%s: UnexportedDecode = %v, want only Sub", p.Name, p.UnexportedDecode)
		}
	}
	if got := pdus[0].DecodeFuncName("Sub"); got != "decodeSub" {
		t.Errorf("DecodeFuncName(Sub) = %q", got)
	}
	if got := pdus[0].DecodeFuncName("Other"); got != "DecodeOther" {
		t.Errorf("DecodeFuncName(Other) = %q", got)
	}
}
