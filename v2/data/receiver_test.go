package data_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	srcID = 3120001
	dstID = 3120002
)

func utf16(p []byte) string {
	var b strings.Builder
	for i := 0; i+1 < len(p); i += 2 {
		if p[i+1] == 0 && p[i] >= 0x20 && p[i] < 0x7f {
			b.WriteByte(p[i])
		}
	}
	return b.String()
}

func TestReceiverSMSCaptures(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		format pdu.Format
		octets int
		blocks uint16
		end    time.Duration
	}{
		{"d-sms", pdu.FormatUnconfirmed, 56, 5, 960 * time.Millisecond},
		{"h-sms", pdu.FormatShortDataDefined, 26, 3, 840 * time.Millisecond},
		{"m-sms", pdu.FormatUnconfirmed, 62, 6, 1020 * time.Millisecond},
	} {
		ev := replay(t, newReceiver(t, nil), readBin(t, "../layer2/testdata/"+c.name+".bin", 0))
		if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" {
			t.Fatalf("%s: events %s", c.name, got)
		}
		want := data.Call{Target: 9990, Source: 3191868}
		if ev[0].Call != want {
			t.Errorf("%s: call %+v, want %+v", c.name, ev[0].Call, want)
		}
		m := ev[1].Message
		if m.Header.Format() != c.format || len(m.Payload) != c.octets || m.Rate != enums.DataTypeRate12 || m.BadCRC {
			t.Errorf("%s: %v, %d octets, %v, bad CRC %v", c.name, m.Header.Format(), len(m.Payload), m.Rate, m.BadCRC)
		}
		if !strings.Contains(utf16(m.Payload), "TEST KI5VMF") {
			t.Errorf("%s: text %q", c.name, utf16(m.Payload))
		}
		s := ev[2].Stats
		if s.Packets != 1 || s.Delivered != 1 || s.Blocks != c.blocks || s.Lost != 0 || s.Duration != c.end {
			t.Errorf("%s: stats %+v", c.name, s)
		}
	}
}

func TestReceiverAnyToneThroughXPR(t *testing.T) {
	t.Parallel()
	bursts := readTimed(t, "testdata/anytone_sms_xpr.txt")
	acks := map[layer2.RawBurst]bool{}
	for _, at := range []time.Duration{12123260 * time.Microsecond, 28563210 * time.Microsecond} {
		i := slices.IndexFunc(bursts, func(b timedBurst) bool { return b.at == at })
		acks[bursts[i].raw] = true
	}
	ipIDs := func(ev []data.Event) []byte {
		var ids []byte
		for _, e := range ev {
			if e.Kind == data.EventDelivered {
				ids = append(ids, e.Message.Payload[5])
				if !strings.Contains(utf16(e.Message.Payload), "THE QUICK BROWN FOX") || e.Message.Rate != enums.DataTypeRate34 {
					t.Errorf("payload %q at %v", utf16(e.Message.Payload), e.Message.Rate)
				}
			}
		}
		return ids
	}

	bridge := replay(t, newReceiver(t, nil), bursts)
	if got := ipIDs(bridge); !bytes.Equal(got, []byte{5, 5, 6, 7}) {
		t.Errorf("bridge: IP IDs %v, want [5 5 6 7]", got)
	}
	responses := ""
	for _, e := range bridge {
		if e.Kind == data.EventResponse {
			responses += e.Response.String() + "+"
		}
	}
	if responses != "SACK+ACK+ACK+ACK+" {
		t.Errorf("bridge: responses heard %q", responses)
	}

	cc := enums.ColorCode(7)
	ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true, Address: 3140598, ColorCode: &cc}), bursts)
	if got := ipIDs(ev); !bytes.Equal(got, []byte{5, 5, 6, 7}) {
		t.Errorf("endpoint: IP IDs %v, want [5 5 6 7]", got)
	}
	n, dups := 0, 0
	for _, e := range ev {
		switch e.Kind {
		case data.EventRespond:
			n++
			if e.Response != packet.ResponseACK || !acks[e.Burst] {
				t.Errorf("respond %v %x is not the ACK the radio took", e.Response, e.Burst)
			}
		case data.EventCallEnd:
			dups += int(e.Stats.Duplicates)
		case data.EventCallStart, data.EventDelivered, data.EventResponse, data.EventReassembled:
		}
	}
	if n != 4 || dups != 0 {
		t.Errorf("endpoint: %d ACKs and %d duplicates, want 4 and 0", n, dups)
	}
}

