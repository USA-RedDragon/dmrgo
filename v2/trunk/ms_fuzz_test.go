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
	ahoy := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceKindIndividualVoice), TargetAddress: msAddr, SourceAddress: 3120002}), testCC)
	own := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 5, TargetAddress: 9, SourceAddress: msAddr}), testCC)
	challenge := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: msAddr, SourceAddress: 0x7A17C0}), testCC)
	return append(newPool(t), ack(t, enums.AckReasonRegAccepted), ack(t, enums.AckReasonWait), ack(t, enums.AckReasonSysBusyRefused), ahoy, own, challenge)
}

func runMS(t *testing.T, pool burstPool, s opSeed) {
	cfg := trunk.MSConfig{Address: msAddr, Seed: uint64(s.cfg), NoRegistration: s.cfg&1 != 0}
	script := s.script()
	ms, err := trunk.NewMS(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	for op := range script.total {
		kind, arg := script.op(op)
		now += time.Duration(arg) * 5 * time.Millisecond
		cach := idle
		if kind&0x80 != 0 {
			cach = busy
		}
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

func msSeeds() []opSeed {
	return []opSeed{
		{cfg: 0, ops: []byte{0, 0, 0, 0, 0, 7, 0, 7, 3, 1, 0, 0, 0, 8, 0, 11, 5, 0}},
		{cfg: 1, ops: []byte{0, 0, 3, 3, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 4, 0, 0, 10}},
		{cfg: 2, ops: []byte{0, 0, 0, 0, 0, 10, 0, 0, 4, 200, 6, 0}},
		{cfg: 0, ops: []byte{0, 0, 0, 0, 3, 1, 0x10, 12, 4, 0, 0x10, 12, 4, 0}},
		{cfg: 0, ops: []byte{0, 0, 0, 0, 0, 7, 0, 7, 3, 1, 0, 0, 0, 8, 0, 1, 0, 4, 5, 0}, from: 7, to: 9, reps: 40},
	}
}

func FuzzMS(f *testing.F) {
	for _, s := range msSeeds() {
		f.Add(s.cfg, s.ops, s.from, s.to, s.reps)
	}
	pool := newMSPool(f)
	f.Fuzz(func(t *testing.T, cfg uint8, ops []byte, from, to, reps uint8) {
		runMS(t, pool, opSeed{cfg: cfg, ops: ops, from: from, to: to, reps: reps})
	})
}
