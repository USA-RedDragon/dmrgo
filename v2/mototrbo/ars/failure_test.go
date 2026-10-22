package ars_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
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

func TestDeviceFailureReason(t *testing.T) {
	t.Parallel()
	m, err := ars.DecodeMessage([]byte{0x00, 0x02, 0xFF, 0xFF})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Header.AckRequired || m.Header.Type != ars.TypeAck || ars.FailureReason(m.Response.Value) != ars.FailureTransmission {
		t.Errorf("%+v", m)
	}
	for _, r := range []ars.FailureReason{ars.FailureUserIDNotValid, ars.FailureUserValidationTimeout, 3, ars.FailureTransmission, 200} {
		var ce *dmrgo.ConfigError
		if _, err := ars.NewFailure(r); !errors.As(err, &ce) {
			t.Errorf("NewFailure(%d) = %v, want a *dmrgo.ConfigError", r, err)
		}
	}
}
