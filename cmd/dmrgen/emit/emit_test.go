package emit_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/emit"
	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const header = `package pdu

import "github.com/USA-RedDragon/dmrgo/v2/bit"

var _ bit.Bit
`

func generate(t *testing.T, src string) (string, error) {
	t.Helper()
	pdus, err := parse.ParseSource("test.go", header+src)
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
	out := mustGenerate(t, `type Wide uint16
type T struct {
	R uint8 `+"`dmr:\"bits:0-1\"`"+`
	A Wide `+"`dmr:\"bits:2-13,delegate,noptr\"`"+`
}`)
	assertContains(t, out,
		"bit.Mask(data[:])",
		"result.A = Wide(bit.Uint(data[2:14]))",
		"bit.PutUint(data[2:14], uint64(s.A))",
	)
}

func TestUnknownCRCAlgorithmIsRejected(t *testing.T) {
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
	_, err := generate(t, `// dmr:fec golay_20_8_7
type T struct {
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A uint16 `+"`dmr:\"bits:0-8\"`"+`
}`)
	if err == nil || !strings.Contains(err.Error(), "carries 8 data bits") {
		t.Fatalf("expected data bits error, got %v", err)
	}
}

func TestSharedDispatchValueWithoutGuardIsRejected(t *testing.T) {
	_, err := generate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}
type T struct {
	Op uint8 `+"`dmr:\"bits:0-7\"`"+`
	A  *Sub  `+"`dmr:\"bits:8-15,dispatch:Op=1\"`"+`
	B  *Sub  `+"`dmr:\"bits:8-15,dispatch:Op=1\"`"+`
}`)
	if err == nil || !strings.Contains(err.Error(), "neither has a when: guard") {
		t.Fatalf("expected guard error, got %v", err)
	}
}

func TestGuardedDispatch(t *testing.T) {
	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}
type T struct {
	Op uint8 `+"`dmr:\"bits:0-7\"`"+`
	K  uint8 `+"`dmr:\"bits:8-15\"`"+`
	A  *Sub  `+"`dmr:\"bits:16-23,dispatch:Op=1,when:K==0\"`"+`
	B  *Sub  `+"`dmr:\"bits:16-23,dispatch:Op=1\"`"+`
}`)
	assertContains(t, out, "if result.K == 0 {", "} else {")
}

func TestSignedFieldIsSignExtended(t *testing.T) {
	out := mustGenerate(t, `type Offset int16
type T struct {
	A int    `+"`dmr:\"bits:0-11,signed\"`"+`
	B Offset `+"`dmr:\"bits:12-19,signed\"`"+`
	C int32  `+"`dmr:\"bits:20-51,signed\"`"+`
	D int    `+"`dmr:\"bits:52-59\"`"+`
}`)
	assertContains(t, out,
		"result.A = int(int32(uint32(bit.Uint(data[0:12]))<<20) >> 20)",
		"result.B = Offset(int32(uint32(bit.Uint(data[12:20]))<<24) >> 24)",
		"result.C = int32(int32(uint32(bit.Uint(data[20:52]))))",
		"result.D = int(bit.Uint(data[52:60]))",
	)
}

func TestCoordinatesAreTwosComplementFloat64(t *testing.T) {
	out := mustGenerate(t, `type T struct {
	Lon float64 `+"`dmr:\"bits:0-24,type:longitude\"`"+`
	Lat float64 `+"`dmr:\"bits:25-48,type:latitude\"`"+`
}`)
	assertContains(t, out,
		"result.Lon = float64(int32(uint32(bit.Uint(data[0:25]))<<7)>>7) * 360.0 / (1 << 25)",
		"result.Lat = float64(int32(uint32(bit.Uint(data[25:49]))<<8)>>8) * 180.0 / (1 << 24)",
		"if !(s.Lon >= -180.0 && s.Lon <= 180.0) {",
		"_rawLon := math.Round(s.Lon * (1 << 25) / 360.0)",
		"bit.PutUint(data[0:25], uint64(int64(_rawLon)))",
		"if !(s.Lat >= -90.0 && s.Lat <= 90.0) {",
		"_rawLat := math.Round(s.Lat * (1 << 24) / 180.0)",
		"_rawLat = min(_rawLat, 8388607)",
	)
}

