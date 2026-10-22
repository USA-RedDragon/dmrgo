package ars_test

import (
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
)

func TestFailureTransmissionValues(t *testing.T) {
	t.Parallel()
	for r := range ars.FailureReason(255) {
		want := "Transmission Failure"
		switch {
		case r == ars.FailureDeviceNotAuthorized, r == ars.FailureUserIDNotValid, r == ars.FailureUserValidationTimeout:
			continue
		case r > 127:
			want = "FailureReason(" + strconv.Itoa(int(r)) + ")"
		}
		if got := r.String(); got != want {
			t.Errorf("FailureReason(%d) = %q, want %q", r, got, want)
		}
	}
}

func TestFailureFromOKDMRLib(t *testing.T) {
	t.Parallel()
	m, err := ars.DecodeMessage([]byte{0x00, 0x02, 0xFF, 0xFF})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Header.AckRequired || m.Header.Type != ars.TypeAck || ars.FailureReason(m.Response.Value) != ars.FailureTransmission {
		t.Errorf("%+v", m)
	}
	f, err := ars.NewFailure(ars.FailureTransmission)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "\x00\x02\xff\x7f" {
		t.Errorf("NewFailure(FailureTransmission) = % x", b)
	}
}
