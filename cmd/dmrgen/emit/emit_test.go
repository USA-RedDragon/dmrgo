package emit_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/emit"
	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const header = `package pdu
`

func generate(t *testing.T, src string) (string, error) {
	t.Helper()
	pdus, err := parse.Source("test.go", header+src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return render(t, pdus)
}

func render(t *testing.T, pdus []parse.PDUStruct) (string, error) {
	t.Helper()
	f, err := emit.GenerateFile(pdus, "pdu")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := f.Render(&buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String(), nil
}

func mustGenerate(t *testing.T, src string) string {
	t.Helper()
	out, err := generate(t, src)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return out
}

func assertContains(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("generated code missing %q\n%s", w, out)
		}
	}
}

func TestNoPtrDelegateUsesWidthOfRange(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Wide uint16
type T struct {
	R uint8 `+"`dmr:\"bits:0-1\"`"+`
	A Wide `+"`dmr:\"bits:2-13,delegate,noptr\"`"+`
}`)
	assertContains(t, out,
		"result.A = Wide((uint64(data[0])<<8 | uint64(data[1])) >> 2 & 0xFFF)",
		"_valA := uint64(s.A)",
		"data[0] = data[0]&^0x3F | byte(_valA>>6)&0x3F",
		"data[1] = data[1]&^0xFC | byte(_valA<<2)&0xFC",
	)
}

func TestStringModifierCallsTheMethod(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Op uint8
type T struct {
	A Op    `+"`dmr:\"bits:0-3,enum,string:aName\"`"+`
	B uint8 `+"`dmr:\"bits:4-7\"`"+`
}`)
	assertContains(t, out, `"T{ A: " + s.aName() + ", B: "`)
}

