package data_test

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func ipHeader(t testing.TB, group bool, sap enums.SAP) pdu.DataHeader {
	t.Helper()
	return setHeader(t, (*pdu.DataHeader).SetUnconfirmedDataHeader, pdu.UnconfirmedDataHeader{
		SAP: sap, Group: group, LLIDDestination: 9, LLIDSource: 3120001,
	})
}

func TestOutputICMPGroup(t *testing.T) {
	t.Parallel()
	for _, group := range []bool{false, true} {
		s := newSender(t, &data.SenderConfig{TxLimit: 300 * time.Millisecond})
		s.Channel(0, true)
		m := &data.Message{Header: ipHeader(t, group, enums.SAPIPBasedPacketData), Rate: enums.DataTypeRate12, Payload: payload(40)}
		if err := s.Send(0, m); err != nil {
			t.Fatal(err)
		}
		out := s.Tick(300*time.Millisecond, nil)
		if len(out) != 1 || out[0].Result != data.ResultTimeLimit || out[0].Group != group {
			t.Fatalf("group %v: outputs %+v", group, out)
		}
		icmp, err := out[0].ICMP()
		if group && !errors.Is(err, ip.ErrNoICMP) || !group && (err != nil || icmp != ip.ICMPHostUnreachable) {
			t.Errorf("group %v: ICMP %v, %v", group, icmp, err)
		}
	}
}

func TestEventICMP(t *testing.T) {
	t.Parallel()
	bad := payload(40)
	for _, c := range []struct {
		name  string
		kind  data.EventKind
		call  bool
		group bool
		sap   enums.SAP
		ok    bool
	}{
		{"individual", data.EventDelivered, false, false, enums.SAPIPBasedPacketData, true},
		{"reassembled", data.EventReassembled, false, false, enums.SAPIPBasedPacketData, true},
		{"group header", data.EventDelivered, false, true, enums.SAPIPBasedPacketData, false},
		{"group call", data.EventDelivered, true, false, enums.SAPIPBasedPacketData, false},
		{"short data", data.EventDelivered, false, false, enums.SAPShortData, false},
		{"call start", data.EventCallStart, false, false, enums.SAPIPBasedPacketData, false},
	} {
		e := data.Event{
			Kind:    c.kind,
			Call:    data.Call{Group: c.call, Target: 9, Source: 3120001},
			Message: data.Message{Header: ipHeader(t, c.group, c.sap), Payload: bad},
		}
		m, pointer, err := e.ICMP(576)
		if c.ok && (err != nil || m != ip.ICMPBadHeader || pointer != 0) || !c.ok && !errors.Is(err, ip.ErrNoICMP) {
			t.Errorf("%s: %v %d %v", c.name, m, pointer, err)
		}
	}
	tms, err := hex.DecodeString(appcapture.TMS)
	if err != nil {
		t.Fatal(err)
	}
	e := data.Event{Kind: data.EventDelivered, Message: data.Message{Header: ipHeader(t, false, enums.SAPIPBasedPacketData), Payload: tms}}
	if _, _, err := e.ICMP(576); !errors.Is(err, ip.ErrNoICMP) {
		t.Errorf("good packet: %v", err)
	}
}
