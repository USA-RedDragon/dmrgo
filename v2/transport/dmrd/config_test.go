package dmrd_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func dmrGatewayConfig(t *testing.T, id uint32, cc, power uint, height int) []byte {
	t.Helper()
	body := fmt.Sprintf("%-8.8s%09d%09d%02d%02d%8.8s%9.9s%03d%-20.20s%-19.19s%c%-124.124s%-40.40s%-40.40s",
		"KI5VMF", 447000000, 434500000, power, cc, fmt.Sprintf("%08f", 30.123456), fmt.Sprintf("%09f", -97.654321), height,
		"Austin, TX", "dmrgo test", '3', "https://example.org", "20261018", "MMDVM")
	p := []byte("RPTC")
	p = append(p, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	p = append(p, body...)
	if len(p) != 302 {
		t.Fatalf("config is %d octets", len(p))
	}
	return p
}

func TestConfigFromDMRGatewayFormat(t *testing.T) {
	t.Parallel()

	p := dmrGatewayConfig(t, 3191868, 9, 8, 10)
	c, err := dmrd.ConfigOf(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() != 3191868 || string(c.Callsign()) != "KI5VMF" || string(c.Location()) != "Austin, TX" || string(c.URL()) != "https://example.org" {
		t.Errorf("id %d callsign %q location %q url %q", c.ID(), c.Callsign(), c.Location(), c.URL())
	}
	if rx, err := c.RXFrequency(); err != nil || rx != 447000000 {
		t.Errorf("rx %d %v", rx, err)
	}
	if cc, err := c.ColorCode(); err != nil || cc != 9 {
		t.Errorf("color code %d %v, want 9 (not octal)", cc, err)
	}
	if pw, err := c.Power(); err != nil || pw != 8 {
		t.Errorf("power %d %v", pw, err)
	}
	if h, err := c.Height(); err != nil || h != 10 {
		t.Errorf("height %d %v, want 10 (not octal)", h, err)
	}
	if s, err := c.Slots(); err != nil || s != 3 {
		t.Errorf("slots %d %v", s, err)
	}
	if string(c.Latitude()) != "30.12345" || string(c.Longitude()) != "-97.65432" {
		t.Errorf("lat %q lon %q", c.Latitude(), c.Longitude())
	}
	if string(c.SoftwareID()) != "20261018" || string(c.PackageID()) != "MMDVM" || string(c.Description()) != "dmrgo test" {
		t.Errorf("software %q package %q description %q", c.SoftwareID(), c.PackageID(), c.Description())
	}
}

func TestConfigSetters(t *testing.T) {
	t.Parallel()

	var c dmrd.Config
	c.Reset()
	c.SetID(42)
	if err := c.SetCallsign([]byte("N0CALL")); err != nil {
		t.Fatal(err)
	}
	if err := c.SetRXFrequency(144500000); err != nil {
		t.Fatal(err)
	}
	if err := c.SetColorCode(15); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSlots(4); err != nil {
		t.Fatal(err)
	}
	got, err := dmrd.ConfigOf(c[:])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != 42 || string(got.Callsign()) != "N0CALL" || string(c[8:16]) != "N0CALL  " {
		t.Errorf("id %d callsign %q", got.ID(), c[8:16])
	}
	if rx, _ := got.RXFrequency(); rx != 144500000 || string(c[16:25]) != "144500000" {
		t.Errorf("rx %d %q", rx, c[16:25])
	}
	if tx, err := got.TXFrequency(); err != nil || tx != 0 || string(c[25:34]) != "000000000" {
		t.Errorf("tx %d %v", tx, err)
	}
	if string(c[36:38]) != "15" || c[97] != '4' {
		t.Errorf("cc %q slots %q", c[36:38], c[97])
	}
	if err := c.SetCallsign([]byte("TOOLONGCALL")); err == nil || string(got.Callsign()) != "N0CALL" {
		t.Errorf("long callsign: %v %q", err, got.Callsign())
	}
	if err := c.SetColorCode(100); err == nil {
		t.Error("color code 100: no error")
	}
	c[36] = ' '
	if _, err := c.ColorCode(); !errors.Is(err, dmrd.ErrText) {
		t.Errorf("color code with a space: %v", err)
	}
}

func TestConfigOfErrors(t *testing.T) {
	t.Parallel()

	p := dmrGatewayConfig(t, 1, 1, 1, 1)
	if _, err := dmrd.ConfigOf(p[:301]); !errors.Is(err, dmrd.ErrShort) {
		t.Error(err)
	}
	if _, err := dmrd.ConfigOf(append(p, 0)); !errors.Is(err, dmrd.ErrLength) {
		t.Error(err)
	}
	q := append([]byte{}, p...)
	q[3] = 'X'
	if _, err := dmrd.ConfigOf(q); !errors.Is(err, dmrd.ErrSignature) {
		t.Error(err)
	}
}

func TestHBRPConfigFromSpecLayout(t *testing.T) {
	t.Parallel()

	body := fmt.Sprintf("%-8s%08X%09d%09d%02d%02d%-8s%-9s%03d%-20s%-20s%-124s%-40s%-40s",
		"DL5DI", 0x00040178, 434787500, 434787500, 0, 1, "+50.4243", "+007.3412", 30, "Berlin", "DMRplus test", "www.ham-dmr.de",
		"linux:dmrrepeater-20150702", "rpm:dmrrepeater-20150720-2_i386")
	p := append([]byte("RPTC"), body...)
	c, err := dmrd.HBRPConfigOf(p)
	if err != nil {
		t.Fatalf("%d octets: %v", len(p), err)
	}
	if id, err := c.ID(); err != nil || id != 0x00040178 {
		t.Errorf("id %x %v", id, err)
	}
	if rx, _ := c.RXFrequency(); rx != 434787500 || string(c.Callsign()) != "DL5DI" || string(c.Latitude()) != "+50.4243" {
		t.Errorf("rx %d callsign %q lat %q", rx, c.Callsign(), c.Latitude())
	}
	if string(c.Description()) != "DMRplus test" || string(c.PackageID()) != "rpm:dmrrepeater-20150720-2_i386" {
		t.Errorf("description %q package %q", c.Description(), c.PackageID())
	}
}
