package emit_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const selectorSrc = `type Sub struct {
	X uint8 ` + "`dmr:\"bits:0-7\"`" + `
}

// dmr:dispatch_if FID==0
// dmr:variant A Sub Op=1
// dmr:variant B Sub Op=2|3 FID==5
type T struct {
	LB  bool    ` + "`dmr:\"bit:0\"`" + `
	Op  uint8   ` + "`dmr:\"bits:1-7\"`" + `
	FID uint8   ` + "`dmr:\"bits:8-15\"`" + `
	Raw [1]byte ` + "`dmr:\"bits:16-23,raw,union\"`" + `
}
`

const getterSrc = `type Sub16 struct {
	Y uint16 ` + "`dmr:\"bits:0-15\"`" + `
}

// dmr:input Mode
// dmr:get Kind uint8 bits:0-3 the kind of the payload
// dmr:get Count uint8 bits:4-5+12-15,only:Kind=1|2 the count of the payload
// dmr:get Flag bool bit:6 the flag of the payload
// dmr:variant X Sub16 Kind=1
// dmr:variant Y Sub16 Kind=2 Count==0 Mode==true
// dmr:variant Z Sub16 Kind=2 Count!=0 Flag==false
type G struct {
	Mode bool    ` + "`dmr:\"-\"`" + `
	Raw  [2]byte ` + "`dmr:\"bits:0-15,raw,union\"`" + `
}
`

func generateUnion(t *testing.T, src string) string {
	t.Helper()
	pdus, err := parse.Source("test.go", header+src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := parse.SetUnexportedDecode(pdus); err != nil {
		t.Fatalf("SetUnexportedDecode: %v", err)
	}
	out, err := render(t, pdus)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnionAsCondition(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, selectorSrc)
	assertContains(t, out,
		"// AsA decodes Raw as a Sub payload. ok is false unless Op is 1 and FID is 0.\n"+
			"func (s *T) AsA() (p Sub, ok bool) {\n\tif !(s.Op == 1 && s.FID == 0) {\n\t\treturn p, false\n\t}\n\treturn decodeSub(s.Raw), true\n}",
		"// AsB decodes Raw as a Sub payload. ok is false unless Op is 2 or 3 and FID\n// is 5.\n"+
			"func (s *T) AsB() (p Sub, ok bool) {\n\tif !((s.Op == 2 || s.Op == 3) && s.FID == 5) {\n\t\treturn p, false\n\t}\n\treturn decodeSub(s.Raw), true\n}",
		"func DecodeT(data [3]byte) T {",
		"result.Raw[0] = data[2]\n\treturn result\n}",
		"data[2] = s.Raw[0]\n\treturn data, nil\n}",
	)
	for _, removed := range []string{"Payloads", "Into(", "_dispatchBytes", "errTPayload"} {
		if strings.Contains(out, removed) {
			t.Errorf("generated code has removed %q\n%s", removed, out)
		}
	}
}

func TestUnionSetSingleValue(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, selectorSrc)
	assertContains(t, out,
		"// SetA makes s carry p as a Sub payload: it sets Op to 1, FID to 0 and Raw,\n"+
			"// and keeps the other fields. It returns a *dmrgo.FieldRangeError when a\n// field of p does not fit its bits, and then leaves s as it is.\n"+
			"func (s *T) SetA(p Sub) error {\n\traw, err := p.encode()\n\tif err != nil {\n\t\treturn err\n\t}\n"+
			"\ts.Op = 1\n\ts.FID = 0\n\ts.Raw = raw\n\treturn nil\n}",
	)
}

func TestUnionSetMultiValueSelector(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, selectorSrc)
	assertContains(t, out,
		"func (s *T) SetB(op uint8, p Sub) error {\n\tif !(op == 2 || op == 3) {\n\t\treturn ErrWrongOpcode\n\t}\n"+
			"\traw, err := p.encode()\n\tif err != nil {\n\t\treturn err\n\t}\n"+
			"\ts.Op = op\n\ts.FID = 5\n\ts.Raw = raw\n\treturn nil\n}",
	)
}

func TestUnionSetParameterDoesNotShadowLocals(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:variant A Sub K==3 Raw=1|2
// dmr:variant B Sub K==4 S=1|2
// dmr:variant C Sub K==5 Err=1|2
// dmr:variant D Sub T=1|2 K==1
// dmr:variant E Sub P=1|2 K==2
type U struct {
	Raw uint8   `+"`dmr:\"bits:0-1\"`"+`
	S   uint8   `+"`dmr:\"bits:2-3\"`"+`
	Err uint8   `+"`dmr:\"bits:4-5\"`"+`
	T   uint8   `+"`dmr:\"bits:6-7\"`"+`
	P   uint8   `+"`dmr:\"bits:8-9\"`"+`
	K   uint8   `+"`dmr:\"bits:10-15\"`"+`
	Pay [1]byte `+"`dmr:\"bits:16-23,raw,union\"`"+`
}`)
	assertContains(t, out,
		"func (s *U) SetA(_raw uint8, p Sub) error {",
		"s.Raw = _raw\n\ts.Pay = raw",
		"func (s *U) SetB(_s uint8, p Sub) error {",
		"s.S = _s",
		"func (s *U) SetC(_err uint8, p Sub) error {",
		"s.Err = _err",
		"func (s *U) SetD(_t uint8, p Sub) error {",
		"s.T = _t\n\ts.K = 1",
		"func (s *U) SetE(_p uint8, p Sub) error {",
		"s.P = _p\n\ts.K = 2",
	)
}