func build(t testing.TB, h pdu.DataHeader, payload []byte, rate enums.DataType) []layer2.RawBurst {
	t.Helper()
	p, err := packet.NewPacket(h, payload, rate)
	if err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&p, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func confirmedHeader(t testing.TB, ns uint8) pdu.DataHeader {
	t.Helper()
	return sapHeader(t, enums.SAPIPBasedPacketData, ns, false)
}

func sapHeader(t testing.TB, sap enums.SAP, ns uint8, resync bool) pdu.DataHeader {
	t.Helper()
	return setHeader(t, (*pdu.DataHeader).SetConfirmed, pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: sap, LLIDDestination: dstID, LLIDSource: srcID,
		FullMessageFlag: true, ReSynchronizeFlag: resync, SendSequenceNumber: ns, FragmentSequenceNumber: 0b1000,
	})
}

func timed(start time.Duration, bursts []layer2.RawBurst, drop ...int) []timedBurst {
	var out []timedBurst
	for i, b := range bursts {
		if !slices.Contains(drop, i) {
			out = append(out, timedBurst{at: start + time.Duration(i)*60*time.Millisecond, raw: b})
		}
	}
	return out
}

func payload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i*7 + 1)
	}
	return p
}

func responses(t *testing.T, ev []data.Event) (string, []int) {
	t.Helper()
	var s []string
	var retry []int
	a := packet.NewAssembler(2)
	for _, e := range ev {
		if e.Kind != data.EventRespond {
			continue
		}
		b, err := layer2.NewBurstFromBytes(e.Burst)
		if err != nil {
			t.Fatal(err)
		}
		m, done, err := a.AddBurst(b)
		if !done {
			continue
		}
		if err != nil {
			t.Fatalf("response: %v", err)
		}
		s = append(s, e.Response.String())
		if m.Response != nil && retry == nil {
			retry = m.Response.RetryBlocks()
		}
		rh, ok := m.Header.AsResponse()
		if !ok {
			t.Fatalf("response header %v is not a C_RHEAD", &m.Header)
		}
		if rh.LLIDDestination != srcID || rh.LLIDSource != dstID {
			t.Errorf("response from %d to %d", rh.LLIDSource, rh.LLIDDestination)
		}
	}
	return strings.Join(s, "+"), retry
}

func TestReceiverConfirmedFaults(t *testing.T) {
	t.Parallel()
	good := build(t, confirmedHeader(t, 0), payload(60), enums.DataTypeRate12)
	short := build(t, sapHeader(t, enums.SAPShortData, 0, false), payload(60), enums.DataTypeRate12)
	if len(good) != 8 {
		t.Fatalf("%d bursts, want a header and 7 blocks", len(good))
	}
	badCRC32 := slices.Clone(good)
	{
		blk := pdu.Rate12Data{}
		p, _ := badCRC32[3].BPTCPayload()
		blk.Data = p
		blk.Data[5] ^= 1
		var d [10]byte
		copy(d[:], blk.Data[2:])
		if err := blk.SetConfirmed(blk.Data[0]>>1, d); err != nil {
			t.Fatal(err)
		}
		badCRC32[3].SetBPTCPayload(blk.Data)
	}
	badHeader := slices.Clone(good)
	{
		p, _ := badHeader[0].BPTCPayload()
		p[4] ^= 0x10
		badHeader[0].SetBPTCPayload(p)
	}
	for _, c := range []struct {
		name      string
		bursts    []timedBurst
		delivered int
		resp      string
		retry     []int
	}{
		{"complete", timed(0, good), 1, "ACK", nil},
		{"middle block lost", timed(0, good, 3), 0, "SACK", []int{2}},
		{"two blocks lost", timed(0, good, 2, 5), 0, "SACK", []int{1, 4}},
		{"last block lost", timed(0, good, 7), 0, "SACK", []int{6}},
		{"message CRC", timed(0, badCRC32), 0, "NACK Packet CRC", nil},
		{"header CRC", timed(0, badHeader), 0, "", nil},
		{"IP twice", append(timed(0, good), timed(2*time.Second, good)...), 2, "ACK+ACK", nil},
		{"duplicate", append(timed(0, short), timed(2*time.Second, short)...), 1, "ACK+ACK", nil},
		{"selective retry", append(timed(0, good, 3), timed(time.Second, retryBursts(t, good, 2))...), 1, "SACK+ACK", []int{2}},
	} {
		r := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID, Source: layer2.SyncSourceBS})
		ev := replay(t, r, c.bursts)
		n := 0
		for _, e := range ev {
			if e.Kind == data.EventDelivered {
				n++
				if !bytes.Equal(e.Message.Payload, payload(60)) {
					t.Errorf("%s: payload %x", c.name, e.Message.Payload)
				}
			}
		}
		resp, retry := responses(t, ev)
		if n != c.delivered || resp != c.resp || !slices.Equal(retry, c.retry) {
			t.Errorf("%s: %d delivered, responses %q, retry %v; want %d, %q, %v (%s)",
				c.name, n, resp, retry, c.delivered, c.resp, c.retry, kinds(ev))
		}
	}
}

