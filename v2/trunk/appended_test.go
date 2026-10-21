package trunk_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func checkPad(t *testing.T, name string, a *trunk.Appended, n int, want [2]int) {
	t.Helper()
	if a.Blocks-1 != want[0] || int(a.PadNibble) != want[1] {
		t.Errorf("%s %d: UAB %d PN %d, want %d %d", name, n, a.Blocks-1, a.PadNibble, want[0], want[1])
	}
}

func TestAppended_PadNibbleTables(t *testing.T) {
	t.Parallel()
	bcd := map[int][2]int{1: {0, 19}, 2: {0, 18}, 3: {0, 17}, 4: {0, 16}, 5: {0, 15}, 6: {0, 14}, 7: {0, 13}, 8: {0, 12}, 9: {0, 11}, 10: {0, 10}, 11: {0, 9}, 12: {0, 8}, 13: {0, 7}, 14: {0, 6}, 15: {0, 5}, 16: {0, 4}, 17: {0, 3}, 18: {0, 2}, 19: {0, 1}, 20: {0, 0}, 21: {1, 23}, 32: {1, 12}, 33: {1, 11}, 34: {1, 10}, 35: {1, 9}, 36: {1, 8}, 37: {1, 7}, 38: {1, 6}, 39: {1, 5}, 40: {1, 4}, 41: {1, 3}, 42: {1, 2}, 43: {1, 1}, 44: {1, 0}, 45: {2, 23}, 46: {2, 22}, 47: {2, 21}, 48: {2, 20}, 49: {2, 19}, 50: {2, 18}, 51: {2, 17}, 52: {2, 16}, 63: {2, 5}, 64: {2, 4}, 65: {2, 3}, 66: {2, 2}, 67: {2, 1}, 68: {2, 0}, 69: {3, 23}, 70: {3, 22}, 71: {3, 21}, 72: {3, 20}, 73: {3, 19}, 74: {3, 18}, 75: {3, 17}, 76: {3, 16}, 77: {3, 15}, 78: {3, 14}, 79: {3, 13}, 80: {3, 12}, 81: {3, 11}, 82: {3, 10}, 83: {3, 9}, 92: {3, 0}}
	iso7 := map[int][2]int{1: {0, 18}, 2: {0, 16}, 3: {0, 14}, 4: {0, 13}, 5: {0, 11}, 6: {0, 9}, 7: {0, 7}, 8: {0, 6}, 9: {0, 4}, 10: {0, 2}, 11: {0, 0}, 12: {1, 23}, 13: {1, 21}, 14: {1, 19}, 15: {1, 17}, 16: {1, 16}, 17: {1, 14}, 18: {1, 12}, 19: {1, 10}, 20: {1, 9}, 21: {1, 7}, 22: {1, 5}, 23: {1, 3}, 24: {1, 2}, 25: {1, 0}, 26: {2, 22}, 27: {2, 20}, 28: {2, 19}, 29: {2, 17}, 30: {2, 15}, 31: {2, 13}, 32: {2, 12}, 33: {2, 10}, 34: {2, 8}, 35: {2, 6}, 36: {2, 5}, 37: {2, 3}, 38: {2, 1}, 39: {3, 23}, 40: {3, 22}, 41: {3, 20}, 42: {3, 18}, 43: {3, 16}, 44: {3, 15}, 45: {3, 13}, 46: {3, 11}, 47: {3, 9}, 48: {3, 8}, 49: {3, 6}, 50: {3, 4}, 51: {3, 2}, 52: {3, 1}}
	iso8 := map[int][2]int{1: {0, 18}, 2: {0, 16}, 3: {0, 14}, 4: {0, 12}, 5: {0, 10}, 6: {0, 8}, 7: {0, 6}, 8: {0, 4}, 9: {0, 2}, 10: {0, 0}, 11: {1, 22}, 12: {1, 20}, 13: {1, 18}, 14: {1, 16}, 15: {1, 14}, 16: {1, 12}, 17: {1, 10}, 18: {1, 8}, 19: {1, 6}, 20: {1, 4}, 21: {1, 2}, 22: {1, 0}, 23: {2, 22}, 24: {2, 20}, 25: {2, 18}, 26: {2, 16}, 27: {2, 14}, 28: {2, 12}, 29: {2, 10}, 30: {2, 8}, 31: {2, 6}, 32: {2, 4}, 33: {2, 2}, 34: {2, 0}, 35: {3, 22}, 36: {3, 20}, 37: {3, 18}, 38: {3, 16}, 39: {3, 14}, 40: {3, 12}, 41: {3, 10}, 42: {3, 8}, 43: {3, 6}, 44: {3, 4}, 45: {3, 2}, 46: {3, 0}}
	utf16 := map[int][2]int{1: {0, 16}, 2: {0, 12}, 3: {0, 8}, 4: {0, 4}, 5: {0, 0}, 6: {1, 20}, 7: {1, 16}, 8: {1, 12}, 9: {1, 8}, 10: {1, 4}, 11: {1, 0}, 12: {2, 20}, 13: {2, 16}, 14: {2, 12}, 15: {2, 8}, 16: {2, 4}, 17: {2, 0}, 18: {3, 20}, 19: {3, 16}, 20: {3, 12}, 21: {3, 8}, 22: {3, 4}, 23: {3, 0}}
	mixed := map[int][2]int{1: {0, 8}, 2: {0, 4}, 3: {0, 0}, 4: {1, 20}, 5: {1, 16}, 6: {1, 12}, 7: {1, 8}, 8: {1, 4}, 9: {1, 0}, 10: {2, 20}, 11: {2, 16}, 12: {2, 12}, 13: {2, 8}, 14: {2, 4}, 15: {2, 0}, 16: {3, 20}, 17: {3, 16}, 18: {3, 12}, 19: {3, 8}, 20: {3, 4}, 21: {3, 0}}
	var a trunk.Appended
	for n, want := range bcd {
		digits := strings.Repeat("7", n)
		if err := a.SetDigits([]byte(digits)); err != nil {
			t.Fatal(err)
		}
		checkPad(t, "BCD", &a, n, want)
		got, err := a.Digits(nil)
		if err != nil || string(got) != digits {
			t.Errorf("BCD %d: %q %v", n, got, err)
		}
	}
	for name, c := range map[string]struct {
		f     enums.UDTFormat
		table map[int][2]int
	}{"ISO7": {enums.UDTISO7, iso7}, "ISO8": {enums.UDTISO8, iso8}, "UTF16": {enums.UDTUTF16BE, utf16}} {
		for n, want := range c.table {
			text := strings.Repeat("z", n)
			if err := a.SetText(c.f, []byte(text)); err != nil {
				t.Fatal(err)
			}
			checkPad(t, name, &a, n, want)
			got, err := a.Text(nil)
			if err != nil || string(got) != text {
				t.Errorf("%s %d: %q %v", name, n, got, err)
			}
		}
	}
	for n, want := range mixed {
		text := strings.Repeat("q", n)
		if err := a.SetMixed(true, 9001, []byte(text)); err != nil {
			t.Fatal(err)
		}
		checkPad(t, "Mixed", &a, n, want)
		got, err := a.Text(nil)
		addr, oneKey, merr := a.Mixed()
		if err != nil || merr != nil || string(got) != text || addr != 9001 || !oneKey {
			t.Errorf("Mixed %d: %q %d %v %v %v", n, got, addr, oneKey, err, merr)
		}
	}
}

