package shortdata_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

// ExampleDecodeHyteraText reads the H-SMS text that an AnyTone radio sent.
func ExampleDecodeHyteraText() {
	data, _ := hex.DecodeString(appcapture.HSMS)
	h, err := shortdata.DecodeHyteraText(data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%04x %s\n", h.Field, h.TextUTF8(nil))
	// Output:
	// 4500 HYT TEST
}

// ExampleDecodeText reads the text of a defined data packet with DD format
// UTF-16BE.
func ExampleDecodeText() {
	data, _ := shortdata.AppendText(nil, enums.DDUTF16BE, []byte("Hello"))
	text, err := shortdata.DecodeText(nil, enums.DDUTF16BE, append(data, 0, 0, 0, 0))
	fmt.Printf("%s %v\n", text, err)
	// Output:
	// Hello <nil>
}

// ExampleAppendText_codings writes a dialed number in BCD, a call sign in
// 7-bit text and a Greek word in ISO/IEC 8859-7, and reads them back.
func ExampleAppendText_codings() {
	for _, c := range []struct {
		format enums.DDFormat
		text   string
	}{
		{enums.DDBCD, "0123*#9"},
		{enums.DD7Bit, "KI5VMF"},
		{enums.DDISO8859Part7, "Γειά"},
	} {
		data, err := shortdata.AppendText(nil, c.format, []byte(c.text))
		if err != nil {
			fmt.Println(err)
			return
		}
		text, err := shortdata.DecodeText(nil, c.format, data)
		fmt.Printf("%s: % x %s %v\n", c.format, data, text, err)
	}
	// Output:
	// BCD: 01 23 ab 9f 0123*#9 <nil>
	// 7-bit: 97 25 ad 69 b1 bf KI5VMF <nil>
	// ISO 8859-7: c3 e5 e9 dc Γειά <nil>
}