func retryBursts(t *testing.T, good []layer2.RawBurst, dbsn ...int) []layer2.RawBurst {
	t.Helper()
	p, err := packet.NewPacket(confirmedHeader(t, 0), payload(60), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rd packet.ResponseData
	for _, i := range dbsn {
		rd.Retry[i] = true
	}
	rp, err := packet.NewBuilder(len(p.Blocks)).Retry(&p, &rd)
	if err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&rp, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	if len(good) < 2 {
		t.Fatal("no packet")
	}
	return b
}

func TestReceiverSequence(t *testing.T) {
	t.Parallel()
	type pk struct {
		ns     uint8
		resync bool
		n      int
	}
	for _, c := range []struct {
		name      string
		sap       enums.SAP
		pkts      []pk
		resp      string
		status    []uint8
		delivered int
	}{
		{"short data", enums.SAPShortData,
			[]pk{{3, false, 20}, {4, false, 20}, {4, false, 20}, {4, false, 30}, {7, false, 20}, {5, false, 20}},
			"ACK+ACK+ACK+ACK+NACK Out Of Sequence+ACK", []uint8{3, 4, 4, 4, 4, 5}, 3},
		{"resync", enums.SAPShortData,
			[]pk{{2, false, 20}, {6, false, 20}, {6, true, 20}, {6, false, 20}, {7, false, 20}, {1, true, 20}},
			"ACK+NACK Out Of Sequence+ACK+ACK+ACK+ACK", []uint8{2, 2, 6, 6, 7, 1}, 4},
		{"IP", enums.SAPIPBasedPacketData,
			[]pk{{3, false, 20}, {4, false, 20}, {4, false, 20}, {4, false, 30}, {7, false, 20}, {0, false, 20}},
			"ACK+ACK+ACK+ACK+ACK+ACK", []uint8{3, 4, 4, 4, 7, 0}, 6},
		{"UDP/IP header compression", enums.SAPUDPIPHeaderCompression,
			[]pk{{0, false, 20}, {0, false, 20}, {5, false, 20}},
			"ACK+ACK+ACK", []uint8{0, 0, 5}, 3},
	} {
		bursts := make([]timedBurst, 0, 64)
		for i, p := range c.pkts {
			b := build(t, sapHeader(t, c.sap, p.ns, p.resync), payload(p.n), enums.DataTypeRate12)
			bursts = append(bursts, timed(time.Duration(i)*time.Second, b)...)
		}
		ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true}), bursts)
		resp, _ := responses(t, ev)
		if resp != c.resp {
			t.Errorf("%s: responses %q, want %q", c.name, resp, c.resp)
		}
		if got := statuses(t, ev); !slices.Equal(got, c.status) {
			t.Errorf("%s: status %v, want %v", c.name, got, c.status)
		}
		n := 0
		for _, e := range ev {
			if e.Kind == data.EventDelivered {
				n++
			}
		}
		if n != c.delivered {
			t.Errorf("%s: %d delivered, want %d (%s)", c.name, n, c.delivered, kinds(ev))
		}
	}
}