func TestAppended_Binary(t *testing.T) {
	t.Parallel()
	var a trunk.Appended
	src := bytes.Repeat([]byte{0xA5}, 46)
	for _, c := range []struct{ n, blocks int }{{1, 1}, {79, 1}, {80, 2}, {175, 2}, {176, 3}, {271, 3}, {272, 4}, {367, 4}} {
		if err := a.SetBinary(src, c.n); err != nil {
			t.Fatal(err)
		}
		if a.Blocks != c.blocks || a.PadNibble != 0 || a.Format != enums.UDTBinary {
			t.Errorf("%d bits: %d blocks PN %d", c.n, a.Blocks, a.PadNibble)
		}
		got, n, err := a.Binary(nil)
		if err != nil || n != c.n {
			t.Fatalf("%d bits: got %d %v", c.n, n, err)
		}
		want := append([]byte(nil), src[:(c.n+7)/8]...)
		if c.n%8 != 0 {
			want[len(want)-1] &= byte(0xFF) << (8 - c.n%8)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%d bits: % x", c.n, got)
		}
	}
	for _, n := range []int{0, 368} {
		if err := a.SetBinary(src, n); !errors.Is(err, trunk.ErrUDTLength) {
			t.Errorf("%d bits: %v", n, err)
		}
	}
	a = trunk.Appended{Format: enums.UDTBinary, Blocks: 1}
	for i := range 10 {
		a.Data[i] = 0xFF
	}
	if _, _, err := a.Binary(nil); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("no end marker: %v", err)
	}
	a.Data[0] = 0x7F
	if _, _, err := a.Binary(nil); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("no data: %v", err)
	}
}

