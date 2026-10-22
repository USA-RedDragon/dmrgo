package ipsc_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
)

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	appendErr := func(f func([]byte) ([]byte, error)) error {
		_, err := f(nil)
		return err
	}
	rawErr := func(u ipsc.User, src layer2.SyncSource) error {
		_, err := u.RawBurst(1, src)
		return err
	}
	var info ipsc.CallInfo
	var u ipsc.User
	_, kind := u.SetRawBurst(layer2.RawBurst{}, layer2.Hint{Kind: layer2.KindUnknown})
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"User opcode", appendErr((&ipsc.User{Opcode: ipsc.OpPeerAliveRequest}).AppendBinary), ipsc.ErrCommand},
		{"Control opcode", appendErr((&ipsc.Control{Opcode: ipsc.OpGroupVoice}).AppendBinary), ipsc.ErrCommand},
		{"Voice.Layout", appendErr((&ipsc.User{Opcode: ipsc.OpGroupVoice, Burst: ipsc.BurstVoice}).AppendBinary), layer2.ErrVoicePosition},
		{"Data.Bits", appendErr((&ipsc.User{Opcode: ipsc.OpGroupData, Burst: ipsc.BurstRaw, Data: ipsc.Payload{HasBody: true, Bits: 2000}}).AppendBinary), ipsc.ErrLength},
		{"SetRawBurst kind", kind, layer2.ErrKind},
		{"RawBurst SyncLost", rawErr(ipsc.User{Burst: ipsc.BurstSyncLost}, layer2.SyncSourceBS), ipsc.ErrCommand},
		{"RawBurst source", rawErr(ipsc.User{Burst: ipsc.BurstVoice, Voice: ipsc.Voice{Layout: ipsc.VoiceLayoutA}}, layer2.SyncSource(99)), layer2.ErrSyncSource},
		{"RawBurst slot type", rawErr(ipsc.User{Burst: ipsc.BurstRaw, Data: ipsc.Payload{HasBody: true, Bits: 196}}, layer2.SyncSourceBS), layer2.ErrDataType},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
	var fre *dmrgo.FieldRangeError
	for name, err := range map[string]error{
		"SetSlot":         info.SetSlot(enums.Slot(3)),
		"ParseKey digit":  parseKeyErr("zz"),
		"ParseKey length": parseKeyErr(strings.Repeat("1", 41)),
	} {
		if !errors.As(err, &fre) {
			t.Errorf("%s: %v, want a *dmrgo.FieldRangeError", name, err)
		}
	}
}

func parseKeyErr(s string) error {
	_, err := ipscauth.ParseKey(s)
	return err
}
