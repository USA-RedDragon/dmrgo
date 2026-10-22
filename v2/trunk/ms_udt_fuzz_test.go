package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func newUDTPool(t testing.TB) burstPool {
	t.Helper()
	var addrs, text trunk.Appended
	if err := addrs.SetAddresses(true, []uint32{501, 502, 503, 504}); err != nil {
		t.Fatal(err)
	}
	if err := text.SetMixed(false, 777, []byte("alias")); err != nil {
		t.Fatal(err)
	}
	pool := burstPool{
		aloha(t), ack(t, enums.AckReasonWait), ack(t, enums.AckReasonMSAccepted), ack(t, enums.AckReasonMSAwayRefused),
		ahoyBurst(t, enums.ServiceKindIndividualShortData, constants.GatewaySDMI, msAddr, false, 1),
		ahoyBurst(t, enums.ServiceKindShortDataPolling, peer, msAddr, false, 0),
		ahoyBurst(t, enums.ServiceKindCallDiversion, constants.GatewayDIVERTI, msAddr, false, 0),
		ahoyBurst(t, enums.ServiceKindSupplementary, constants.GatewayDGNAI, msAddr, false, 1),
		ahoyBurst(t, enums.ServiceKindDuplexVoice, peer, msAddr, false, 0),
		ahoyBurst(t, enums.ServiceKindIndividualVoice, peer, msAddr, true, 0),
		dxGrant(t, false, msAddr, peer), dxGrant(t, true, peer, msAddr), pvGrant(t, msAddr, peer, false),
		usbdPoll(t, pdu.ServiceTypeLIP, 1, msAddr), tvGrant(t, 501, 5, false),
	}
	pool = append(pool, download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &addrs)...)
	pool = append(pool, download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &text)...)
	return append(pool, download(t, pdu.UDTHeader{SupplementaryFlag: true, LLIDDestination: msAddr, LLIDSource: constants.GatewayDIVERTI}, &addrs)...)
}

func runMSUDT(t *testing.T, pool burstPool, s opSeed) {
	cfg := trunk.MSConfig{Address: msAddr, NoRegistration: true, Seed: uint64(s.cfg), Ambient: s.cfg&1 != 0, NoDuplex: s.cfg&2 != 0}
	script := s.script()
	ms, err := trunk.NewMS(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var a trunk.Appended
	if err := a.SetText(enums.UDTISO7, []byte("fuzz")); err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	for op := range script.total {
		kind, arg := script.op(op)
		now += time.Duration(arg) * 5 * time.Millisecond
		switch kind % 8 {
		case 0, 1, 2:
			raw := pool[int(arg)%len(pool)]
			out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot(kind >> 6)}, &idle, out[:0])
		case 3:
			switch arg % 8 {
			case 0:
				_ = ms.SendShortData(now, arg&8 != 0, peer, &a, layer3.ShortDataServiceOptions{})
			case 1:
				_ = ms.PollShortData(now, peer, enums.UDTNMEA, 1+int(arg>>4)%4)
			case 2:
				_ = ms.Divert(now, constants.GatewayMSI, layer3.DiversionServiceOptions{Set: true, Voice: true}, &a)
			case 3:
				_ = ms.Divert(now, constants.GatewayTSI, layer3.DiversionServiceOptions{Voice: true}, nil)
			case 4:
				_ = ms.AssignGroups(now, peer, &a)
			case 5:
				_ = ms.CallDuplex(now, peer, layer3.VoiceServiceOptions{})
			case 6:
				_ = ms.CallAmbient(now, peer, layer3.VoiceServiceOptions{IsEmergency: arg&8 != 0})
			case 7:
				_ = ms.CallDuplexData(now, peer, layer3.DataServiceOptions{})
			}
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
				ms.SetPollResponse(&a)
				ms.SetUSBDResponse(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP})
			} else {
				ms.SetPollResponse(nil)
				ms.SetUSBDResponse(nil)
			}
			out = out[:0]
		case 7:
			out = ms.Cancel(now, out[:0])
			_ = ms.GroupAlias(nil, 777)
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

func msUDTSeeds() []opSeed {
	return []opSeed{
		{cfg: 1, ops: []byte{0, 0, 3, 0, 0, 0, 0, 4, 0, 0, 0, 2}},
		{cfg: 0, ops: []byte{0, 0, 3, 1, 0, 0, 0, 5, 0, 15, 0, 16, 0, 17}},
		{cfg: 3, ops: []byte{6, 1, 0, 13, 0, 9, 0, 12, 4, 200}},
		{cfg: 2, ops: []byte{3, 4, 0, 0, 0, 7, 0, 2, 3, 2, 0, 6}},
		{cfg: 0, ops: []byte{0, 0, 3, 1, 0, 0, 0, 5, 0, 13, 4, 1}, from: 4, to: 6, reps: 40},
	}
}

func FuzzMSUDT(f *testing.F) {
	for _, s := range msUDTSeeds() {
		f.Add(s.cfg, s.ops, s.from, s.to, s.reps)
	}
	pool := newUDTPool(f)
	f.Fuzz(func(t *testing.T, cfg uint8, ops []byte, from, to, reps uint8) {
		runMSUDT(t, pool, opSeed{cfg: cfg, ops: ops, from: from, to: to, reps: reps})
	})
}