func TestNonContiguousRangesForAllIntegerKinds(t *testing.T) {
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
		"bit.PutUint(data[1:2], _valI>>3)",
		"bit.PutUint(data[10:13], _valS)",
		"bit.PutUint(data[3:4], _valE>>3)",
		"_valD := uint64(s.D)",
		"bit.PutUint(data[16:24], _valD>>4)",
	)
}

func TestDispatchIfAndFallback(t *testing.T) {
	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:dispatch_if FID==0
type T struct {
	Op  uint8       `+"`dmr:\"bits:0-7\"`"+`
	FID uint8       `+"`dmr:\"bits:8-15\"`"+`
	A   *Sub        `+"`dmr:\"bits:16-23,dispatch:Op=1\"`"+`
	Raw [1]byte     `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out,
		"if result.FID == 0 {",
		"\tdefault:\n\t\t\tfor _i := range 8 {\n\t\t\t\tresult.Raw[_i/8] |= byte(_dispatchBits[_i]) << (7 - _i%8)",
		"} else {\n\t\tfor _i := range 8 {\n\t\t\tresult.Raw[_i/8] |= byte(_dispatchBits[_i]) << (7 - _i%8)",
		"\tdefault:\n\t\tfor _i := range 8 {\n\t\t\tdata[16+_i] = bit.Bit(s.Raw[_i/8] >> (7 - _i%8) & 1)",
		`_ret += fmt.Sprintf("Raw: %x", s.Raw)`,
	)
	if strings.Count(out, "data[16+_i] = bit.Bit(s.Raw[_i/8]") != 1 {
		t.Error("fallback should only be encoded in the dispatch default branch")
	}
}

func TestEncodeChecksDispatchField(t *testing.T) {
	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:input Mode
// dmr:dispatch_if FID==0
type T struct {
	Mode bool       `+"`dmr:\"-\"`"+`
	Op   uint8      `+"`dmr:\"bits:0-7\"`"+`
	FID  uint8      `+"`dmr:\"bits:8-15\"`"+`
	A    *Sub       `+"`dmr:\"bits:16-23,dispatch:Op=1,when:Mode==true\"`"+`
	B    *Sub       `+"`dmr:\"bits:16-23,dispatch:Op=2\"`"+`
	Raw  [1]byte    `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out,
		`{"A", s.A != nil}`,
		`{"B", s.B != nil}`,
		`return [24]bit.Bit{}, fmt.Errorf("T: %s and %s are both set", _payload, _p.name)`,
		"\tif _payload != \"\" {\n\t\tvar result T\n\t\tresult.Op = uint8(bit.Uint(data[0:8]))\n\t\tresult.FID = uint8(bit.Uint(data[8:16]))\n\t\tresult.Mode = s.Mode\n\t\t_want := \"Raw\"",
		"\t\t\t\tif result.Mode {\n\t\t\t\t\t_want = \"A\"\n\t\t\t\t}",
		"\t\t\tcase 2:\n\t\t\t\t_want = \"B\"",
		`return [24]bit.Bit{}, fmt.Errorf("T.%s is set, but Op, FID and Mode select %s", _payload, _want)`,
	)
}

func TestGuardedDispatchFallsBackToRaw(t *testing.T) {
	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:input Mode
type T struct {
	Mode bool       `+"`dmr:\"-\"`"+`
	Op   uint8      `+"`dmr:\"bits:0-7\"`"+`
	K    uint8      `+"`dmr:\"bits:8-15\"`"+`
	A    *Sub       `+"`dmr:\"bits:16-23,dispatch:Op=1,when:K==2\"`"+`
	Raw  [1]byte    `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out, "if result.K == 2 {", "} else {\n\t\t\tfor _i := range 8 {\n\t\t\t\tresult.Raw[_i/8] |= byte(_dispatchBits[_i]) << (7 - _i%8)")
}

