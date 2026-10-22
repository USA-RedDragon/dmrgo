package trunk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestUDT_AppendedFromFields(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"hello", "a text that needs more than one block of data"} {
		var a trunk.Appended
		if err := a.SetText(enums.UDTISO8, []byte(text)); err != nil {
			t.Fatal(err)
		}
		u := trunk.UDT{Header: pdu.UDTHeader{UDTFormat: a.Format, PadNibble: a.PadNibble, UAB: uint8(a.Blocks - 1)}, Data: a.Data}
		got := u.Appended()
		if s, err := got.Text(nil); err != nil || string(s) != text || got.Blocks != a.Blocks {
			t.Errorf("%q: Text %q, %v, blocks %d", text, s, err, got.Blocks)
		}
	}
}
