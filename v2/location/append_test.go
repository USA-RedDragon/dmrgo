package location_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

type airedPacket struct {
	header pdu.UnconfirmedDataHeader
	blocks [][12]byte
}

func airedPositions(t *testing.T) []airedPacket {
	t.Helper()
	f, err := os.Open("../layer2/testdata/anytone_aprs.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []airedPacket
	var b layer2.Burst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		raw, err := hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := b.DecodeFromBytes(layer2.RawBurst(raw)); err != nil {
			t.Fatal(err)
		}
		switch d := b.Data.(type) {
		case *pdu.DataHeader:
			u, ok := d.AsUnconfirmed()
			if !ok {
				t.Fatalf("header %s", d.String())
			}
			out = append(out, airedPacket{header: u})
		case *pdu.Rate12Data:
			if len(out) == 0 {
				t.Fatal("block before a header")
			}
			out[len(out)-1].blocks = append(out[len(out)-1].blocks, d.Data)
		}
	}
	return out
}

func TestAppendPacketMatchesAnyTone(t *testing.T) {
	t.Parallel()

	aired := airedPositions(t)
	ms := aprsMessages(t)
	if len(aired) != 10 || len(ms) != 10 {
		t.Fatalf("%d aired packets and %d messages, want 10", len(aired), len(ms))
	}
	var d location.Decoder
	for i := range ms {
		f, err := d.Decode(&ms[i].Header, ms[i].Data)
		if err != nil {
			t.Fatal(err)
		}
		var h location.Header
		data, err := location.AppendPacket([]byte("x"), &f, &h)
		if err != nil || data[0] != 'x' || string(data[1:]) != string(ms[i].Data) {
			t.Fatalf("packet %d: %q, %v, want %q", i, data, err, ms[i].Data)
		}
		p, err := packet.NewPacket(h.Data, data[1:], enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		u, ok := p.Header.AsUnconfirmed()
		if !ok || u != aired[i].header {
			t.Fatalf("packet %d: header %+v, want %+v", i, u, aired[i].header)
		}
		if len(p.Blocks) != len(aired[i].blocks) {
			t.Fatalf("packet %d: %d blocks, want %d", i, len(p.Blocks), len(aired[i].blocks))
		}
		for k, blk := range p.Blocks {
			if r, ok := blk.(*pdu.Rate12Data); !ok || r.Data != aired[i].blocks[k] {
				t.Errorf("packet %d block %d: %v, want %x", i, k, blk, aired[i].blocks[k])
			}
		}
	}
}

func TestAppendPacketRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []location.Fix{
		{Format: location.FormatNMEA, Source: 5, Destination: 7, Valid: true, Latitude: -33.8568, Longitude: 151.2153,
			Speed: 12.5, HasSpeed: true, Course: 270.25, HasCourse: true,
			Year: 2026, Month: 10, Day: 28, Hour: 9, Minute: 5, Second: 60, HasTime: true},
		{Format: location.FormatNMEA, Source: 16777215, Destination: 1, Valid: true, Latitude: 90, Longitude: -180},
		{Format: location.FormatNMEA, Source: 5, Destination: 7, HasTime: true, Year: 2026, Month: 1, Day: 31},
	}
	var d location.Decoder
	for i, f := range tests {
		var h location.Header
		data, err := location.AppendPacket(nil, &f, &h)
		if err != nil {
			t.Fatalf("fix %d: %v", i, err)
		}
		g, err := d.Decode(&h.Data, data)
		if !f.Valid {
			if !errors.Is(err, location.ErrNoPosition) || g.Valid || g.Format != location.FormatNMEA || g.Source != f.Source ||
				g.HasTime != f.HasTime || g.Day != f.Day {
				t.Errorf("fix %d: %+v, %v", i, g, err)
			}
			continue
		}
		if err != nil || g.Format != f.Format || g.Source != f.Source || g.Destination != f.Destination || !g.Valid ||
			!near(g.Latitude, f.Latitude, 1e-6) || !near(g.Longitude, f.Longitude, 1e-6) ||
			g.HasSpeed != f.HasSpeed || !near(g.Speed, f.Speed, 0.005) || g.HasCourse != f.HasCourse || !near(g.Course, f.Course, 0.005) ||
			g.HasTime != f.HasTime || g.Year != f.Year || g.Month != f.Month || g.Day != f.Day || g.Hour != f.Hour ||
			g.Minute != f.Minute || g.Second != f.Second {
			t.Errorf("fix %d: %+v, %v, want %+v", i, g, err, f)
		}
	}
}

func TestAppendPacketErrors(t *testing.T) {
	t.Parallel()

	var h location.Header
	tests := []struct {
		name string
		f    location.Fix
		want error
	}{
		{"LRRP", location.Fix{Format: location.FormatLRRP, Valid: true}, location.ErrFormat},
		{"no format", location.Fix{}, location.ErrFormat},
		{"latitude", location.Fix{Format: location.FormatNMEA, Valid: true, Latitude: 91}, nmea.ErrField},
		{"speed", location.Fix{Format: location.FormatNMEA, Speed: -1, HasSpeed: true}, nmea.ErrField},
		{"month", location.Fix{Format: location.FormatNMEA, HasTime: true, Day: 1}, nmea.ErrField},
		{"radio ID", location.Fix{Format: location.FormatNMEA, Source: 1 << 24}, nil},
	}
	for _, tt := range tests {
		out, err := location.AppendPacket([]byte("x"), &tt.f, &h)
		if tt.want == nil {
			if err == nil || string(out) != "x" {
				t.Errorf("%s: %q, %v, want an error", tt.name, out, err)
			}
			continue
		}
		if !errors.Is(err, tt.want) || string(out) != "x" {
			t.Errorf("%s: %q, %v, want %v", tt.name, out, err, tt.want)
		}
	}
	var f location.Fix
	if _, err := location.AppendPacket(nil, nil, &h); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil Fix: %v", err)
	}
	if _, err := location.AppendPacket(nil, &f, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil Header: %v", err)
	}
}