func TestEncodeReportsOutOfRangeValues(t *testing.T) {
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
		"func (s *T) Encode() ([60]bit.Bit, error) {",
		"if uint64(s.Part)>>12 != 0 {",
		`return [60]bit.Bit{}, &bit.FieldRangeError{`,
		"if uint64(s.Addr)>>24 != 0 {",
		"if int64(s.Off) < -128 || int64(s.Off) > 127 {",
		"_kidBits, err := s.Kid.Encode()",
		`return [60]bit.Bit{}, fmt.Errorf("T.Kid: %w", err)`,
		"return data, nil",
	)
	if strings.Contains(out, "uint64(s.Full)>>") {
		t.Error("a uint8 in an 8-bit field cannot overflow and needs no check")
	}
}

func TestConstIsEncoded(t *testing.T) {
	out := mustGenerate(t, `type Format uint8

const FormatX Format = 5

// dmr:const 0-3=FormatX
// dmr:const 4-5+10-11=0x9
type T struct {
	A uint8 `+"`dmr:\"bits:6-9\"`"+`
	B uint8 `+"`dmr:\"bits:12-15\"`"+`
}`)
	assertContains(t, out,
		"bit.PutUint(data[0:4], uint64(FormatX))",
		"_const1 := uint64(9)",
		"bit.PutUint(data[4:6], _const1>>2)",
		"bit.PutUint(data[10:12], _const1)",
	)
}

func TestInputsBecomeDecodeParameters(t *testing.T) {
	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:input Mode
type T struct {
	Mode bool  `+"`dmr:\"-\"`"+`
	Op   uint8 `+"`dmr:\"bits:0-7\"`"+`
	A    *Sub  `+"`dmr:\"bits:8-15,dispatch:Op=1,when:Mode==false\"`"+`
	B    *Sub  `+"`dmr:\"bits:8-15,dispatch:Op=1,when:Mode==true\"`"+`
}`)
	assertContains(t, out,
		"func DecodeT(data [16]bit.Bit, mode bool) T {",
		"_decoded := DecodeSub(_dispatchBits)",
		"result.Mode = mode",
		"if !result.Mode {",
		"} else if result.Mode {",
	)
}

func TestDecodeReportsFECInTheFECField(t *testing.T) {
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
		"func DecodeG(data [20]bit.Bit) G {",
		"corrected, result.FEC = golay.Decode2087(data)",
		"func DecodeC(data [16]bit.Bit) C {",
		"result.FEC.Uncorrectable = !crc.CheckCRC8(data[:8], _crc)",
		"func DecodeP(data [8]bit.Bit) P {",
	)
	if strings.Contains(out, "fecResult") {
		t.Errorf("generated code has a separate FEC result:\n%s", out)
	}
}

func TestRawFieldIsPackedMSBFirstWithZeroPadding(t *testing.T) {
	out := mustGenerate(t, `type T struct {
	A uint8   `+"`dmr:\"bits:0-3\"`"+`
	R [2]byte `+"`dmr:\"bits:4-15,raw\"`"+`
}`)
	assertContains(t, out,
		"for _i := range 12 {\n\t\tresult.R[_i/8] |= byte(data[4+_i]) << (7 - _i%8)",
		"if s.R[1]&15 != 0 {",
		`Field: "R",`,
		"for _i := range 12 {\n\t\tdata[4+_i] = bit.Bit(s.R[_i/8] >> (7 - _i%8) & 1)",
		`fmt.Sprintf("T{ A: %d, R: %x }", s.A, s.R)`,
	)
}

func TestEncodeBitsWrapsEncode(t *testing.T) {
	out := mustGenerate(t, `type T struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	assertContains(t, out,
		"func (s *T) EncodeBits() ([]bit.Bit, error) {\n\tbits, err := s.Encode()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn bits[:], nil\n}",
	)
}
