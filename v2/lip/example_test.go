package lip_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/lip"
)

// ExampleDecodeLongReport builds a long location report with a location
// message reference, as a radio sends it in a UDT, and reads it back.
func ExampleDecodeLongReport() {
	r := lip.LongReport{
		Time:     lip.Time{Type: lip.TimeElapsed},
		Location: lip.Location{Shape: lip.ShapeCircle, Latitude: 51.5007, Longitude: -0.1246, Uncertainty: 10},
		Velocity: lip.Velocity{Type: lip.VelocityHorizontalDirection, Horizontal: 20, Direction: 64},
	}
	pdu, n, _ := r.Encode(nil)
	pdu, _, _ = lip.AppendElement(pdu, n, lip.ElementMessageReference, []byte{42}, 8)

	h, _ := lip.DecodeHeader(pdu)
	got, err := lip.DecodeLongReport(pdu)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%v: %v %.4f %.4f, speed code %d\n", h.Extension, got.Location.Shape, got.Location.Latitude, got.Location.Longitude, got.Velocity.Horizontal)
	e, ok, _ := lip.NextElement(pdu, got.Elements)
	fmt.Println(ok, e.ID, e.Uint(pdu))
	// Output:
	// Long location report: Location circle 51.5007 -0.1246, speed code 20
	// true Location message reference 42
}
