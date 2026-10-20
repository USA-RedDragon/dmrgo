package emit_test

import (
	"strings"
	"testing"
)

const frameSrc = `import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Data is a DMRD packet.
// dmr:frame 53 55 71
// dmr:magic 0-31="DMRD"
type Data struct {
	Seq         uint8           ` + "`dmr:\"bits:32-39\"`" + `
	Source      uint32          ` + "`dmr:\"bits:40-63\"`" + `
	Destination uint32          ` + "`dmr:\"bits:64-87\"`" + `
	Repeater    uint32          ` + "`dmr:\"bits:88-119\"`" + `
	Slot        enums.Slot      ` + "`dmr:\"bits:120,enum,bias:1\"`" + `
	Private     bool            ` + "`dmr:\"bit:121\"`" + `
	Frame       uint8           ` + "`dmr:\"bits:122-123\"`" + `
	Value       uint8           ` + "`dmr:\"bits:124-127\"`" + `
	Stream      uint32          ` + "`dmr:\"bits:128-159\"`" + `
	Burst       layer2.RawBurst ` + "`dmr:\"bits:160-423,packed\"`" + `
	BER         uint8           ` + "`dmr:\"bits:424-431\"`" + `
	RSSI        uint8           ` + "`dmr:\"bits:432-439\"`" + `
	UUID        [16]byte        ` + "`dmr:\"bits:440-567,packed\"`" + `
	Size        uint8           ` + "`dmr:\"-\"`" + `
}

// Freq is a test frame.
// dmr:frame 16
type Freq struct {
	RX  uint32  ` + "`dmr:\"bits:0-31,le\"`" + `
	Dec uint32  ` + "`dmr:\"bits:32-71,dec\"`" + `
	Hex uint32  ` + "`dmr:\"bits:72-103,hex\"`" + `
	Pad uint8   ` + "`dmr:\"bits:104-111\"`" + `
	Sw  [2]byte ` + "`dmr:\"bits:112-127,packed,swap16\"`" + `
}

// Token is a test frame.
// dmr:frame 4
type Token struct {
	T [2]byte ` + "`dmr:\"bits:0-31,packed,hex\"`" + `
}

// configLayout is the layout.
// dmr:view Config 20
// dmr:magic 0-31="RPTC"
type configLayout struct {
	// ID is the repeater ID.
	ID uint32 ` + "`dmr:\"bits:32-63\"`" + `
	// Callsign is the callsign.
	Callsign [8]byte ` + "`dmr:\"bits:64-127,packed,text\"`" + `
	// Power is the power in dBm.
	Power uint8 ` + "`dmr:\"bits:128-143,dec\"`" + `
	// Slots is the slots digit.
	Slots uint16 ` + "`dmr:\"bits:144-159\"`" + `
}
`

func TestFrameDecodeChecksLengthAndMagic(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, frameSrc)
	assertContains(t, out,
		"func DecodeData(data []byte) (Data, error) {",
		"case 53, 55, 71:",
		"return Data{}, ErrShort",
		"return Data{}, ErrLength",
		"if data[0] != 0x44 || data[1] != 0x4D || data[2] != 0x52 || data[3] != 0x44 {",
		"return Data{}, ErrSignature",
		"result.Size = uint8(len(data))",
		"length is not one of these and ErrSignature when octets 0 to 3 do not hold",
	)
}

func TestFrameTailFieldsNeedTheirSize(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, frameSrc)
	assertContains(t, out,
		"if len(data) >= 55 {\n\t\tresult.BER = uint8(uint64(data[53]))\n\t\tresult.RSSI = uint8(uint64(data[54]))\n\t}",
		"if len(data) >= 71 {\n\t\tcopy(result.UUID[0:16], data[55:71])",
		"if _size >= 55 {\n\t\tdata[53] = byte(uint64(s.BER))",
		"case 0, 53:",
		"return dst, errDataSize",
		"for range _size {\n\t\tdst = append(dst, 0)\n\t}",
	)
}

func TestFrameEncodings(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, frameSrc)
	assertContains(t, out,
		"_vRX = uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24",
		"data[3] = byte(_vRX >> 24)",
		"if _c < '0' || _c > '9' {\n\t\t\treturn Freq{}, ErrText",
		"_vDec = _vDec*10 + uint64(_c-'0')",
		"data[_i] = '0' + byte(_vDec%10)",
		"if uint64(s.Dec) > uint64(0x1869f) {\n\t\treturn dst[:_n], errFreqDecRange",
		"_vHex = _vHex<<4 | uint64(_c-'a'+10)",
		"data[_i] = \"0123456789ABCDEF\"[_vHex&15]",
		"result.Sw[_i] = data[14+_i+1]",
		"data[14+_i] = s.Sw[_i+1]",
		"result.Slot = enums.Slot(_vSlot + 1)",
		"if uint64(s.Slot) < 1 || uint64(s.Slot) > 2 {",
		"_vSlot := uint64(s.Slot) - 1",
		"_v = _v<<4 | (_c - 'a' + 10)",
		"result.T[_i] = _v",
		"data[0+2*_i] = \"0123456789ABCDEF\"[s.T[_i]>>4]",
		"data[1+2*_i] = \"0123456789ABCDEF\"[s.T[_i]&15]",
	)
}

func TestViewMethods(t *testing.T) {
	t.Parallel()

	out := mustGenerate(t, frameSrc)
	assertContains(t, out,
		"func (c *Config) ID() uint32 {",
		"func (c *Config) SetID(v uint32) {",
		"func (c *Config) Callsign() []byte {",
		"return c[8:_e]",
		"func (c *Config) SetCallsign(v []byte) error {",
		"return errConfigCallsignRange",
		"func (c *Config) Power() (uint8, error) {",
		"return 0, ErrText",
		"func (c *Config) SetPower(v uint8) error {",
		"func (c *Config) Reset() {",
		"func (c *Config) magicOK() bool {",
		"// Power returns the power in dBm. It returns ErrText when the field holds a",
	)
	if strings.Contains(out, "configLayout") {
		t.Errorf("generated view code names the layout struct\n%s", out)
	}
}

func TestViewFieldNeedsDoc(t *testing.T) {
	t.Parallel()

	_, err := generate(t, "// dmr:view V 1\ntype layout struct {\n\tA uint8 `dmr:\"bits:0-7\"`\n}")
	if err == nil || !strings.Contains(err.Error(), "needs a doc comment") {
		t.Fatalf("got %v", err)
	}
}
