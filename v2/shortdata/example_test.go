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

// ExampleText reads the text of a defined data packet with DD format
// UTF-16BE.
func ExampleText() {
	data, _ := shortdata.AppendText(nil, enums.DDUTF16BE, []byte("Hello"))
	text, err := shortdata.Text(nil, enums.DDUTF16BE, append(data, 0, 0, 0, 0))
	fmt.Printf("%s %v\n", text, err)
	// Output:
	// Hello <nil>
}
