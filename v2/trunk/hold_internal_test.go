package trunk

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestHeldUploadDiesWithItsTransaction(t *testing.T) {
	t.Parallel()
	ts, err := NewTSCC(&TSCCConfig{Channel: 1, Payload: [MaxPayloadChannels]uint16{10}, PayloadLen: 1})
	if err != nil {
		t.Fatal(err)
	}
	const a, b = 3120001, 3120002
	if !ts.addOp(udtOp{kind: opShortData, caller: a, called: b, until: constants.TNPTimerDefault}) {
		t.Fatal("no room for the op")
	}
	ts.u.in = UDT{Header: pdu.UDTHeader{Opcode: pdu.UDTOpcodeInboundHeader, LLIDSource: a, LLIDDestination: b}, CRCOK: true}
	copy(ts.u.in.Data[:], "FIRST")
	ts.u.downN, ts.u.downSlot = 2, enums.Slot1
	ts.uploaded(0, nil)
	if !ts.u.heldOn {
		t.Fatal("upload during a download not held")
	}
	late := time.Minute
	ts.udtTimers(late, nil)
	if !ts.addOp(udtOp{kind: opShortData, caller: a, called: b, until: late + constants.TNPTimerDefault}) {
		t.Fatal("no room for the second op")
	}
	ts.u.downN = 0
	ts.release(late, nil)
	if ts.u.downN != 0 {
		t.Errorf("the upload of the expired transaction went to %d with the second op", ts.u.hdr.LLIDDestination)
	}
}

func TestHeldAhoyDiesWithItsSetup(t *testing.T) {
	t.Parallel()
	const a, b = 3120001, 3120002
	for _, kind := range []enums.ServiceKind{enums.ServiceKindIndividualVoice, enums.ServiceKindStatusTransport} {
		ts, err := NewTSCC(&TSCCConfig{Channel: 1, Payload: [MaxPayloadChannels]uint16{10}, PayloadLen: 1, PowerSave: true})
		if err != nil {
			t.Fatal(err)
		}
		ts.svc.ps[0], ts.svc.nPS = psRec{addr: b, rq: 7, offset: 127}, 1
		r := pdu.RandomAccessPDU{ServiceKind: uint8(kind), TargetAddress: b, SourceAddress: a}
		if kind == enums.ServiceKindStatusTransport {
			ts.statusRandom(0, &r, nil)
		} else {
			ts.random(0, &r, nil)
		}
		held := func() int {
			n := 0
			for i := range ts.qLen {
				c := pdu.DecodeCSBK(ts.queue[(ts.qHead+i)%queueLen].p)
				if x, ok := c.AsAhoy(); ok && x.TargetAddress == b {
					n++
				}
			}
			return n
		}
		if held() == 0 {
			t.Fatalf("%v: no C_AHOY to B held", kind)
		}
		for now := time.Duration(0); now <= constants.TPTimerDefault+time.Second; now += slotLength {
			ts.Tick(now, nil)
		}
		if n := held(); n != 0 {
			t.Errorf("%v: %d C_AHOY to B still held after the setup timed out", kind, n)
		}
	}
}