func TestUnknownCRCAlgorithmIsRejected(t *testing.T) {
	t.Parallel()

	_, err := render(t, []parse.PDUStruct{{
		Name:      "T",
		Package:   "pdu",
		InputSize: 96,
		CRC:       &parse.CRCDirective{Algorithm: "crc_32"},
		Fields:    []parse.Field{{Name: "A", Kind: parse.FieldUint, BitStart: 0, BitEnd: 7, BitWidth: 8, GoType: "uint8", TypeName: "uint8"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown CRC algorithm") {
		t.Fatalf("expected unknown CRC algorithm error, got %v", err)
	}
}

func TestUnknownFECCodecIsRejected(t *testing.T) {
	t.Parallel()

	_, err := generate(t, `// dmr:fec golay_99
type T struct {
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if err == nil || !strings.Contains(err.Error(), "unknown FEC codec") {
		t.Fatalf("expected unknown FEC codec error, got %v", err)
	}
}

func TestFieldsBeyondFECDataBitsAreRejected(t *testing.T) {
	t.Parallel()

	_, err := generate(t, `// dmr:fec golay_20_8_7
type T struct {
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A uint16 `+"`dmr:\"bits:0-8\"`"+`
}`)
	if err == nil || !strings.Contains(err.Error(), "carries 8 data bits") {
		t.Fatalf("expected data bits error, got %v", err)
	}
}

func TestSignedFieldIsSignExtended(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Offset int16
type T struct {
	A int    `+"`dmr:\"bits:0-11,signed\"`"+`
	B Offset `+"`dmr:\"bits:12-19,signed\"`"+`
	C int32  `+"`dmr:\"bits:20-51,signed\"`"+`
	D int    `+"`dmr:\"bits:52-59\"`"+`
}`)
	assertContains(t, out,
		"result.A = int(int32(uint32((uint64(data[0])<<8|uint64(data[1]))>>4)<<20) >> 20)",
		"result.B = Offset(int32(uint32((uint64(data[1])<<8|uint64(data[2]))>>4&0xFF)<<24) >> 24)",
		"result.C = int32(int32(uint32((uint64(data[2])<<32 | uint64(data[3])<<24 | uint64(data[4])<<16 | uint64(data[5])<<8 | uint64(data[6])) >> 4 & 0xFFFFFFFF)))",
		"result.D = int((uint64(data[6])<<8 | uint64(data[7])) >> 4 & 0xFF)",
	)
}

func TestCoordinatesAreTwosComplementFloat64(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type T struct {
	Lon float64 `+"`dmr:\"bits:0-24,type:longitude\"`"+`
	Lat float64 `+"`dmr:\"bits:25-48,type:latitude\"`"+`
}`)
	assertContains(t, out,
		"result.Lon = float64(int32(uint32((uint64(data[0])<<24|uint64(data[1])<<16|uint64(data[2])<<8|uint64(data[3]))>>7)<<7)>>7) * 360.0 / (1 << 25)",
		"result.Lat = float64(int32(uint32((uint64(data[3])<<24|uint64(data[4])<<16|uint64(data[5])<<8|uint64(data[6]))>>7&0xFFFFFF)<<8)>>8) * 180.0 / (1 << 24)",
		"if !(s.Lon >= -180.0 && s.Lon <= 180.0) {",
		"_rawLon := math.Round(s.Lon * (1 << 25) / 360.0)",
		"_valLon := uint64(int64(_rawLon))",
		"data[3] = data[3]&^0x80 | byte(_valLon<<7)&0x80",
		"if !(s.Lat >= -90.0 && s.Lat <= 90.0) {",
		"_rawLat := math.Round(s.Lat * (1 << 24) / 180.0)",
		"_rawLat = min(_rawLat, 8388607)",
	)
}

func TestNonContiguousRangesForAllIntegerKinds(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Kind uint8
func (k Kind) String() string { return "" }
type Wrapped uint16
func (w Wrapped) String() string { return "" }
type T struct {
	U uint8   `+"`dmr:\"bits:0+4-6\"`"+`
	I int     `+"`dmr:\"bits:1+7-9\"`"+`
	S int     `+"`dmr:\"bits:2+10-12,signed\"`"+`
	E Kind    `+"`dmr:\"bits:3+13-15,enum\"`"+`
	D Wrapped `+"`dmr:\"bits:16-23+24-27,delegate,noptr\"`"+`
}`)
	assertContains(t, out,
		"_tmpU <<= 3",
		"result.U = uint8(_tmpU)",
		"result.I = int(_tmpI)",
		"result.S = int(int32(uint32(_tmpS)<<28) >> 28)",
		"result.E = Kind(_tmpE)",
		"result.D = Wrapped(_tmpD)",
		"_valI := uint64(s.I)",
		"data[0] = data[0]&^0x40 | byte(_valI>>3<<6)&0x40",
		"data[1] = data[1]&^0x38 | byte(_valS<<3)&0x38",
		"data[0] = data[0]&^0x10 | byte(_valE>>3<<4)&0x10",
		"_valD := uint64(s.D)",
		"data[2] = byte(_valD >> 4)",
	)
}

func TestEncodeReportsOutOfRangeValues(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}
type T struct {
	Full uint8  `+"`dmr:\"bits:0-7\"`"+`
	Part uint16 `+"`dmr:\"bits:8-19\"`"+`
	Addr int    `+"`dmr:\"bits:20-43\"`"+`
	Off  int    `+"`dmr:\"bits:44-51,signed\"`"+`
	Kid  Sub    `+"`dmr:\"bits:52-59,delegate\"`"+`
}`)
	assertContains(t, out,
		"func (s *T) Encode() ([8]byte, error) {",
		"if uint64(s.Part)>>12 != 0 {",
		`return [8]byte{}, errTPartRange`,
		"errTPartRange = &dmrgo.FieldRangeError{\n\t\tBits:  12,\n\t\tField: \"Part\",\n\t\tType:  \"T\",\n\t}",
		"errTOffRange = &dmrgo.FieldRangeError{\n\t\tBits:   8,\n\t\tField:  \"Off\",\n\t\tSigned: true,\n\t\tType:   \"T\",\n\t}",
		"if uint64(s.Addr)>>24 != 0 {",
		"if int64(s.Off) < -128 || int64(s.Off) > 127 {",
		"_kidBytes, err := s.Kid.Encode()",
		"if err != nil {\n\t\treturn [8]byte{}, err\n\t}",
		"return data, nil",
	)
	if strings.Contains(out, "uint64(s.Full)>>") {
		t.Error("a uint8 in an 8-bit field cannot overflow and needs no check")
	}
}

func TestConstIsEncoded(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Format uint8

const FormatX Format = 5

// dmr:const 0-3=FormatX
// dmr:const 4-5+10-11=0x9
type T struct {
	A uint8 `+"`dmr:\"bits:6-9\"`"+`
	B uint8 `+"`dmr:\"bits:12-15\"`"+`
}`)
	assertContains(t, out,
		"data[0] = data[0]&^0xF0 | byte(uint64(FormatX)<<4)&0xF0",
		"_const1 := uint64(9)",
		"data[0] = data[0]&^0x0C | byte(_const1>>2<<2)&0x0C",
		"data[1] = data[1]&^0x30 | byte(_const1<<4)&0x30",
	)
}

func TestDecodeReportsFECInTheFECField(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `// dmr:fec golay_20_8_7
type G struct {
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A   uint8      `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:crc crc_8
// dmr:input_size 16
type C struct {
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A   uint8      `+"`dmr:\"bits:0-7\"`"+`
}

type P struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	assertContains(t, out,
		"func DecodeG(data [3]byte) G {",
		"_cw, result.FEC = golay.Decode2087(uint32((uint64(data[0])<<16 | uint64(data[1])<<8 | uint64(data[2])) >> 4))",
		"func DecodeC(data [2]byte) C {",
		"result.FEC.Uncorrectable = !crc.CheckCRC8(data[:], 8, uint8(uint64(data[1])))",
		"func DecodeP(data [1]byte) P {",
	)
	if strings.Contains(out, "fecResult") {
		t.Errorf("generated code has a separate FEC result:\n%s", out)
	}
}

func TestRawFieldIsPackedMSBFirstWithZeroPadding(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type T struct {
	A uint8   `+"`dmr:\"bits:0-3\"`"+`
	R [2]byte `+"`dmr:\"bits:4-15,raw\"`"+`
}`)
	assertContains(t, out,
		"bits.Copy(result.R[:], 0, data[:], 4, 12)",
		"if s.R[1]&15 != 0 {",
		"return [2]byte{}, errTRRange",
		"bits.Copy(data[:], 4, s.R[:], 0, 12)",
		`return "T{ A: " + text.Int(int64(s.A)) + ", R: " + text.Hex(s.R[:]) + " }"`,
	)
}

func TestGeneratedHeaderAndDocs(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `// UDTHeader is a test header (ETSI TS 102 361-1 Table 9.17D).
type UDTHeader struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// EmbeddedSignalling is a test PDU (ETSI TS 102 361-1 clause 9.1.2).
type EmbeddedSignalling struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if !strings.HasPrefix(out, "// Code generated by dmrgen. DO NOT EDIT.\n") {
		t.Errorf("header is not a Go generated-code line:\n%s", out)
	}
	assertContains(t, out,
		"// DecodeUDTHeader decodes a UDTHeader. data holds",
		"// (ETSI TS 102 361-1 Table 9.17D)\nfunc DecodeUDTHeader(",
		"// Encode encodes an EmbeddedSignalling. It returns",
		"// (ETSI TS 102 361-1 clause 9.1.2)\nfunc (s *EmbeddedSignalling) Encode(",
		"// ETSI TS 102 361-1 clause 9.1.2\n",
	)
}

func TestPackedFieldIsPackedWithoutAllocating(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type T struct {
	A uint8   `+"`dmr:\"bits:0-7\"`"+`
	D [2]byte `+"`dmr:\"bits:8-23,packed\"`"+`
}`)
	assertContains(t, out,
		"copy(result.D[0:2], data[1:3])",
		"copy(data[1:3], s.D[0:2])",
	)
	if strings.Contains(out, "bits.Copy(") {
		t.Errorf("byte-aligned packed field uses bits.Copy:\n%s", out)
	}
}
