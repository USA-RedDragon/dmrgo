package csbk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestNewMatch(t *testing.T) {
	t.Parallel()

	m, err := csbk.NewMatch(enums.FIDMotorola, pdu.CSBKAckOutbound, pdu.CSBKNegativeAcknowledgement)
	if err != nil {
		t.Fatal(err)
	}
	want := csbk.Match{FID: enums.FIDMotorola, Opcodes: [4]pdu.CSBKOpcode{pdu.CSBKAckOutbound, pdu.CSBKNegativeAcknowledgement}, N: 2}
	if m != want {
		t.Errorf("NewMatch = %+v, want %+v", m, want)
	}
	four := []pdu.CSBKOpcode{1, 2, 3, 4}
	if m, err := csbk.NewMatch(enums.FIDStandardized, four...); err != nil || m.N != 4 || m.Opcodes != [4]pdu.CSBKOpcode{1, 2, 3, 4} {
		t.Errorf("NewMatch(4) = %+v, %v", m, err)
	}
	for _, ops := range [][]pdu.CSBKOpcode{nil, {1, 2, 3, 4, 5}} {
		m, err := csbk.NewMatch(enums.FIDStandardized, ops...)
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != "Match" || m != (csbk.Match{}) {
			t.Errorf("NewMatch(%d opcodes) = %+v, %v", len(ops), m, err)
		}
	}
}
