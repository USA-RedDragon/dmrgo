package trunk

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCancelCallEndsTheWaitForTheGrant(t *testing.T) {
	t.Parallel()
	const addr, caller, other = 3120001, 3120002, 3120003
	m, err := NewMS(&MSConfig{Address: addr, NoRegistration: true})
	if err != nil {
		t.Fatal(err)
	}
	h := layer2.Hint{Slot: enums.Slot1}
	for _, c := range []struct {
		kind    enums.ServiceKind
		from    uint32
		pending bool
		outputs int
	}{
		{enums.ServiceKindIndividualVoice, caller, true, 2},
		{enums.ServiceKindCancelCall, other, true, 1},
		{enums.ServiceKindCancelCall, caller, false, 2},
	} {
		a := pdu.AhoyPDU{ServiceKind: uint8(c.kind), TargetAddress: addr, SourceAddress: c.from}
		out := m.ahoyAny(time.Second, &a, &h, nil)
		if m.pending != c.pending || len(out) != c.outputs {
			t.Errorf("after %v from %d: pending %v, %d outputs, want %v and %d", c.kind, c.from, m.pending, len(out), c.pending, c.outputs)
		}
	}
}

func TestDuplexAhoyWaitsForThePowerSaveSlot(t *testing.T) {
	t.Parallel()
	const a, b = 3120001, 3120002
	for _, kind := range []enums.ServiceKind{enums.ServiceKindDuplexVoice, enums.ServiceKindShortDataPolling} {
		ts, err := NewTSCC(&TSCCConfig{Channel: 1, Payload: [MaxPayloadChannels]uint16{10}, PayloadLen: 1, PowerSave: true})
		if err != nil {
			t.Fatal(err)
		}
		ts.svc.ps[0], ts.svc.nPS = psRec{addr: b, rq: 7, offset: 127}, 1
		ts.random(0, &pdu.RandomAccessPDU{ServiceKind: uint8(kind), TargetAddress: b, SourceAddress: a}, nil)
		for now := time.Duration(0); now < time.Second; now += slotLength {
			for _, o := range ts.Tick(now, nil) {
				if o.Kind != TSCCBurst {
					continue
				}
				bu, err := layer2.NewBurstFromBytes(o.Burst)
				if err != nil {
					continue
				}
				if c, ok := bu.Data.(*pdu.CSBK); ok {
					if x, ok := c.AsAhoy(); ok && x.TargetAddress == b {
						t.Fatalf("%v: C_AHOY to the sleeping B at %v", kind, now)
					}
				}
			}
		}
	}
}

func TestCancelAckIsNotADuplexAnswer(t *testing.T) {
	t.Parallel()
	const a, b = 3120001, 3120002
	ts, err := NewTSCC(&TSCCConfig{Channel: 1, Payload: [MaxPayloadChannels]uint16{10}, PayloadLen: 1, PowerSave: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Duration(0)
	tick := func(n int) {
		for range n {
			for _, o := range ts.Tick(now, nil) {
				if o.Kind == TSCCCallStart {
					t.Errorf("call start %+v", o.Grant)
				}
			}
			now += slotLength
		}
	}
	ts.random(now, &pdu.RandomAccessPDU{ServiceKind: uint8(enums.ServiceKindIndividualVoice), TargetAddress: b, SourceAddress: a}, nil)
	tick(10)
	ts.svc.ps[0], ts.svc.nPS = psRec{addr: b, rq: 7, offset: 127}, 1
	ts.random(now, &pdu.RandomAccessPDU{ServiceKind: uint8(enums.ServiceKindCancelCall), TargetAddress: b, SourceAddress: a}, nil)
	tick(10)
	ts.random(now, &pdu.RandomAccessPDU{ServiceKind: uint8(enums.ServiceKindDuplexVoice), TargetAddress: b, SourceAddress: a}, nil)
	tick(10)
	ts.unqueue(ts.queue[ts.qHead].p)
	for _, o := range ts.answer(now, &pdu.AckInboundPDU{ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: a, AdditionalInfo: b}, nil) {
		if o.Kind == TSCCCallStart {
			t.Errorf("B's C_ACKU to the C_AHOY Cancel Call started %+v", o.Grant)
		}
	}
}
