package zeroalloc_test

import (
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// captureBursts returns the bursts of a capture under layer2/testdata, each
// decoded into its own Burst.
func captureBursts(t testing.TB, name string) []*layer2.Burst {
	t.Helper()
	data, err := os.ReadFile("../../layer2/testdata/" + name + ".bin")
	if err != nil || len(data)%len(layer2.RawBurst{}) != 0 {
		t.Fatalf("read %s: %v", name, err)
	}
	out := make([]*layer2.Burst, 0, len(data)/len(layer2.RawBurst{}))
	for i := 0; i < len(data); i += len(layer2.RawBurst{}) {
		b, err := layer2.NewBurstFromBytes(layer2.RawBurst(data[i:]))
		if err != nil {
			t.Fatalf("%s burst %d: %v", name, len(out), err)
		}
		out = append(out, b)
	}
	return out
}

// addBursts adds every burst to a and fails unless exactly one packet
// completes.
func addBursts(t testing.TB, a *packet.Assembler, bursts []*layer2.Burst) {
	t.Helper()
	msgs := 0
	for _, b := range bursts {
		_, done, err := a.AddBurst(b)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			msgs++
		}
	}
	if msgs != 1 {
		t.Fatalf("%d messages", msgs)
	}
}

func confirmedHeader() pdu.DataHeader {
	return pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002,
		LLIDSource: 3120001, FullMessageFlag: true, SendSequenceNumber: 5, FragmentSequenceNumber: 8}}
}

func unconfirmedHeader() pdu.DataHeader {
	return pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001, FullMessageFlag: true}}
}

// results keeps the results of measured calls, so that they escape as they
// do in a caller that keeps them.
type results struct {
	pkt     packet.Packet
	raws    []layer2.RawBurst
	ints    []int
	rate12s []*pdu.Rate12Data
	data    packet.ResponseData
}

func octets(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i*7 + 3)
	}
	return p
}

// packetEntries returns the entries of package layer2/packet. The bursts
// of the d-sms, m-sms and h-sms captures are decoded once, outside the
// measured calls.
func packetEntries(t testing.TB) []entry {
	t.Helper()
	out := assemblerEntries(t)
	keep := new(results)

	uh, ch := unconfirmedHeader(), confirmedHeader()
	payload := octets(53)
	var b packet.Builder
	segmentAllocs := map[string]float64{
		"Unconfirmed Rate 1/2": 1, "Unconfirmed Rate 3/4": 1, "Unconfirmed Rate 1": 1,
		"Confirmed Rate 1/2": 13, "Confirmed Rate 3/4": 9, "Confirmed Rate 1": 7,
	}
	for _, h := range []*pdu.DataHeader{&uh, &ch} {
		for _, rate := range []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1} {
			must(b.Segment(*h, payload, rate))
			name := h.Format.String() + " " + rate.String()
			out = append(out, entry{"layer2/packet.Builder.Segment", name, segmentAllocs[name], func() {
				keep.pkt, _ = b.Segment(*h, payload, rate)
			}})
		}
	}
	out = append(out,
		entry{"layer2/packet.Segment", "unconfirmed rate 1/2", 5, func() {
			keep.pkt, _ = packet.Segment(uh, payload, enums.DataTypeRate12)
		}},
		entry{"layer2/packet.Segment", "confirmed rate 3/4", 13, func() {
			keep.pkt, _ = packet.Segment(ch, payload, enums.DataTypeRate34)
		}},
	)

	conf := must(packet.Segment(ch, octets(50), enums.DataTypeRate12))
	dst := make([]layer2.RawBurst, 0, 1+len(conf.Blocks))
	out = append(out,
		entry{"layer2/packet.Bursts", "confirmed rate 1/2", 15, func() {
			keep.raws, _ = packet.Bursts(conf, 1, layer2.SyncSourceBS)
		}},
		entry{"layer2/packet.AppendBursts", "confirmed rate 1/2", 13, func() {
			keep.raws, _ = packet.AppendBursts(dst[:0], &conf, 1, layer2.SyncSourceBS)
		}},
	)
	return append(out, responseEntries(t)...)
}

