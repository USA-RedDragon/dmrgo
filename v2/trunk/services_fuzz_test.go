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

func svcAhoy(t testing.TB, kind enums.ServiceKind, flag, group bool, mirror, ab uint8, target, source uint32) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{ServiceOptsMirror: mirror, ServiceKindFlag: flag,
		GroupIndividual: group, AppendedBlocks: ab, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: source}), testCC)
}

func svcAck(t testing.TB, r enums.AckReason, info uint8, from uint32) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckOutbound, pdu.AckOutboundPDU{
		ResponseInfo: info, ReasonCode: byte(r), TargetAddress: msAddr, AdditionalInfo: from}), testCC)
}

func newMSSvcPool(t testing.TB) burstPool {
	t.Helper()
	mass := func(w, mask uint8, addr uint32) layer2.RawBurst {
		return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetBroadcast, pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementMassReg,
			BroadcastParms1: uint16(w)<<5 | uint16(mask), Reg: true, Backoff: 3, SysIdentCode: sysIdent(t), BroadcastParms2: addr}), testCC)
	}
	return append(newMSPool(t),
		svcAhoy(t, enums.ServiceKindSupplementary, false, false, 0, 0, msAddr, constants.GatewaySTUNI),
		svcAhoy(t, enums.ServiceKindSupplementary, true, false, 0, 0, msAddr, constants.GatewaySTUNI),
		svcAhoy(t, enums.ServiceKindSupplementary, false, false, 0, 0, msAddr, constants.GatewayKILLI),
		svcAhoy(t, enums.ServiceKindStatusTransport, false, false, 0x1F, 3, msAddr, 3120002),
		svcAhoy(t, enums.ServiceKindStatusTransport, false, true, 0x05, 1, 9, 3120002),
		svcAhoy(t, enums.ServiceKindRegistration, false, false, 0, 1, msAddr, constants.GatewayIPI),
		svcAhoy(t, enums.ServiceKindIndividualVoice, false, false, 0, 1, msAddr, constants.GatewaySUPLI),
		svcAck(t, enums.AckReasonAuthResponseTS, 0, 0x123456),
		svcAck(t, enums.AckReasonRegAccepted, 3, constants.GatewayREGI),
		svcAck(t, enums.AckReasonStatusPollAcceptedMS, 17, 3120002),
		svcAck(t, enums.AckReasonMSAccepted, 0, 3120002),
		svcAck(t, enums.AckReasonIPConnectionFailed, 0, constants.GatewayIPI),
		svcAck(t, enums.AckReasonNotSupported, 0, constants.GatewayTSI),
		mass(1, 0, 0), mass(0, 0, 0), mass(3, 24, msAddr),
		voiceSuperframe(t)[5])
}

func runMSServices(t *testing.T, pool burstPool, in []byte) {
	cfg := trunk.MSConfig{Address: msAddr, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1}
	if len(in) > 0 {
		cfg.Seed = uint64(in[0])
		cfg.PowerSave = in[0] & 7
		cfg.NoStun = in[0]&8 != 0
		if in[0]&16 != 0 {
			cfg.Auth = xorAuth(0x5A5A5A)
		}
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
		var cach [3]byte
		if kind&0x80 != 0 {
			cach = [3]byte{0x80, 0x02, 0x02}
		}
		switch kind % 9 {
		case 0, 1, 2, 3:
			raw := pool[int(arg)%len(pool)]
			out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot(kind >> 5 & 3)}, &cach, out[:0])
		case 4:
			if at, ok := ms.Deadline(); ok && at > now {
				now = at
			}
			out = ms.Tick(now, out[:0])
		case 5:
			_ = ms.SendStatus(now, arg&1 != 0, uint32(arg)+1, trunk.Status(arg>>1))
			_ = ms.SetSupplementary(arg&15, []byte{arg, arg})
			out = out[:0]
		case 6:
			_ = ms.PollStatus(now, uint32(arg)+1)
			_ = ms.AdviseIP(now, [16]byte{arg}, arg&1 != 0, arg&2 != 0)
			out = out[:0]
		case 7:
			out, _ = ms.Interrupt(now, arg&1 != 0, out[:0])
		case 8:
			_ = ms.Call(now, arg&1 != 0, uint32(arg)+1, layer3.VoiceServiceOptions{IsSuppData: arg&2 != 0})
			out = ms.Lost(now, out[:0])
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

func msSvcSeeds() [][]byte {
	return [][]byte{
		{0, 0, 0, 0, 0, 0, 7, 0, 8, 5, 30, 0, 22, 0, 23, 4, 0},
		{0x13, 0, 0, 0, 0, 0, 7, 0, 22, 0, 26, 0, 23, 4, 200},
		{2, 0, 0, 0, 0, 0, 7, 0, 27, 4, 20, 0, 21, 4, 50, 7, 1},
	}
}

func FuzzMSServices(f *testing.F) {
	for _, s := range msSvcSeeds() {
		f.Add(s)
	}
	pool := newMSSvcPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runMSServices(t, pool, in)
	})
}