func TestAppended_Addresses(t *testing.T) {
	t.Parallel()
	var a trunk.Appended
	for _, c := range []struct{ n, blocks, slots int }{{1, 1, 3}, {3, 1, 3}, {4, 2, 7}, {7, 2, 7}, {8, 3, 11}, {11, 3, 11}, {12, 4, 15}, {15, 4, 15}} {
		list := make([]uint32, c.n)
		for i := range list {
			list[i] = 0x100000 + uint32(i)
		}
		if err := a.SetAddresses(c.n == 3, list); err != nil {
			t.Fatal(err)
		}
		l, err := a.Addresses()
		if err != nil || a.Blocks != c.blocks || l.Len != c.slots || l.OneKey != (c.n == 3) {
			t.Fatalf("%d: blocks %d %+v %v", c.n, a.Blocks, l, err)
		}
		for i := range l.Len {
			want := uint32(0)
			if i < c.n {
				want = list[i]
			}
			if l.Addresses[i] != want {
				t.Errorf("%d: address %d is %d", c.n, i, l.Addresses[i])
			}
		}
	}
	if err := a.SetAddresses(false, make([]uint32, 16)); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("16 addresses: %v", err)
	}
	if _, err := (&trunk.Appended{Format: enums.UDTBCD, Blocks: 1}).Addresses(); !errors.Is(err, trunk.ErrUDTFormat) {
		t.Errorf("BCD as addresses: %v", err)
	}
}

func TestAppended_DigitsAndText(t *testing.T) {
	t.Parallel()
	var a trunk.Appended
	if err := a.SetDigits([]byte("0123456789*#")); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(a.Data[:10], want) || a.PadNibble != 8 {
		t.Errorf("digits % x PN %d", a.Data[:10], a.PadNibble)
	}
	if err := a.SetDigits([]byte("12a")); !errors.Is(err, trunk.ErrUDTCharacter) {
		t.Errorf("letter digit: %v", err)
	}
	if err := a.SetDigits([]byte("12")); err != nil {
		t.Fatal(err)
	}
	a.Data[0] = 0xC0
	if _, err := a.Digits(nil); !errors.Is(err, trunk.ErrUDTCharacter) {
		t.Errorf("reserved digit: %v", err)
	}
	if err := a.SetText(enums.UDTISO7, []byte("AB")); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x83, 0x0B, 0xFF}; !bytes.Equal(a.Data[:3], want) || a.PadNibble != 16 {
		t.Errorf("ISO7 % x PN %d", a.Data[:3], a.PadNibble)
	}
	for _, c := range []struct {
		f    enums.UDTFormat
		text string
	}{{enums.UDTISO7, "é"}, {enums.UDTISO8, "€"}} {
		if err := a.SetText(c.f, []byte(c.text)); !errors.Is(err, trunk.ErrUDTCharacter) {
			t.Errorf("%v %q: %v", c.f, c.text, err)
		}
	}
	if err := a.SetText(enums.UDTISO8, []byte("café")); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Text(nil); string(got) != "café" {
		t.Errorf("ISO8 %q", got)
	}
	if err := a.SetText(enums.UDTUTF16BE, []byte("a😀")); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x00, 0x61, 0xD8, 0x3D, 0xDE, 0x00}; !bytes.Equal(a.Data[:6], want) || a.PadNibble != 8 {
		t.Errorf("UTF16 % x PN %d", a.Data[:6], a.PadNibble)
	}
	if got, _ := a.Text(nil); string(got) != "a😀" {
		t.Errorf("UTF16 %q", got)
	}
	if err := a.SetText(enums.UDTUTF16BE, []byte(strings.Repeat("x", 24))); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("24 units: %v", err)
	}
	if err := a.SetText(enums.UDTBCD, []byte("1")); !errors.Is(err, trunk.ErrUDTFormat) {
		t.Errorf("BCD text: %v", err)
	}
	if err := a.SetMixed(false, 0, nil); err != nil || a.Blocks != 1 || a.PadNibble != 0 {
		t.Errorf("DGNA alias delete: %v %+v", err, a)
	}
	if got, _ := a.Text(nil); len(got) != 0 {
		t.Errorf("empty alias %q", got)
	}
}