func TestReceiverProprietarySAP(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		sap       enums.SAP
		delivered int
	}{
		{enums.SAPIPBasedPacketData, 2},
		{enums.SAPShortData, 1},
	} {
		p, err := packet.NewPacket(sapHeader(t, enums.SAPProprietaryPacketData, 0, false), payload(20), enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		editConfirmed(t, &p.Header, func(c *pdu.ConfirmedDataHeader) { c.BlocksToFollow++ })
		b, err := packet.Bursts(&p, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		prop := setHeader(t, (*pdu.DataHeader).SetProprietary, pdu.ProprietaryHeader{SAP: c.sap, MFID: 0x10})
		ph, err := layer2.BuildBurst(&prop, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		b = slices.Insert(b, 1, ph)
		ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true}), append(timed(0, b), timed(2*time.Second, b)...))
		n := 0
		for _, e := range ev {
			if e.Kind == data.EventDelivered {
				n++
			}
		}
		if resp, _ := responses(t, ev); n != c.delivered || resp != "ACK+ACK" {
			t.Errorf("P_HEAD SAP %v: %d delivered, responses %q; want %d, ACK+ACK (%s)", c.sap, n, resp, c.delivered, kinds(ev))
		}
	}
}

func statuses(t *testing.T, ev []data.Event) []uint8 {
	t.Helper()
	var s []uint8
	for _, e := range ev {
		if e.Kind != data.EventRespond {
			continue
		}
		b, err := layer2.NewBurstFromBytes(e.Burst)
		if err != nil {
			t.Fatal(err)
		}
		if dh, ok := b.Data.(*pdu.DataHeader); ok {
			if rh, ok := dh.AsResponse(); ok {
				s = append(s, rh.ResponseStatus)
			}
		}
	}
	return s
}

func TestReceiverNoAnswer(t *testing.T) {
	t.Parallel()
	group := confirmedHeader(t, 0)
	editConfirmed(t, &group, func(c *pdu.ConfirmedDataHeader) { c.Group = true })
	for _, c := range []struct {
		name string
		cfg  data.ReceiverConfig
		h    pdu.DataHeader
	}{
		{"not respond", data.ReceiverConfig{}, confirmedHeader(t, 0)},
		{"other address", data.ReceiverConfig{Respond: true, Address: 99}, confirmedHeader(t, 0)},
		{"group", data.ReceiverConfig{Respond: true}, group},
	} {
		ev := replay(t, newReceiver(t, &c.cfg), timed(0, build(t, c.h, payload(20), enums.DataTypeRate34)))
		if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" {
			t.Errorf("%s: %s", c.name, got)
		}
	}
}

func TestReceiverTransportSeq(t *testing.T) {
	t.Parallel()
	good := build(t, confirmedHeader(t, 0), payload(60), enums.DataTypeRate12)
	r := newReceiver(t, &data.ReceiverConfig{Respond: true, Lost: -1})
	ev := make([]data.Event, 0, data.MaxEvents)
	var all []data.Event
	at := time.Duration(0)
	for i := range good {
		if i == 4 {
			continue
		}
		at += 5 * time.Millisecond
		ev = r.Burst(at, &good[i], layer2.Hint{HasSeq: true, Seq: uint16(65530 + i)}, ev[:0])
		all = append(all, ev...)
	}
	_, retry := responses(t, all)
	if !slices.Equal(retry, []int{3}) {
		t.Errorf("retry %v, want [3] (%s)", retry, kinds(all))
	}
}

