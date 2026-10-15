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
	K  uint8 `+"`dmr:\"bits:8-9\"`"+`
	A  *Sub  `+"`dmr:\"bits:16-23,dispatch:Op=1,when:K==0\"`"+`
	B  *Sub  `+"`dmr:\"bits:16-23,dispatch:Op=1\"`"+`
}`)
	assertContains(t, out, "if result.K == 0 {", "} else {")
}
