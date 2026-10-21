package emit_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const streamSrc = `// Head is a test frame.
// dmr:frame 1
type Head struct {
	Ext  bool  ` + "`dmr:\"bit:0\"`" + `
	Type uint8 ` + "`dmr:\"bits:1-7\"`" + `
}

// Msg is a test stream.
// dmr:stream
type Msg struct {
	Len  uint16 ` + "`dmr:\"u16,length\"`" + `
	Hd   Head   ` + "`dmr:\"frame\"`" + `
	Seq  uint8  ` + "`dmr:\"u8,if:Hd.Ext\"`" + `
	Name []byte ` + "`dmr:\"lv8,utf16le\"`" + `
	T    Toks   ` + "`dmr:\"tokens,if:Hd.Type==3\"`" + `
	Rest []byte ` + "`dmr:\"rest,if:Hd.Type!=3\"`" + `
}

// Toks is a test token set.
// dmr:tokens
type Toks struct {
	ID     []byte  ` + "`dmr:\"lvvar,token:0x22\"`" + `
	HasID  bool    ` + "`dmr:\"-\"`" + `
	Lat    float64 ` + "`dmr:\"i32,scale:90/31,token:0x66\"`" + `
	HasLat bool    ` + "`dmr:\"-\"`" + `
	OK     bool    ` + "`dmr:\"flag,token:0x38\"`" + `
}
`

func TestStreamCode(t *testing.T) {
	t.Parallel()

	pdus, err := parse.Source("test.go", header+streamSrc)
	if err != nil {
		t.Fatal(err)
	}
	parse.SetUnexportedDecode(pdus)
	out, err := render(t, pdus)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out,
		"func DecodeMsg(data []byte) (Msg, error) {",
		"func decodeMsgAt(data []byte, _o int) (Msg, int, error) {",
		"_end := _o + int(result.Len)",
		"data = data[:_end]",
		"if result.Hd.Ext {",
		"if result.Hd.Type == 3 {",
		"if result.Hd.Type != 3 {",
		"func decodeToksAt(data []byte, _o int) (Toks, int, error) {",
		"case 0x22:",
		"result.HasID = true",
		"return Toks{}, 0, ErrToken",
		"result.Lat = float64(int64((uint64(data[_o])<<24|uint64(data[_o+1])<<16|uint64(data[_o+2])<<8|uint64(data[_o+3]))<<32)>>32) * (90.0 / (1 << 31))",
		"func (s *Msg) AppendBinary(dst []byte) ([]byte, error) {",
		"func (s *Msg) NameUTF8(dst []byte) []byte {",
		"func (s *Msg) SetNameUTF8(dst, text []byte) []byte {",
		"if s.HasLat {",
		"if s.OK {",
	)
	if strings.Contains(out, "func DecodeToks(") {
		t.Errorf("a token set has an exported Decode\n%s", out)
	}
}