func TestUnionGetters(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, getterSrc)
	assertContains(t, out,
		"// Kind returns the kind of the payload.\nfunc (s *G) Kind() uint8 {\n\treturn uint8(uint64(s.Raw[0]) >> 4)\n}",
		"// Count returns the count of the payload. It returns 0 unless Kind is 1 or\n// 2.\n"+
			"func (s *G) Count() uint8 {\n\tif !(s.Kind() == 1 || s.Kind() == 2) {\n\t\treturn 0\n\t}\n"+
			"\tv := uint64(s.Raw[0]) >> 2 & 0x3\n\tv <<= 4\n\tv |= uint64(s.Raw[1]) & 0xF\n\treturn uint8(v)\n}",
		"// Flag returns the flag of the payload.\nfunc (s *G) Flag() bool {\n\treturn s.Raw[0]&0x02 != 0\n}",
		"if !(s.Kind() == 1) {",
		"// AsY decodes Raw as a Sub16 payload. ok is false unless Kind is 2, Count is\n// 0 and Mode is true.\n",
		"if !(s.Kind() == 2 && s.Count() == 0 && s.Mode) {",
		"if !(s.Kind() == 2 && s.Count() != 0 && !s.Flag()) {",
		"func DecodeG(data [2]byte, mode bool) G {\n\tvar result G\n\tresult.Mode = mode\n",
	)
}

func TestUnionSetChecksGetterCondition(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, getterSrc)
	assertContains(t, out,
		"func (s *G) SetZ(p Sub16) error {\n\traw, err := p.encode()\n\tif err != nil {\n\t\treturn err\n\t}\n"+
			"\tt := *s\n\tt.Raw = raw\n\tif _, ok := t.AsZ(); !ok {\n\t\treturn errGZPayload\n\t}\n\t*s = t\n\treturn nil\n}",
		"func (s *G) SetY(p Sub16) error {",
		"if _, ok := t.AsY(); !ok {\n\t\treturn errGYPayload\n\t}",
		`errors.New("G.SetZ: the payload does not meet the conditions of AsZ")`,
	)
	if strings.Contains(out, "s.Mode =") || strings.Contains(out, "s.Kind =") {
		t.Errorf("Set writes an input or a getter\n%s", out)
	}
}

func TestUnionString(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, selectorSrc+getterSrc)
	assertContains(t, out,
		"func (s *T) String() string {\n\t_ret := \"T{ \"\n"+
			"\t_ret += \"LB: \" + text.Bool(s.LB) + \", Op: \" + text.Int(int64(s.Op)) + \", FID: \" + text.Int(int64(s.FID)) + \", \"\n"+
			"\tif v, ok := s.AsA(); ok {\n\t\t_ret += v.String()\n"+
			"\t} else if v, ok := s.AsB(); ok {\n\t\t_ret += v.String()\n"+
			"\t} else {\n\t\t_ret += \"Raw: \" + text.Hex(s.Raw[:])\n\t}\n"+
			"\t_ret += \" }\"\n\treturn _ret\n}",
		"\t_ret += \"Mode: \" + text.Bool(s.Mode) + \", \"\n"+
			"\tif v, ok := s.AsX(); ok {",
		"} else if v, ok := s.AsZ(); ok {\n\t\t_ret += v.String()\n\t} else {\n\t\t_ret += \"Raw: \" + text.Hex(s.Raw[:])\n\t}",
	)
}

func TestUnionStringAfterStrideField(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:variant A Sub Op=1
type U struct {
	Op  uint8   `+"`dmr:\"bits:0-7\"`"+`
	Arr [2]Sub  `+"`dmr:\"bits:8-23,delegate,stride:8\"`"+`
	Pay [1]byte `+"`dmr:\"bits:24-31,raw,union\"`"+`
}`)
	assertContains(t, out,
		"\t_ret += \"Op: \" + text.Int(int64(s.Op))\n\t_ret += \", Arr: [\"\n",
		"\t_ret += \"]\"\n\t_ret += \", \"\n\tif v, ok := s.AsA(); ok {",
		"} else {\n\t\t_ret += \"Pay: \" + text.Hex(s.Pay[:])\n\t}",
	)
	if strings.Contains(out, `", " + ", `) || strings.Contains(out, "_ret += \", \"\n\t_ret += \", ") {
		t.Errorf("String writes two separators\n%s", out)
	}
}

