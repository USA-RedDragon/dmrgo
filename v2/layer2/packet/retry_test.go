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

// laterTry returns a copy of the header of p for a later try that resends
// the blocks with the given indexes: F clear and the block count set to
// their number.
func laterTry(t *testing.T, p packet.Packet, blocks []int) packet.Packet {
	t.Helper()
	h := p.Header
	n := uint8(len(blocks))
	switch {
	case h.ConfirmedDataHeader != nil:
		c := *h.ConfirmedDataHeader
		c.FullMessageFlag, c.BlocksToFollow = false, n
		h.ConfirmedDataHeader = &c
	case h.UnconfirmedDataHeader != nil:
		c := *h.UnconfirmedDataHeader
		c.FullMessageFlag, c.BlocksToFollow = false, n
		h.UnconfirmedDataHeader = &c
	case h.DefinedDataHeader != nil:
		c := *h.DefinedDataHeader
		c.FullMessageFlag, c.AppendedBlocks = false, n
		h.DefinedDataHeader = &c
	case h.RawDataHeader != nil:
		c := *h.RawDataHeader
		c.FullMessageFlag, c.AppendedBlocks = false, n
		h.RawDataHeader = &c
	default:
		t.Fatalf("header %v", h.Format)
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

// corrupt returns block i of p encoded and decoded, with one bit flipped so
// that its CRC-9 fails.
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

// TestAssembler_FirstHeaderLost loses the C_HEAD of the first try. A later
// try that resends every block (F clear) then delivers the packet; its
// pad octets stay in Data, because POC counts them only on the first try.
func TestAssembler_FirstHeaderLost(t *testing.T) {
	t.Parallel()

	data := payload(25)
	p, err := packet.Segment(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	pad := int(p.Header.ConfirmedDataHeader.PadOctetCount)
	if pad == 0 {
		t.Fatal("test needs pad octets")
	}
	var rx packet.Assembler
	for i, b := range p.Blocks {
		if _, err := add(&rx, encodeBlock(t, b)); !errors.Is(err, packet.ErrNoHeader) {
			t.Fatalf("block %d of the first try: %v, want ErrNoHeader", i, err)
		}
	}
	m, err := feed(t, &rx, laterTry(t, p, allBlocks(p)))
	if err != nil || m == nil {
		t.Fatalf("later try: %v %v", m, err)
	}
	if len(m.Data) != len(data)+pad || !bytes.Equal(m.Data[:len(data)], data) || m.Header.ConfirmedDataHeader.FullMessageFlag {
		t.Fatalf("data %x, want %x and %d pad octets", m.Data, data, pad)
	}
}

// TestAssembler_FirstHeaderLostPartialRetry loses the C_HEAD of the first
// try and gets a later try of some blocks only. The Assembler does not
// deliver it: the DBSNs 0 to BF-1 are not all there, or the message CRC
// fails.
func TestAssembler_FirstHeaderLostPartialRetry(t *testing.T) {
	t.Parallel()

	p, err := packet.Segment(confirmed(), payload(60), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rx packet.Assembler
	if m, err := feed(t, &rx, laterTry(t, p, []int{1, 4})); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("retry of 1 and 4: %v %v, want ErrBlocksMissing", m, err)
	}
	rx.Reset()
	if _, err := feed(t, &rx, laterTry(t, p, []int{0, 1})); !errors.Is(err, packet.ErrMessageCRC) {
		t.Fatalf("retry of 0 and 1: %v, want ErrMessageCRC", err)
	}
}

// TestAssembler_RetryWhileBlocksLost gets a later try while blocks of the
// first try are still missing, as when a block burst was lost on air and
// nobody called Lost. The later try fills the gap.
func TestAssembler_RetryWhileBlocksLost(t *testing.T) {
	t.Parallel()

	data := payload(60)
	p, err := packet.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	var rx packet.Assembler
	if m, err := add(&rx, encodeHeader(t, &p.Header)); m != nil || err != nil {
		t.Fatal(m, err)
	}
	for i, b := range p.Blocks {
		if i == 1 || i == len(p.Blocks)-1 {
			continue
		}
		if m, err := add(&rx, encodeBlock(t, b)); m != nil || err != nil {
			t.Fatalf("block %d: %v %v", i, m, err)
		}
	}
	m, err := feed(t, &rx, laterTry(t, p, []int{1, len(p.Blocks) - 1}))
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after the later try: %v %+v", err, m)
	}
}

// TestAssembler_UnexpectedRetry gets a later try of another packet while a
// packet is in progress. The Assembler drops the header and the packet in
// progress still completes.
func TestAssembler_UnexpectedRetry(t *testing.T) {
	t.Parallel()

	data := payload(30)
	p, err := packet.Segment(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	other := confirmed()
	other.ConfirmedDataHeader.SendSequenceNumber = 6
	q, err := packet.Segment(other, payload(30), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rx packet.Assembler
	if _, err := add(&rx, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	if _, err := add(&rx, encodeBlock(t, p.Blocks[0])); err != nil {
		t.Fatal(err)
	}
	stray := laterTry(t, q, []int{1})
	if m, err := add(&rx, encodeHeader(t, &stray.Header)); m != nil || !errors.Is(err, packet.ErrUnexpectedRetry) {
		t.Fatalf("stray later try: %v %v, want ErrUnexpectedRetry", m, err)
	}
	var m *packet.Message
	for _, b := range p.Blocks[1:] {
		if m, err = add(&rx, encodeBlock(t, b)); err != nil {
			t.Fatal(err)
		}
	}
	if m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("packet in progress: %+v", m)
	}
}

// TestAssembler_ShortDataRetry retries a short data packet with SARQ set,
// for a DD_HEAD and an R_HEAD: one block fails its CRC-9, and a later try
// with that block completes the packet.
func TestAssembler_ShortDataRetry(t *testing.T) {
	t.Parallel()

	headers := map[string]pdu.DataHeader{
		"DD_HEAD": {Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{
			SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, DefinedData: 2, SARQ: true, FullMessageFlag: true}},
		"R_HEAD": {Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{
			SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, SourcePort: 1, DestinationPort: 2, SARQ: true, FullMessageFlag: true}},
	}
	for name, h := range headers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := payload(40)
			p, err := packet.Segment(h, data, enums.DataTypeRate12)
			if err != nil {
				t.Fatal(err)
			}
			var rx packet.Assembler
			if _, err := add(&rx, encodeHeader(t, &p.Header)); err != nil {
				t.Fatal(err)
			}
			var last error
			for i, b := range p.Blocks {
				if i == 2 {
					b = corrupt(t, p, i)
				} else {
					b = encodeBlock(t, b)
				}
				_, last = add(&rx, b)
			}
			if !errors.Is(last, packet.ErrBlocksMissing) || !slices.Equal(rx.Missing(), []int{2}) {
				t.Fatalf("first try: %v, missing %v", last, rx.Missing())
			}
			m, err := feed(t, &rx, laterTry(t, p, []int{2}))
			if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
				t.Fatalf("later try: %v %+v", err, m)
			}
		})
	}
}
