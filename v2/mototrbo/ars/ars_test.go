package ars_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const registration = "000cf020073331393138363800" + "00"

func TestRegistration(t *testing.T) {
	t.Parallel()

	data := unhex(t, registration)
	m, err := ars.DecodeMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := m.DeviceID()
	if m.Header.Type != ars.TypeDeviceRegistration || !m.Header.Extension || !m.Header.AckRequired || m.Registration.Event != ars.EventInitial ||
		!ok || id != 3191868 || len(m.User) != 0 || len(m.Password) != 0 || len(m.Extra) != 0 {
		t.Fatalf("got %s", m.String())
	}
	n := ars.NewRegistration(ars.EventInitial)
	if _, err := n.SetDeviceID(nil, 3191868); err != nil {
		t.Fatal(err)
	}
	out, err := n.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Errorf("NewRegistration = %x, %v, want %x", out, err, data)
	}
}

func TestAnswers(t *testing.T) {
	t.Parallel()

	ack, err := ars.NewAck(1)
	if err != nil {
		t.Fatal(err)
	}
	fail, err := ars.NewFailure(ars.FailureDeviceNotAuthorized)
	if err != nil {
		t.Fatal(err)
	}
	q := ars.NewQuery()
	for _, tt := range []struct {
		m    ars.Message
		want string
	}{{ack, "0002bf01"}, {fail, "0002ff00"}, {q, "000114"}} {
		out, err := tt.m.AppendBinary(nil)
		if err != nil || hex.EncodeToString(out) != tt.want {
			t.Errorf("%s = %x, %v, want %s", tt.m.String(), out, err, tt.want)
		}
		back, err := ars.DecodeMessage(out)
		if err != nil || back.Header != tt.m.Header || back.Response != tt.m.Response {
			t.Errorf("decoded %s, %v", back.String(), err)
		}
	}
	if _, err := ars.NewAck(128); err == nil {
		t.Error("NewAck(128) did not fail")
	}
	if _, err := ars.NewFailure(200); err == nil {
		t.Error("NewFailure(200) did not fail")
	}
}

func TestDeviceID(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		device string
		ok     bool
	}{{"", false}, {"12a", false}, {"16777216", false}, {"16777215", true}, {"123456789", false}} {
		m := ars.Message{Device: []byte(tt.device)}
		if _, ok := m.DeviceID(); ok != tt.ok {
			t.Errorf("DeviceID(%q) ok %t", tt.device, ok)
		}
	}
	var m ars.Message
	buf, err := m.SetDeviceID([]byte("x"), 0)
	if err != nil || string(buf) != "x0" || string(m.Device) != "0" {
		t.Errorf("SetDeviceID(0) = %q, %q, %v", buf, m.Device, err)
	}
	buf, err = m.SetDeviceID([]byte("x"), 0xFFFFFF)
	if id, ok := m.DeviceID(); err != nil || string(buf) != "x16777215" || !ok || id != 0xFFFFFF {
		t.Errorf("SetDeviceID(0xFFFFFF) = %q, %v", buf, err)
	}
	var fe *dmrgo.FieldRangeError
	if buf, err := m.SetDeviceID([]byte("x"), 0x1000000); !errors.As(err, &fe) || fe.Bits != 24 || string(buf) != "x" {
		t.Errorf("SetDeviceID(0x1000000) = %q, %v", buf, err)
	}
	var nilMsg *ars.Message
	if _, err := nilMsg.SetDeviceID(nil, 1); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil m: %v", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	data := unhex(t, registration)
	for _, tt := range []struct {
		data []byte
		want error
	}{{nil, ars.ErrShort}, {data[:8], ars.ErrShort}, {append(append([]byte{}, data...), 1), ars.ErrLength}, {[]byte{0, 3, 0x00, 9, 0x31}, ars.ErrShort}} {
		if _, err := ars.DecodeMessage(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%x: %v, want %v", tt.data, err, tt.want)
		}
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if ars.TypeAck.String() != "Ack" || ars.Type(2).String() != "Type(2)" || ars.EventRefresh.String() != "Refresh" || ars.Event(3).String() != "Event(3)" {
		t.Error("names")
	}
	if ars.EncodingASCII.String() != "ASCII" || ars.Encoding(1).String() != "Encoding(1)" {
		t.Error("Encoding names")
	}
	for r, want := range map[ars.FailureReason]string{
		ars.FailureDeviceNotAuthorized: "Device Not Authorized", ars.FailureUserIDNotValid: "User ID Not Valid",
		ars.FailureUserValidationTimeout: "User Validation Timeout", ars.FailureTransmission: "Transmission Failure",
		ars.FailureReason(3): "FailureReason(3)",
	} {
		if r.String() != want {
			t.Errorf("%d: %s", r, r.String())
		}
	}
}
