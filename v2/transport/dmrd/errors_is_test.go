package dmrd_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	appendErr := func(f func([]byte) ([]byte, error)) error {
		_, err := f(nil)
		return err
	}
	control := func(c dmrd.Control, d dmrd.Dialect) error {
		_, err := c.AppendBinary(nil, d)
		return err
	}
	_, alias := (&dmrd.TalkerAlias{Type: 4}).LC()
	_, local := (&dmrd.LocalTalkerAlias{Type: 4}).LC()
	_, token := dmrdauth.Token(dmrd.DialectLocal, 1, []byte("x"))
	var bridge dmrdsession.Bridge
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"Data.Size", appendErr((&dmrd.Data{Size: 1}).AppendBinary), dmrd.ErrLength},
		{"TalkerAlias.Size", appendErr((&dmrd.TalkerAlias{Size: 1}).AppendBinary), dmrd.ErrLength},
		{"Position.Size", appendErr((&dmrd.Position{Size: 1}).AppendBinary), dmrd.ErrLength},
		{"Control dialect", control(dmrd.Control{Command: dmrd.CommandRPTL}, dmrd.Dialect(99)), dmrd.ErrCommand},
		{"Control text", control(dmrd.Control{Command: dmrd.CommandRPTPING, Text: []byte("x")}, dmrd.DialectMMDVM), dmrd.ErrLength},
		{"BridgeControl", appendErr((&dmrd.BridgeControl{Command: dmrd.CommandRPTL}).AppendBinary), dmrd.ErrCommand},
		{"Bridge.Version", appendErr((&dmrd.Bridge{Version: 2}).AppendBinary), dmrd.ErrSignature},
		{"Bridge.Data", appendErr((&dmrd.Bridge{Version: 1, Data: dmrd.Data{Size: 55}}).AppendBinary), dmrd.ErrLength},
		{"SetHint", (&dmrd.Data{}).SetHint(layer2.Hint{Kind: layer2.KindUnknown}), layer2.ErrKind},
		{"dmrdauth.Token", token, dmrd.ErrCommand},
		{"BridgeKey.SignInPlace", dmrdauth.NewBridgeKey([]byte("k")).SignInPlace([]byte{1}), dmrd.ErrCommand},
		{"dmrdsession.Bridge", bridge.SignInPlace(make([]byte, 60)), dmrgo.ErrZeroValue},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
	var fre *dmrgo.FieldRangeError
	for _, err := range []error{alias, local} {
		if !errors.As(err, &fre) {
			t.Errorf("talker alias Type 4: %v, want a *dmrgo.FieldRangeError", err)
		}
	}
}
