package packet_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func laterTry(t *testing.T, p packet.Packet, blocks []int) packet.Packet {
	t.Helper()
	h := p.Header
	n := uint8(len(blocks))
	if c, ok := h.AsConfirmed(); ok {
		c.FullMessageFlag, c.BlocksToFollow = false, n
		mustSet(t, h.SetConfirmed(c))
	} else if c, ok := h.AsUnconfirmed(); ok {
		c.FullMessageFlag, c.BlocksToFollow = false, n
		mustSet(t, h.SetUnconfirmed(c))
	} else if c, ok := h.AsDefinedData(); ok {
		c.FullMessageFlag, c.AppendedBlocks = false, n
		mustSet(t, h.SetDefinedData(c))
	} else if c, ok := h.AsRawData(); ok {
		c.FullMessageFlag, c.AppendedBlocks = false, n
		mustSet(t, h.SetRawData(c))
	} else {
		t.Fatalf("header %v", h.Format())
	}
	out := packet.Packet{Header: h}
	for _, i := range blocks {
		out.Blocks = append(out.Blocks, p.Blocks[i])
	}
	return out
}

func allBlocks(p packet.Packet) []int {
	out := make([]int, len(p.Blocks))
	for i := range out {
		out[i] = i
	}
	return out
}

func corrupt(t *testing.T, p packet.Packet, i int) layer2.Data {
	t.Helper()
	b := encodeBlock(t, p.Blocks[i])
	switch r := b.(type) {
	case *pdu.Rate12Data:
		r.Data[4] ^= 0x08
	case *pdu.Rate34Data:
		r.Data[4] ^= 0x08
	case *pdu.Rate1Data:
		r.Data[4] ^= 0x08
	}
	return b
}

func TestAssembler_FirstHeaderLost(t *testing.T) {
	t.Parallel()

	data := payload(25)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	pad := int(get(t, &p.Header, (*pdu.DataHeader).AsConfirmed).PadOctetCount)
	if pad == 0 {
		t.Fatal("test needs pad octets")
	}
	rx := packet.NewAssembler(127)
	for i, b := range p.Blocks {
		if _, err := add(rx, encodeBlock(t, b)); !errors.Is(err, packet.ErrNoHeader) {
			t.Fatalf("block %d of the first try: %v, want ErrNoHeader", i, err)
		}
	}
	m, err := feed(t, rx, laterTry(t, p, allBlocks(p)))
	if err != nil || m == nil {
		t.Fatalf("later try: %v %v", m, err)
	}
	if len(m.Data) != len(data)+pad || !bytes.Equal(m.Data[:len(data)], data) || get(t, &m.Header, (*pdu.DataHeader).AsConfirmed).FullMessageFlag {
		t.Fatalf("data %x, want %x and %d pad octets", m.Data, data, pad)
	}
}

func TestAssembler_FirstHeaderLostPartialRetry(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(confirmed(), payload(60), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	if m, err := feed(t, rx, laterTry(t, p, []int{1, 4})); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("retry of 1 and 4: %v %v, want ErrBlocksMissing", m, err)
	}
	rx.Reset()
	if _, err := feed(t, rx, laterTry(t, p, []int{0, 1})); !errors.Is(err, packet.ErrMessageCRC) {
		t.Fatalf("retry of 0 and 1: %v, want ErrMessageCRC", err)
	}
}

func TestAssembler_RetryWhileBlocksLost(t *testing.T) {
	t.Parallel()

	data := payload(60)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	if m, err := add(rx, encodeHeader(t, &p.Header)); m != nil || err != nil {
		t.Fatal(m, err)
	}
	for i, b := range p.Blocks {
		if i == 1 || i == len(p.Blocks)-1 {
			continue
		}
		if m, err := add(rx, encodeBlock(t, b)); m != nil || err != nil {
			t.Fatalf("block %d: %v %v", i, m, err)
		}
	}
	m, err := feed(t, rx, laterTry(t, p, []int{1, len(p.Blocks) - 1}))
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after the later try: %v %+v", err, m)
	}
}

func TestAssembler_UnexpectedRetry(t *testing.T) {
	t.Parallel()

	data := payload(30)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	other := confirmedHeader()
	other.SendSequenceNumber = 6
	q, err := packet.NewPacket(header(t, (*pdu.DataHeader).SetConfirmed, other), payload(30), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	if _, err := add(rx, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	if _, err := add(rx, encodeBlock(t, p.Blocks[0])); err != nil {
		t.Fatal(err)
	}
	stray := laterTry(t, q, []int{1})
	if m, err := add(rx, encodeHeader(t, &stray.Header)); m != nil || !errors.Is(err, packet.ErrUnexpectedRetry) {
		t.Fatalf("stray later try: %v %v, want ErrUnexpectedRetry", m, err)
	}
	var m *packet.Message
	for _, b := range p.Blocks[1:] {
		if m, err = add(rx, encodeBlock(t, b)); err != nil {
			t.Fatal(err)
		}
	}
	if m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("packet in progress: %+v", m)
	}
}

func TestAssembler_ShortDataRetry(t *testing.T) {
	t.Parallel()

	headers := map[string]pdu.DataHeader{
		"DD_HEAD": header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
			SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, DefinedData: 2, SARQ: true, FullMessageFlag: true}),
		"R_HEAD": header(t, (*pdu.DataHeader).SetRawData, pdu.RawDataHeader{
			SAP: enums.SAPShortData, AppendedBlocks: 1, LLIDDestination: 1, LLIDSource: 2, SourcePort: 1, DestinationPort: 2, SARQ: true, FullMessageFlag: true}),
	}
	for name, h := range headers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := payload(40)
			p, err := packet.NewPacket(h, data, enums.DataTypeRate12)
			if err != nil {
				t.Fatal(err)
			}
			rx := packet.NewAssembler(127)
			if _, err := add(rx, encodeHeader(t, &p.Header)); err != nil {
				t.Fatal(err)
			}
			var last error
			for i, b := range p.Blocks {
				if i == 2 {
					b = corrupt(t, p, i)
				} else {
					b = encodeBlock(t, b)
				}
				_, last = add(rx, b)
			}
			if !errors.Is(last, packet.ErrBlocksMissing) || !slices.Equal(rx.Missing(), []int{2}) {
				t.Fatalf("first try: %v, missing %v", last, rx.Missing())
			}
			m, err := feed(t, rx, laterTry(t, p, []int{2}))
			if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
				t.Fatalf("later try: %v %+v", err, m)
			}
		})
	}
}

func TestAssembler_OrphanRetry(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(confirmed(), payload(60), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	rx.AcceptBadCRC = true
	if m, err := feed(t, rx, laterTry(t, p, []int{0, 1})); m == nil || !m.BadCRC || !errors.Is(err, packet.ErrMessageCRC) {
		t.Fatalf("partial later try with AcceptBadCRC: %v %+v, want ErrMessageCRC", err, m)
	}
	rx.Reset()
	if _, err := feed(t, rx, laterTry(t, p, []int{3, 4})); !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("later try of 3 and 4: %v", err)
	}
	if _, err := rx.Response(); !errors.Is(err, packet.ErrNoFirstTry) {
		t.Fatalf("Response: %v, want ErrNoFirstTry", err)
	}

	next := confirmedHeader()
	next.SendSequenceNumber = 6
	data := payload(40)
	q, err := packet.NewPacket(header(t, (*pdu.DataHeader).SetConfirmed, next), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	m, err := feed(t, rx, laterTry(t, q, allBlocks(q)))
	if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
		t.Fatalf("full later try of the next packet: %v %+v", err, m)
	}
}
