//go:build !tinygo

package trunk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsMSUDT(t *testing.T) {
	r := onTSCC(t, &trunk.MSConfig{Ambient: true})
	var text, groups trunk.Appended
	if err := text.SetText(enums.UDTISO8, []byte("alloc free")); err != nil {
		t.Fatal(err)
	}
	if err := groups.SetAddresses(false, []uint32{501, 502}); err != nil {
		t.Fatal(err)
	}
	a := aloha(t)
	sdmi := ahoyBurst(t, enums.ServiceKindIndividualShortData, constants.GatewaySDMI, msAddr, false, 1)
	poll := ahoyBurst(t, enums.ServiceKindShortDataPolling, peer, msAddr, false, 0)
	usbd := usbdPoll(t, pdu.ServiceTypeLIP, 0, msAddr)
	done := ack(t, enums.AckReasonMSAccepted)
	dgna := download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &groups)
	r.ms.SetPollResponse(&text)
	r.ms.SetUSBDResponse(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP})
	buf := make([]byte, 0, trunk.MaxAppendedOctets*4)
	allocs := testing.AllocsPerRun(5, func() {
		_ = r.ms.SendShortData(r.now, false, peer, &text, layer3.ShortDataServiceOptions{})
		r.feed(a, &idle)
		r.feed(sdmi, &idle)
		r.feed(done, &idle)
		r.feed(poll, &idle)
		r.feed(usbd, &idle)
		for i := range dgna {
			r.feed(dgna[i], &idle)
		}
		_ = r.ms.Divert(r.now, constants.GatewayTSI, layer3.DiversionServiceOptions{Voice: true}, nil)
		r.feed(a, &idle)
		r.feed(done, &idle)
		ud, _ := r.ms.UDT()
		u := ud.Appended()
		buf, _ = u.Text(buf[:0])
		buf = r.ms.GroupAlias(buf[:0], 501)
		_ = text.SetText(enums.UDTUTF16BE, []byte("alloc free"))
		buf, _ = text.Text(buf[:0])
		_ = text.SetText(enums.UDTISO8, []byte("alloc free"))
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}
