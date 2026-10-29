package lip_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/lip"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
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

// ExampleDecodeImmediateRequest requests a long location report with the
// time of the position in the appended data of a UDT, answers it as a
// radio does, and reads the answer.
func ExampleDecodeImmediateRequest() {
	q := lip.ImmediateRequest{ReportType: lip.ReportLongPosition}
	req, _, err := q.Encode(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	var a trunk.Appended
	if err := a.SetLIP(req); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(a.Format, a.Blocks, "block")

	got, err := a.LIP(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	h, err := lip.DecodeHeader(got)
	if err != nil || h.Extension != lip.ExtensionImmediateReport {
		fmt.Println(h.Extension, err)
		return
	}
	in, err := lip.DecodeImmediateRequest(got)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("request:", in.ReportType)

	r := lip.LongReport{
		Time:     lip.Time{Type: lip.TimePosition, Day: 2, Hour: 20, Minute: 0, Second: 37},
		Location: lip.Location{Shape: lip.ShapePoint, Latitude: 34.212166, Longitude: 108.833333},
	}
	answer, _, err := r.Encode(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := a.SetLIP(answer); err != nil {
		fmt.Println(err)
		return
	}
	got, err = a.LIP(got[:0])
	if err != nil {
		fmt.Println(err)
		return
	}
	rep, err := lip.DecodeLongReport(got)
	if err != nil {
		fmt.Println(err)
		return
	}
	t := rep.Time
	fmt.Printf("answer: %v %.4f %.4f, day %d %02d:%02d:%02d UTC\n", rep.Location.Shape, rep.Location.Latitude, rep.Location.Longitude, t.Day, t.Hour, t.Minute, t.Second)
	// Output:
	// LIP 1 block
	// request: Long location report with time of position
	// answer: Location point 34.2122 108.8333, day 2 20:00:37 UTC
}