func TestReceiverEnds(t *testing.T) {
	t.Parallel()
	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: dstID, LLIDSource: srcID, FullMessageFlag: true})
	pkt := build(t, u, payload(40), enums.DataTypeRate1)
	voice, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{}, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	idle, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	other := u
	if err := other.SetAddresses(dstID, 42); err != nil {
		t.Fatal(err)
	}
	otherPkt := build(t, other, payload(10), enums.DataTypeRate12)
	for _, c := range []struct {
		name   string
		next   layer2.RawBurst
		hint   layer2.Hint
		reason data.EndReason
	}{
		{"voice", voice, layer2.Hint{}, data.EndVoice},
		{"idle", idle, layer2.Hint{}, data.EndIdle},
		{"sync lost", idle, layer2.Hint{SyncLost: true}, data.EndSyncLost},
		{"transport", pkt[0], layer2.Hint{End: layer2.EndStrong}, data.EndTransport},
		{"superseded", otherPkt[0], layer2.Hint{}, data.EndSuperseded},
	} {
		r := newReceiver(t, nil)
		ev := make([]data.Event, 0, data.MaxEvents)
		at := time.Duration(0)
		for i := range pkt {
			ev = r.Burst(at, &pkt[i], layer2.Hint{}, ev[:0])
			at += 60 * time.Millisecond
		}
		ev = r.Burst(at, &c.next, c.hint, ev[:0])
		end := slices.IndexFunc(ev, func(e data.Event) bool { return e.Kind == data.EventCallEnd })
		if end < 0 || ev[end].Reason != c.reason || ev[end].Stats.Delivered != 1 {
			t.Errorf("%s: %s", c.name, kinds(ev))
		}
	}
	r := newReceiver(t, nil)
	ev := make([]data.Event, 0, data.MaxEvents)
	ev = r.Burst(0, &pkt[0], layer2.Hint{}, ev[:0])
	ev = r.Lost(60*time.Millisecond, ev[:0])
	if got := kinds(ev); got != "CallEnd(SyncLost)" || ev[0].Stats.Lost != uint16(len(pkt)-1) {
		t.Errorf("Lost: %s %+v", got, ev[0].Stats)
	}
	if r.State() != data.StateIdle {
		t.Errorf("state %v after Lost", r.State())
	}
}

func TestReceiverUDT(t *testing.T) {
	t.Parallel()
	h := setHeader(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{
		SAP: enums.SAPShortData, UDTFormat: 4, LLIDDestination: dstID, LLIDSource: srcID,
		PadNibble: 2, UAB: 1, Opcode: pdu.UDTOpcodeInboundHeader,
	})
	blocks := [][12]byte{[12]byte(payload(12)), {}}
	copy(blocks[1][:], "dmrgo UDT!")
	pdu.SetMBCLastBlockCRC(blocks)
	bursts := make([]layer2.RawBurst, 0, 3)
	for _, d := range []layer2.Data{&h, &pdu.Rate12Data{Data: blocks[0]}, &pdu.Rate12Data{Data: blocks[1]}} {
		b, err := layer2.BuildBurst(d, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		bursts = append(bursts, b)
	}
	ev := replay(t, newReceiver(t, nil), timed(0, bursts))
	if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" {
		t.Fatalf("events %s", got)
	}
	m := ev[1].Message
	want := append(payload(12), "dmrgo UDT"...)
	if uh, ok := m.Header.AsUDT(); !ok || uh.Opcode != pdu.UDTOpcodeInboundHeader || !bytes.Equal(m.Payload, want) {
		t.Errorf("message %+v, payload %q", m.Header, m.Payload)
	}

	bad := slices.Clone(bursts)
	p, _ := bad[2].BPTCPayload()
	p[0] ^= 1
	bad[2].SetBPTCPayload(p)
	ev = replay(t, newReceiver(t, nil), timed(0, bad))
	if got := kinds(ev); got != "CallStart CallEnd(Quiet)" || ev[1].Stats.BadCRC != 1 {
		t.Errorf("bad CRC: %s %+v", got, ev[len(ev)-1].Stats)
	}
	ev = replay(t, newReceiver(t, &data.ReceiverConfig{AcceptBadCRC: true}), timed(0, bad))
	if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" || !ev[1].Message.BadCRC {
		t.Errorf("AcceptBadCRC: %s", got)
	}
}

