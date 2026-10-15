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
		"result.A = Wide(bit.BitsToUint16(data[:], 2, 12))",
		"bit.BitsFromUint16(uint16(s.A), 12)",
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
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	if err == nil || !strings.Contains(err.Error(), "unknown FEC codec") {
		t.Fatalf("expected unknown FEC codec error, got %v", err)
	}
}

func TestFieldsBeyondFECDataBitsAreRejected(t *testing.T) {
	_, err := generate(t, `// dmr:fec golay_20_8_7
type T struct {
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
		"result.A = int(int32(bit.BitsToUint32(data[:], 0, 12)<<20) >> 20)",
		"result.B = Offset(int32(bit.BitsToUint32(data[:], 12, 8)<<24) >> 24)",
		"result.C = int32(int32(bit.BitsToUint32(data[:], 20, 32)))",
		"result.D = bit.BitsToInt(data[:], 52, 8)",
	)
}

func TestCoordinatesAreTwosComplementFloat64(t *testing.T) {
	out := mustGenerate(t, `type T struct {
	Lon float64 `+"`dmr:\"bits:0-24,type:longitude\"`"+`
	Lat float64 `+"`dmr:\"bits:25-48,type:latitude\"`"+`
}`)
	assertContains(t, out,
		"result.Lon = float64(int32(bit.BitsToUint32(data[:], 0, 25)<<7)>>7) * 360.0 / (1 << 25)",
		"result.Lat = float64(int32(bit.BitsToUint32(data[:], 25, 24)<<8)>>8) * 180.0 / (1 << 24)",
		"if !(s.Lon >= -180.0 && s.Lon <= 180.0) {",
		"_rawLon := math.Round(s.Lon * (1 << 25) / 360.0)",
		"copy(data[0:25], bit.BitsFromUint32(uint32(int32(_rawLon)), 25))",
		"if !(s.Lat >= -90.0 && s.Lat <= 90.0) {",
		"_rawLat := math.Round(s.Lat * (1 << 24) / 180.0)",
		"_rawLat = min(_rawLat, 8388607)",
	)
}

func TestNonContiguousRangesForAllIntegerKinds(t *testing.T) {
	out := mustGenerate(t, `type Kind uint8
func KindFromInt(i int) Kind { return Kind(i) }
func KindToName(k Kind) string { return "" }
type Wrapped uint16
func WrappedToName(w Wrapped) string { return "" }
type T struct {
	U uint8   `+"`dmr:\"bits:0+4-6\"`"+`
	I int     `+"`dmr:\"bits:1+7-9\"`"+`
	S int     `+"`dmr:\"bits:2+10-12,signed\"`"+`
	E Kind    `+"`dmr:\"bits:3+13-15,enum\"`"+`
	D Wrapped `+"`dmr:\"bits:16-23+24-27,delegate,noptr\"`"+`
}`)
	assertContains(t, out,
		"_tmpU <<= 3",
		"result.U = _tmpU",
		"result.I = int(_tmpI)",
		"result.S = int(int32(uint32(_tmpS)<<28) >> 28)",
		"result.E = KindFromInt(int(_tmpE))",
		"result.D = Wrapped(_tmpD)",
		"_valI := uint8(s.I)",
		"copy(data[1:2], bit.BitsFromUint8(_valI>>3, 1))",
		"copy(data[10:13], bit.BitsFromUint8(_valS, 3))",
		"copy(data[3:4], bit.BitsFromUint8(_valE>>3, 1))",
		"_valD := uint16(s.D)",
		"copy(data[16:24], bit.BitsFromUint16(_valD>>4, 8))",
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
	Raw [8]bit.Bit  `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out,
		"if result.FID == 0 {",
		"\tdefault:\n\t\t\tcopy(result.Raw[:], _dispatchBits[:])",
		"} else {\n\t\tcopy(result.Raw[:], _dispatchBits[:])",
		"\tdefault:\n\t\tcopy(data[16:24], s.Raw[:])",
		`_ret += fmt.Sprintf("Raw: %v", s.Raw)`,
	)
	if strings.Count(out, "copy(data[16:24], s.Raw[:])") != 1 {
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
	Raw  [8]bit.Bit `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out,
		`{"A", s.A != nil}`,
		`{"B", s.B != nil}`,
		`return [24]bit.Bit{}, fmt.Errorf("T: %s and %s are both set", _payload, _p.name)`,
		"\tif _payload != \"\" {\n\t\tvar result T\n\t\tresult.Op = bit.BitsToUint8(data[:], 0, 8)\n\t\tresult.FID = bit.BitsToUint8(data[:], 8, 8)\n\t\tresult.Mode = s.Mode\n\t\t_want := \"Raw\"",
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
	Raw  [8]bit.Bit `+"`dmr:\"bits:16-23,raw,fallback\"`"+`
}`)
	assertContains(t, out, "if result.K == 2 {", "} else {\n\t\t\tcopy(result.Raw[:], _dispatchBits[:])")
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
		"func EncodeT(s *T) ([60]bit.Bit, error) {",
		"if uint64(s.Part)>>12 != 0 {",
		`return [60]bit.Bit{}, fmt.Errorf("T.Part: value %d does not fit in 12 bits", s.Part)`,
		"if uint64(s.Addr)>>24 != 0 {",
		"if int64(s.Off) < -128 || int64(s.Off) > 127 {",
		"_kidBits, err := EncodeSub(&s.Kid)",
		`return [60]bit.Bit{}, fmt.Errorf("T.Kid: %w", err)`,
		"return data, nil",
	)
	if strings.Contains(out, "uint64(s.Full)") {
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
		"copy(data[0:4], bit.BitsFromUint8(uint8(FormatX), 4))",
		"_const1 := uint8(9)",
		"copy(data[4:6], bit.BitsFromUint8(_const1>>2, 2))",
		"copy(data[10:12], bit.BitsFromUint8(_const1, 2))",
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
		"func DecodeT(data [16]bit.Bit, mode bool) (T, fec.FECResult) {",
		"result.Mode = mode",
		"if !result.Mode {",
		"} else if result.Mode {",
	)
}