func TestStrideFieldStringHasOneSeparator(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

type U struct {
	Op  uint8  `+"`dmr:\"bits:0-7\"`"+`
	Arr [2]Sub `+"`dmr:\"bits:8-23,delegate,stride:8\"`"+`
	B   uint8  `+"`dmr:\"bits:24-31\"`"+`
}`)
	assertContains(t, out,
		"\t_ret := \"U{ \"\n\t_ret += \"Op: \" + text.Int(int64(s.Op))\n\t_ret += \", Arr: [\"\n",
		"\t_ret += \"]\"\n\t_ret += \", B: \" + text.Int(int64(s.B))\n\t_ret += \" }\"\n",
	)
}

func TestSubPDUDecodeIsUnexported(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, `type Kind uint8
func (o Kind) String() string { return "" }
type Sub struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}
//dmr:subpdu
type Marked struct {
	B uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:variant Inner Sub Op=1
type Top struct {
	Op  Kind    `+"`dmr:\"bits:0-7,enum\"`"+`
	Raw [1]byte `+"`dmr:\"bits:8-15,raw,union\"`"+`
}`)
	assertContains(t, out, "func decodeSub(", "func decodeMarked(", "func DecodeTop(", "return decodeSub(s.Raw), true")
	if strings.Contains(out, "func DecodeSub(") || strings.Contains(out, "func DecodeMarked(") {
		t.Errorf("sub-PDU Decode is exported\n%s", out)
	}
}

func TestNoUnionMethodsWithoutUnion(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `type T struct {
	A uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	for _, s := range []string{"Payloads", "Into(", ") As", ") Set"} {
		if strings.Contains(out, s) {
			t.Errorf("PDU without a union has %q:\n%s", s, out)
		}
	}
}

func TestInputNamedPIsRenamed(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, `// dmr:input P
type T struct {
	P  bool  `+"`dmr:\"-\"`"+`
	Op uint8 `+"`dmr:\"bits:0-7\"`"+`
}`)
	assertContains(t, out,
		"func DecodeT(data [1]byte, _p bool) T {",
		"result.P = _p",
		`return "T{ P: " + text.Bool(s.P) + ", Op: " + text.Int(int64(s.Op)) + " }"`,
	)
}

func TestUnionSetChecksPayloadField(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, `// dmr:const 0-3=1
type SubA struct {
	Count uint8 `+"`dmr:\"bits:4-7\"`"+`
	X     uint8 `+"`dmr:\"bits:8-15\"`"+`
}

// dmr:get Kind uint8 bits:0-3 the kind of the payload
// dmr:get Count uint8 bits:4-7 the count of the payload
// dmr:variant A SubA Kind=1 Count!=0
// dmr:variant B SubA Kind=2
type U struct {
	Raw [2]byte `+"`dmr:\"bits:0-15,raw,union\"`"+`
}`)
	assertContains(t, out,
		"func (s *U) SetA(p SubA) error {\n\tif p.Count < 1 || p.Count > 15 {\n\t\treturn errUSetACount\n\t}\n\traw, err := p.encode()\n\tif err != nil {\n\t\treturn err\n\t}\n\ts.Raw = raw\n\treturn nil\n}",
		"errUSetACount = &dmrgo.FieldRangeError{",
		"Max:   15.0,",
		"Min:   1.0,",
		"Type:  \"SubA\",",
		"or p.Count is 0",
		"if _, ok := t.AsB(); !ok {",
	)
}

func TestFECMaskExportedError(t *testing.T) {
	t.Parallel()

	pdus, err := parse.Source("test.go", header+`// dmr:fec reed_solomon_12_9_4
// dmr:input DT
// dmr:fec_mask DT err:ErrNoMask 1=0x969696
type T struct {
	DT  uint8      `+"`dmr:\"-\"`"+`
	FEC fec.Result `+"`dmr:\"-\"`"+`
	A   [9]byte    `+"`dmr:\"bits:0-71,raw\"`"+`
}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := render(t, pdus)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "return [12]byte{}, ErrNoMask")
	if strings.Contains(out, "errTDTMask") {
		t.Errorf("generated a mask error beside err:ErrNoMask\n%s", out)
	}
}

func TestUnionResetAndStringAs(t *testing.T) {
	t.Parallel()

	out := generateUnion(t, `type Sub struct {
	X uint8 `+"`dmr:\"bits:0-7\"`"+`
}

// dmr:reset H=HNone
// dmr:string_as Extra
// dmr:variant A Sub Op=1
// dmr:variant B Sub Op=2
type T struct {
	H   uint8   `+"`dmr:\"-\"`"+`
	Op  uint8   `+"`dmr:\"bits:0-7\"`"+`
	Raw [1]byte `+"`dmr:\"bits:8-15,raw,union\"`"+`
}`)
	assertContains(t, out,
		"\ts.Op = 1\n\ts.H = HNone\n\ts.Raw = raw\n",
		"H to HNone",
		"\ts.Op = 2\n\ts.H = HNone\n\ts.Raw = raw\n",
		"} else if v, ok := s.AsExtra(); ok {",
	)
}