// assemblerEntries returns the entries of packet.Assembler.
func assemblerEntries(t testing.TB) []entry {
	t.Helper()
	var out []entry
	freshAllocs := map[string]float64{"d-sms": 3, "m-sms": 3, "h-sms": 2}
	reuseAllocs := map[string]float64{"d-sms": 1, "m-sms": 1, "h-sms": 0}
	for _, name := range []string{"d-sms", "m-sms", "h-sms"} {
		bursts := captureBursts(t, name)
		var fresh packet.Assembler
		addBursts(t, &fresh, bursts)
		reuse := packet.Assembler{Reuse: true}
		addBursts(t, &reuse, bursts)
		out = append(out,
			entry{"layer2/packet.Assembler.AddBurst", name, freshAllocs[name], func() {
				for _, b := range bursts {
					_, _, _ = fresh.AddBurst(b)
				}
			}},
			entry{"layer2/packet.Assembler.AddBurst", name + " reuse", reuseAllocs[name], func() {
				for _, b := range bursts {
					_, _, _ = reuse.AddBurst(b)
				}
			}},
		)
	}

	conf := must(packet.Segment(confirmedHeader(), octets(50), enums.DataTypeRate34))
	hdr := conf.Header
	reuse := packet.Assembler{Reuse: true}
	addAll := func() {
		_, _, _ = reuse.Add(&hdr)
		for _, d := range conf.Blocks {
			_, _, _ = reuse.Add(d)
		}
	}
	addAll()
	udt := pdu.DataHeader{Format: pdu.FormatUnifiedDataTransport, UDTHeader: &pdu.UDTHeader{SAP: 1}}
	var ign, single packet.Assembler
	out = append(out,
		entry{"layer2/packet.Assembler.Add", "confirmed rate 3/4 reuse", 9, addAll},
		entry{"layer2/packet.Assembler.Add", "UDT header (ignored)", 0, func() { _, _, _ = ign.Add(&udt) }},
		entry{"layer2/packet.Assembler.Add", "C_HEAD", 0, func() { _, _, _ = single.Add(&hdr) }},
		entry{"layer2/packet.Assembler.Reset", "reuse", 0, reuse.Reset},
	)

	var pending packet.Assembler
	pendingReuse := packet.Assembler{Reuse: true}
	for _, a := range []*packet.Assembler{&pending, &pendingReuse} {
		if _, _, err := a.Add(&hdr); err != nil {
			t.Fatal(err)
		}
		if _, _, err := a.Add(conf.Blocks[0]); err != nil {
			t.Fatal(err)
		}
	}
	missing := make([]int, 0, len(conf.Blocks))
	keep := new(results)
	must(pendingReuse.Response())
	return append(out,
		entry{"layer2/packet.Assembler.Missing", "3 of 4", 1, func() { keep.ints = pending.Missing() }},
		entry{"layer2/packet.Assembler.AppendMissing", "3 of 4", 0, func() { keep.ints = pending.AppendMissing(missing[:0]) }},
		entry{"layer2/packet.Assembler.Response", "SACK", 3, func() { keep.pkt, _ = pending.Response() }},
		entry{"layer2/packet.Assembler.Response", "SACK reuse", 0, func() { keep.pkt, _ = pendingReuse.Response() }},
	)
}

// responseEntries returns the entries of the response packet functions.
func responseEntries(t testing.TB) []entry {
	t.Helper()
	var rd packet.ResponseData
	rd.Retry[1], rd.Retry[70] = true, true
	one := must(rd.Encode(64))
	two := must(rd.Encode(100))
	bad := *one[0]
	bad.Data[0] ^= 0x80
	blocks := make([]pdu.Rate12Data, 0, 2)
	retry := make([]int, 0, 2)
	ch := confirmedHeader()
	ch.ConfirmedDataHeader.BlocksToFollow = 100
	var b packet.Builder
	must(b.NewResponse(ch.ConfirmedDataHeader, packet.ResponseSACK, &rd))
	rh := pdu.ResponsePacketHeader{ResponseClass: 2}
	keep := new(results)
	return []entry{
		{"layer2/packet.ResponseData.Encode", "100 blocks", 2, func() { keep.rate12s, _ = rd.Encode(100) }},
		{"layer2/packet.ResponseData.AppendEncode", "100 blocks", 0, func() { blocks, _ = rd.AppendEncode(blocks[:0], 100) }},
		{"layer2/packet.DecodeResponseData", "one block", 0, func() { keep.data, _ = packet.DecodeResponseData(one...) }},
		{"layer2/packet.DecodeResponseData", "two blocks", 0, func() { keep.data, _ = packet.DecodeResponseData(two...) }},
		{"layer2/packet.DecodeResponseData", "bad CRC", 0, func() { keep.data, _ = packet.DecodeResponseData(&bad) }},
		{"layer2/packet.ResponseData.RetryBlocks", "2 blocks", 1, func() { keep.ints = rd.RetryBlocks() }},
		{"layer2/packet.ResponseData.AppendRetryBlocks", "2 blocks", 0, func() { retry = rd.AppendRetryBlocks(retry[:0]) }},
		{"layer2/packet.NewResponse", "ACK", 1, func() {
			keep.pkt, _ = packet.NewResponse(ch.ConfirmedDataHeader, packet.ResponseACK, nil)
		}},
		{"layer2/packet.NewResponse", "SACK", 3, func() {
			keep.pkt, _ = packet.NewResponse(ch.ConfirmedDataHeader, packet.ResponseSACK, &rd)
		}},
		{"layer2/packet.Builder.NewResponse", "ACK", 0, func() {
			keep.pkt, _ = b.NewResponse(ch.ConfirmedDataHeader, packet.ResponseACK, nil)
		}},
		{"layer2/packet.Builder.NewResponse", "SACK", 0, func() {
			keep.pkt, _ = b.NewResponse(ch.ConfirmedDataHeader, packet.ResponseSACK, &rd)
		}},
		{"layer2/packet.NewResponseCode", "SACK", 0, func() { _ = packet.NewResponseCode(2, 0) }},
		{"layer2/packet.ResponseOf", "SACK", 0, func() { _ = packet.ResponseOf(&rh) }},
		{"layer2/packet.Response.Class", "SACK", 0, func() { _ = packet.ResponseSACK.Class() }},
		{"layer2/packet.Response.Type", "SACK", 0, func() { _ = packet.ResponseSACK.Type() }},
	}
}
