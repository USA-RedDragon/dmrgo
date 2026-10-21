package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func newMSPool(t testing.TB) burstPool {
	t.Helper()
	ahoy := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceIndividualVoice), TargetAddress: msAddr, SourceAddress: 3120002}}, testCC)
	own := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 5, TargetAddress: 9, SourceAddress: msAddr}}, testCC)
	challenge := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceRegistration), TargetAddress: msAddr, SourceAddress: 0x7A17C0}}, testCC)
	return append(newPool(t), ack(t, enums.ReasonRegAccepted), ack(t, enums.ReasonWait), ack(t, enums.ReasonSysBusyRefused), ahoy, own, challenge)
}

func runMS(t *testing.T, pool burstPool, in []byte) {
	cfg := trunk.MSConfig{Address: msAddr}
	if len(in) > 0 {
		cfg.Seed = uint64(in[0])
		cfg.NoRegistration = in[0]&1 != 0
		in = in[1:]
	}
	ms, err := trunk.NewMS(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	for op := 0; op < maxFuzzOps && len(in) >= 2; op++ {
		kind, arg := in[0], in[1]
		in = in[2:]
		now += time.Duration(arg) * 5 * time.Millisecond
		cach := layer2.CACH{TACT: pdu.TACT{AccessType: kind&0x80 != 0}}
		switch kind % 7 {
		case 0, 1, 2:
			raw := pool[int(arg)%len(pool)]
			out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot(kind >> 5 & 3)}, &cach, out[:0])
			for _, o := range out {
				if o.Kind == trunk.OutputAuthChallenge && kind&0x10 != 0 {
					out = ms.AuthResponse(now, o.Challenge^0xE8CA1D, out)
					break
				}
			}
		case 3:
			_ = ms.Call(now, arg&1 != 0, uint32(arg)+1, layer3.VoiceServiceOptions{IsEmergency: arg&2 != 0})
			out = out[:0]
		case 4:
			if at, ok := ms.Deadline(); ok && at > now {
				now = at
			}
			out = ms.Tick(now, out[:0])
		case 5:
			out = ms.EndCall(now, out[:0])
		case 6:
			if arg&1 != 0 {
				out = ms.Cancel(now, out[:0])
			} else {
				out = ms.Lost(now, out[:0])
			}
		}
		if len(out) > trunk.MaxOutputs || cap(out) != trunk.MaxOutputs {
			t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
		}
		if at, ok := ms.Deadline(); ok && at < now {
			out = ms.Tick(now, out[:0])
			if at2, ok2 := ms.Deadline(); ok2 && at2 <= at {
				t.Fatalf("op %d: Deadline %v stays after a Tick at %v", op, at2, now)
			}
		}
	}
}

func msSeeds() [][]byte {
	return [][]byte{
		{0, 0, 0, 0, 0, 0, 7, 0, 7, 3, 1, 0, 0, 0, 8, 0, 11, 5, 0},
		{1, 0, 0, 3, 3, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 4, 0, 0, 10},
		{2, 0, 0, 0, 0, 0, 10, 0, 0, 4, 200, 6, 0},
		{0, 0, 0, 0, 0, 3, 1, 0x10, 12, 4, 0, 0x10, 12, 4, 0},
	}
}

func FuzzMS(f *testing.F) {
	for _, s := range msSeeds() {
		f.Add(s)
	}
	pool := newMSPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runMS(t, pool, in)
	})
}
