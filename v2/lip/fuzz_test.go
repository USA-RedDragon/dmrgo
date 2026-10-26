package lip_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/lip"
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(data []byte)) {
	r := lip.LongReport{
		Time:     lip.Time{Type: lip.TimePosition, Day: 3, Hour: 4, Minute: 5, Second: 6},
		Location: lip.Location{Shape: lip.ShapeEllipseAltitudeUncertainty, Latitude: 12.5, Longitude: -3.25, HalfMajor: 9, Altitude: 300},
		Velocity: lip.Velocity{Type: lip.VelocityAll, Horizontal: 40, Direction: 90},
	}
	p, n, _ := r.Encode(nil)
	p, _, _ = lip.AppendElement(p, n, lip.ElementMessageReference, []byte{7}, 8)
	add(p)
	q, n, _ := (&lip.ImmediateRequest{ReportType: lip.ReportLongElapsed}).Encode(nil)
	q, _, _ = lip.AppendElement(q, n, lip.ElementExtendedUserData, make([]byte, 9), 72)
	add(q)
	a, _, _ := (&lip.ReportAck{}).Encode(nil)
	add(a)
	e, _, _ := (&lip.EnableDisable{AckRequested: true, Flags: 0x80}).Encode(nil)
	add(e)
	add([]byte{0x00, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x10})
}

func fuzzDecode(t *testing.T, data []byte) {
	h, err := lip.DecodeHeader(data)
	if err != nil || h.Short {
		return
	}
	var elements int
	var out []byte
	var n int
	switch {
	case h.Extension == lip.ExtensionLongReport:
		r, err := lip.DecodeLongReport(data)
		if err != nil {
			return
		}
		out, n, err = r.Encode(nil)
		if err != nil {
			t.Fatalf("DecodeLongReport(%x) = %+v does not encode: %v", data, r, err)
		}
		back, err := lip.DecodeLongReport(out)
		if err != nil || back.Elements != n || n != r.Elements {
			t.Fatalf("DecodeLongReport(%x) re-decodes to %+v %v", data, back, err)
		}
		elements = r.Elements
	case h.Extension == lip.ExtensionReportAck:
		a, err := lip.DecodeReportAck(data)
		if err != nil {
			return
		}
		elements = a.Elements
		out, n, err = a.Encode(nil)
		if err != nil {
			t.Fatal(err)
		}
	case h.Extension == lip.ExtensionImmediateReport && !h.Response:
		q, err := lip.DecodeImmediateRequest(data)
		if err != nil {
			return
		}
		elements = q.Elements
		out, n, err = q.Encode(nil)
		if err != nil {
			t.Fatal(err)
		}
	case h.Extension == lip.ExtensionEnableDisable:
		e, err := lip.DecodeEnableDisable(data)
		if err != nil {
			return
		}
		elements = e.Elements
		out, n, err = e.Encode(nil)
		if err != nil {
			t.Fatal(err)
		}
	default:
		return
	}
	if n != elements || !bytes.Equal(out[:n/8], data[:n/8]) || (n%8 != 0 && out[n/8]>>(8-n%8) != data[n/8]>>(8-n%8)) {
		t.Fatalf("%x encodes to %x, %d bits, elements at %d", data, out, n, elements)
	}
	for at := elements; ; {
		e, ok, err := lip.NextElement(data, at)
		if err != nil || !ok {
			return
		}
		if e.Next <= at || e.Next > len(data)*8 {
			t.Fatalf("NextElement(%x, %d) = %+v", data, at, e)
		}
		_ = e.Uint(data)
		_ = e.AppendData(nil, data)
		at = e.Next
	}
}
