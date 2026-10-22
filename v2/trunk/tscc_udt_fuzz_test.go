package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func upload(t testing.TB, h pdu.UDTHeader, a *trunk.Appended) []layer2.RawBurst {
	t.Helper()
	h.UDTFormat, h.PadNibble, h.UAB = a.Format, a.PadNibble, uint8(a.Blocks-1)
	if h.Opcode == 0 {
		h.Opcode = pdu.UDTOpcodeInboundHeader
	}
	blocks := make([][12]byte, a.Blocks)
	for i := range blocks {
		copy(blocks[i][:], a.Data[12*i:min(12*i+12, trunk.MaxAppendedOctets)])
	}
	pdu.SetMBCLastBlockCRC(blocks)
	hdr, err := layer2.BuildBurst(withPDU(t, &pdu.DataHeader{}, (*pdu.DataHeader).SetUDT, h), testCC, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]layer2.RawBurst, 0, 1+len(blocks))
	out = append(out, hdr)
	for _, b := range blocks {
		raw, err := layer2.BuildDataBurst(b, enums.DataTypeRate12, testCC, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func newTSCCUDTPool(t testing.TB) burstPool {
	t.Helper()
	rnd := func(kind enums.ServiceKind, opts, rsvd uint8, target uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceOptions: opts, Reserved: rsvd, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: 3120001}))
	}
	ackU := func(r enums.AckReason, info uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(r), TargetAddress: 3120001, AdditionalInfo: info}))
	}
	var text, addrs trunk.Appended
	if err := text.SetText(enums.UDTISO7, []byte("fuzz")); err != nil {
		t.Fatal(err)
	}
	if err := addrs.SetAddresses(false, []uint32{3120003}); err != nil {
		t.Fatal(err)
	}
	usbd, err := layer2.BuildBurst(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP}, testCC, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	pool := burstPool{
		rnd(enums.ServiceKindIndividualShortData, 0, 0, 3120002), rnd(enums.ServiceKindTalkgroupShortData, 0, 1, 9),
		rnd(enums.ServiceKindShortDataPolling, 0b0101, 0, 3120002), rnd(enums.ServiceKindCallDiversion, 0b0101000, 0, constants.GatewayMSI),
		rnd(enums.ServiceKindCallDiversion, 0b0001000, 0, constants.GatewayTSI), rnd(enums.ServiceKindSupplementary, 0, 1<<2, constants.GatewayDGNAI),
		rnd(enums.ServiceKindDuplexVoice, 0, 0, 3120002), rnd(enums.ServiceKindDuplexPacketData, 0, 0, 3120002),
		rnd(enums.ServiceKindIndividualVoice, 0, 0b0010, 3120002), rnd(enums.ServiceKindIndividualShortData, 0, 0, constants.GatewayPSTNI),
		ackU(enums.AckReasonMSAccepted, 3120002), ackU(enums.AckReasonMSNotSupported, 3120002), ackU(enums.AckReasonMSAccepted, 3120001), usbd,
		msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetMaintenance, pdu.MaintenancePDU{TargetAddress: 3120002, SourceAddress: 3120001})),
	}
	pool = append(pool, upload(t, pdu.UDTHeader{LLIDDestination: 3120002, LLIDSource: 3120001}, &text)...)
	pool = append(pool, upload(t, pdu.UDTHeader{OptionFlag: true, LLIDDestination: constants.GatewayMSI, LLIDSource: 3120001}, &addrs)...)
	pool = append(pool, upload(t, pdu.UDTHeader{LLIDDestination: 3120002, LLIDSource: 3120001, Opcode: pdu.UDTOpcodeDGNAInboundHeader}, &addrs)...)
	return append(pool, upload(t, pdu.UDTHeader{LLIDDestination: 3120001, LLIDSource: 3120002}, &text)...)
}

func runTSCCUDT(t *testing.T, pool burstPool, in []byte) {
	cfg := tsccConfig()
	cfg.Reg = false
	if len(in) > 0 {
		cfg.PayloadLen = 1 + int(in[0]&1)
		in = in[1:]
	}
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var a trunk.Appended
	if err := a.SetAddresses(true, []uint32{501}); err != nil {
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
			for range int(arg % 8) {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
				if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
					t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
				}
			}
		case 3:
			switch arg % 4 {
			case 0:
				_ = ts.SendShortData(now, arg&4 != 0, 3120002, &a)
			case 1:
				_ = ts.AssignGroups(now, 3120002, &a)
			case 2:
				_ = ts.PollShortData(now, 3120002, 1+int(arg>>4)%4)
			case 3:
				_ = ts.PollUSBD(now, 3120002, pdu.ServiceTypeLIP, arg>>4&3)
			}
			_, _, _ = ts.Diverted(3120001)
			out = out[:0]
		}
		if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
			t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
		}
	}
}

func tsccUDTSeeds() [][]byte {
	return [][]byte{
		{0, 0, 0, 2, 7, 0, 15, 0, 16, 0, 17, 2, 7, 0, 10, 2, 7},
		{1, 0, 3, 2, 7, 0, 21, 0, 22, 2, 7, 0, 4, 2, 7},
		{0, 0, 6, 2, 7, 0, 10, 2, 7, 0, 14, 2, 7, 3, 3, 2, 7, 0, 13},
		{1, 0, 2, 2, 5, 0, 27, 0, 28, 2, 7, 0, 12, 3, 0, 2, 7, 0, 10, 2, 255},
	}
}

func FuzzTSCCUDT(f *testing.F) {
	for _, s := range tsccUDTSeeds() {
		f.Add(s)
	}
	pool := newTSCCUDTPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runTSCCUDT(t, pool, in)
	})
}