func TestAppended_NMEAIPLIP(t *testing.T) {
	t.Parallel()
	var a trunk.Appended
	short := trunk.NMEAShort{North: true, Fix: true, Speed: 12, LatDegrees: 41, LatMinutes: 52, LatFraction: 1234,
		LonDegrees: 87, LonMinutes: 37, LonFraction: 9876, Hours: 23, Minutes: 59, TenSeconds: 5}
	if err := a.SetNMEAShort(&short); err != nil {
		t.Fatal(err)
	}
	if got, err := a.NMEAShort(); err != nil || got != short {
		t.Errorf("short NMEA %+v %v", got, err)
	}
	if _, err := a.NMEALong(); !errors.Is(err, trunk.ErrUDTFormat) {
		t.Errorf("short as long: %v", err)
	}
	long := trunk.NMEALong{East: true, LatDegrees: 1, Seconds: 59, Course: 359}
	if err := a.SetNMEALong(&long); err != nil {
		t.Fatal(err)
	}
	if got, err := a.NMEALong(); err != nil || got != long || a.Blocks != 2 {
		t.Errorf("long NMEA %+v %v", got, err)
	}
	long.Course = 512
	if err := a.SetNMEALong(&long); err == nil {
		t.Error("course 512 encoded")
	}
	a.SetIP([16]byte{192, 0, 2, 7}, false)
	if ip, v6, err := a.IP(); err != nil || v6 || ip != [16]byte{192, 0, 2, 7} || a.Blocks != 1 {
		t.Errorf("IPv4 %v %v %v", ip, v6, err)
	}
	v6addr := [16]byte{0x20, 0x01, 0x0d, 0xb8, 15: 1}
	a.SetIP(v6addr, true)
	if ip, v6, err := a.IP(); err != nil || !v6 || ip != v6addr || a.Blocks != 2 {
		t.Errorf("IPv6 %v %v %v", ip, v6, err)
	}
	r := trunk.LIPShortReport{TimeElapsed: 1, Longitude: -90, Latitude: 45, PositionError: layer3.PositionError(2), Velocity: 28, Direction: 4}
	p, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetLIP(p[:]); err != nil || a.Blocks != 1 || a.PadNibble != 0 {
		t.Fatalf("LIP %v %+v", err, a)
	}
	got, err := a.LIP(nil)
	if err != nil || len(got) != 10 {
		t.Fatalf("LIP %d %v", len(got), err)
	}
	if back := trunk.DecodeLIPShortReport([10]byte(got)); back != r {
		t.Errorf("LIP report %+v", back)
	}
	if err := a.SetLIP(make([]byte, 47)); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("47 octets: %v", err)
	}
}
