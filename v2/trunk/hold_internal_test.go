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