func TestReceiverDropped(t *testing.T) {
	t.Parallel()
	good := build(t, confirmedHeader(t, 0), payload(40), enums.DataTypeRate12)
	ev := replay(t, newReceiver(t, nil), timed(0, good[1:]))
	if len(ev) != 0 {
		t.Errorf("events %s for blocks without a header", kinds(ev))
	}
	r := newReceiver(t, &data.ReceiverConfig{MaxBlocks: 2})
	ev = replay(t, r, timed(0, good))
	if got := kinds(ev); got != "CallStart CallEnd(Quiet)" || ev[1].Stats.Dropped != uint16(len(good)) {
		t.Errorf("MaxBlocks: %s %+v", got, ev[len(ev)-1].Stats)
	}
}

func TestNewReceiverErrors(t *testing.T) {
	t.Parallel()
	cc := enums.ColorCode(16)
	for _, c := range []struct {
		cfg   data.ReceiverConfig
		field string
	}{
		{data.ReceiverConfig{ColorCode: &cc}, "ColorCode"},
		{data.ReceiverConfig{Source: 9}, "Source"},
		{data.ReceiverConfig{MaxBlocks: 128}, "MaxBlocks"},
		{data.ReceiverConfig{MaxBlocks: -1}, "MaxBlocks"},
		{data.ReceiverConfig{Peers: data.MaxPeers + 1}, "Peers"},
		{data.ReceiverConfig{MaxFragment: data.MaxMessage + 1}, "MaxFragment"},
		{data.ReceiverConfig{MaxMessage: -1}, "MaxMessage"},
	} {
		_, err := data.NewReceiver(&c.cfg)
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != c.field || !strings.Contains(err.Error(), c.field) {
			t.Errorf("%+v: %v, want a ConfigError for %s", c.cfg, err, c.field)
		}
	}
}

func TestReceiverStrings(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ got, want string }{
		{data.EventCallStart.String(), "CallStart"},
		{data.EventRespond.String(), "Respond"},
		{data.EventKind(99).String(), "EventKind(99)"},
		{data.EndQuiet.String(), "Quiet"},
		{data.EndSuperseded.String(), "Superseded"},
		{data.EndReason(0).String(), "EndReason(0)"},
		{data.StateReceiving.String(), "Receiving"},
		{data.State(9).String(), "State(9)"},
	} {
		if c.got != c.want {
			t.Errorf("%q, want %q", c.got, c.want)
		}
	}
}

func TestReceiverProprietaryHeader(t *testing.T) {
	t.Parallel()
	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPProprietaryPacketData, LLIDDestination: dstID, LLIDSource: srcID, FullMessageFlag: true})
	p, err := packet.NewPacket(u, payload(40), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	uh, ok := p.Header.AsUnconfirmed()
	if !ok {
		t.Fatalf("header %v is not a U_HEAD", &p.Header)
	}
	uh.BlocksToFollow++
	if err := p.Header.SetUnconfirmed(uh); err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&p, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	prop := setHeader(t, (*pdu.DataHeader).SetProprietary, pdu.ProprietaryHeader{SAP: enums.SAPIPBasedPacketData, MFID: 0x10})
	ph, err := layer2.BuildBurst(&prop, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	b = slices.Insert(b, 1, ph)
	check := func(name string, ev []data.Event) {
		t.Helper()
		end := ev[len(ev)-1]
		if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" || end.Stats.Lost != 0 ||
			ev[1].Message.Proprietary == nil || !bytes.Equal(ev[1].Message.Payload, payload(40)) {
			t.Errorf("%s: %s, stats %+v", name, got, end.Stats)
		}
	}
	check("time", replay(t, newReceiver(t, nil), timed(0, b)))
	r := newReceiver(t, &data.ReceiverConfig{Lost: -1})
	var all []data.Event
	ev := make([]data.Event, 0, data.MaxEvents)
	for i := range b {
		ev = r.Burst(time.Duration(i)*5*time.Millisecond, &b[i], layer2.Hint{HasSeq: true, Seq: uint16(100 + i)}, ev[:0])
		all = append(all, ev...)
	}
	all = append(all, r.Tick(time.Second, ev[:0])...)
	check("seq", all)
}