func newTSCCSvcPool(t testing.TB) burstPool {
	t.Helper()
	rnd := func(kind enums.ServiceKind, opts, res uint8, target uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceOptions: opts, Reserved: res, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: 3120001}))
	}
	ackU := func(r enums.AckReason, target, from uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(r), TargetAddress: target, AdditionalInfo: from}))
	}
	msData := func(d layer2.Data) layer2.RawBurst {
		raw, err := layer2.BuildBurst(d, testCC, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	head := func(dst uint32, uab uint8) layer2.RawBurst {
		return msData(withPDU(t, &pdu.DataHeader{}, (*pdu.DataHeader).SetUDTHeader, pdu.UDTHeader{UDTFormat: 6,
			LLIDDestination: dst, LLIDSource: 3120001, UAB: uab, SupplementaryFlag: true, Opcode: pdu.UDTOpcodeInboundHeader}))
	}
	block := msData(&pdu.Rate12Data{Data: [12]byte{10, 1, 2, 3}})
	return append(newInboundPool(t),
		rnd(enums.ServiceKindRegistration, 0b0011001, 0, constants.GatewayREGI),
		rnd(enums.ServiceKindRegistration, 0b0010000, 0, constants.GatewayREGI),
		rnd(enums.ServiceKindStatusTransport, 0x1F, 1, 3120002),
		rnd(enums.ServiceKindStatusTransport, 0x5F, 3, 9),
		rnd(enums.ServiceKindStatusTransport, 0x5E, 3, 9),
		rnd(enums.ServiceKindStatusTransport, 0x3F, 7, 3120002),
		rnd(enums.ServiceKindIndividualVoice, 0x10, 4, 3120002),
		ackU(enums.AckReasonMSAccepted, constants.GatewaySTUNI, 3120002),
		ackU(enums.AckReasonMSAccepted, constants.GatewaySUPLI, 3120002),
		ackU(enums.AckReasonStatusPollAcceptedMS, 3120001, 3120002),
		msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckvitation, pdu.AckvitationPDU{
			ServiceKind: uint8(enums.ServiceKindSupplementary), TargetAddress: 77, SourceAddress: 3120002})),
		head(constants.GatewayIPI, 0), head(constants.GatewaySUPLI, 1), block,
		voiceSuperframe(t)[0], voiceSuperframe(t)[3])
}

func runTSCCServices(t *testing.T, pool burstPool, in []byte) {
	cfg := tsccConfig()
	if len(in) > 0 {
		cfg.PayloadLen = 1 + int(in[0]&1)
		cfg.Reg = in[0]&2 != 0
		cfg.PowerSave, cfg.IP, cfg.Supplementary, cfg.TransmitInterrupt = in[0]&4 != 0, in[0]&8 != 0, in[0]&16 != 0, in[0]&32 != 0
		cfg.MaxCall = time.Duration(in[0]>>6) * time.Second
		if in[0]&64 != 0 {
			cfg.Auth = xorAuth(1)
		}
		in = in[1:]
	}
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	now := time.Duration(0)
	for op := 0; op < maxFuzzOps && len(in) >= 2; op++ {
		kind, arg := in[0], in[1]
		in = in[2:]
		ch := uint16(tsccChannel)
		if kind&0x80 != 0 {
			ch = 10 + uint16(kind>>6&1)
		}
		switch kind % 4 {
		case 0, 1:
			raw := pool[int(arg)%len(pool)]
			out = ts.Inbound(now, ch, enums.Slot(1+arg>>7), &raw, layer2.Hint{}, out[:0])
		case 2:
			for range int(arg % 16) {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
				if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
					t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
				}
			}
		case 3:
			addr := 3120000 + uint32(arg&3)
			switch arg >> 2 & 7 {
			case 0:
				_ = ts.Stun(now, addr, arg&32 != 0)
			case 1:
				_ = ts.Kill(now, addr)
			case 2:
				_ = ts.SendStatus(now, arg&32 != 0, addr, 0xFFFECB, trunk.Status(arg>>1))
			case 3:
				_ = ts.MassRegister(arg&15, arg>>4, addr)
			default:
				_ = ts.Interrupt(now, 10+uint16(arg>>5&1), enums.Slot(1+arg>>6&1), enums.RCCommand(arg&7))
			}
			out = out[:0]
		}
		if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
			t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
		}
	}
}

func tsccSvcSeeds() [][]byte {
	return [][]byte{
		{0x3F, 0, 9, 2, 7, 0, 11, 2, 7, 3, 0, 2, 255, 0, 21, 0, 22, 0, 23, 2, 9},
		{0x7E, 3, 4, 2, 9, 0, 17, 2, 3, 0, 18, 2, 200, 3, 9, 2, 255},
		{0xBF, 0, 2, 2, 7, 0x80, 24, 0x80, 25, 3, 16, 2, 255, 2, 255},
	}
}

func FuzzTSCCServices(f *testing.F) {
	for _, s := range tsccSvcSeeds() {
		f.Add(s)
	}
	pool := newTSCCSvcPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runTSCCServices(t, pool, in)
	})
}
