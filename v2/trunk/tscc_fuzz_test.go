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

func msBurst(t testing.TB, c *pdu.CSBK) layer2.RawBurst {
	t.Helper()
	raw, err := layer2.BuildBurst(c, testCC, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newInboundPool(t testing.TB) burstPool {
	t.Helper()
	rnd := func(kind enums.ServiceKind, opts uint8, target uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceOptions: opts, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: 3120001}))
	}
	return burstPool{
		rnd(enums.ServiceKindRegistration, 1, 0xFFFEC6),
		rnd(enums.ServiceKindRegistration, 0, 0xFFFEC6),
		rnd(enums.ServiceKindTalkgroupVoice, 0, 9),
		rnd(enums.ServiceKindIndividualVoice, 0, 3120002),
		rnd(enums.ServiceKindStatusTransport, 0, 3120002),
		msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: 3120001, AdditionalInfo: 3120002})),
		msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetMaintenance, pdu.MaintenancePDU{TargetAddress: 9, SourceAddress: 3120001})),
		voiceLCRaw(t, enums.DataTypeVoiceLCHeader),
		voiceLCRaw(t, enums.DataTypeTerminatorWithLC),
		msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(enums.AckReasonAuthResponseMS), TargetAddress: 0x123456, AdditionalInfo: 3120001})),
	}
}

func voiceLCRaw(t testing.TB, dt enums.DataType) layer2.RawBurst {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, 9, 3120001, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lc.BurstDataType = dt
	raw, err := layer2.BuildBurst(&lc, testCC, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func runTSCC(t *testing.T, pool burstPool, in []byte) {
	cfg := tsccConfig()
	if len(in) > 0 {
		cfg.PayloadLen = 1 + int(in[0]&1)
		cfg.Reg = in[0]&2 != 0
		cfg.Authenticate = in[0]&4 != 0
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
		if kind&1 == 0 {
			raw := pool[int(arg)%len(pool)]
			out = ts.Inbound(now, ch, &raw, layer2.Hint{Slot: enums.Slot(1 + arg>>7)}, out[:0])
			for _, o := range out {
				if o.Kind == trunk.TSCCAuthChallenge && kind&2 != 0 {
					ts.ExpectAuth(o.Address, 0x123456)
				}
			}
		} else {
			for range int(arg % 8) {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
				if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
					t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
				}
			}
		}
		if len(out) > trunk.MaxTSCCOutputs || cap(out) != trunk.MaxTSCCOutputs {
			t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
		}
	}
}

func tsccSeeds() [][]byte {
	return [][]byte{
		{3, 0, 0, 1, 7, 2, 2, 1, 7, 0x81, 7, 6, 3, 1, 200},
		{0, 3, 3, 1, 7, 5, 5, 1, 255, 0xc0, 8, 0x80, 6, 1, 255},
		{6, 2, 0, 1, 1, 0, 9, 1, 7, 0, 0, 1, 255, 1, 255, 1, 255},
	}
}

func FuzzTSCC(f *testing.F) {
	for _, s := range tsccSeeds() {
		f.Add(s)
	}
	pool := newInboundPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runTSCC(t, pool, in)
	})
}
