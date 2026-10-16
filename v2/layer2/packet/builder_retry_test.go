package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TestBuilder_RetrySACK sends a confirmed packet whose blocks 1 and 3 fail
// their CRC-9, decodes the SACK, and resends the blocks it flags with
// Builder.Retry.
func TestBuilder_RetrySACK(t *testing.T) {
	t.Parallel()

	data := payload(70)
	var tx packet.Builder
	p, err := tx.Segment(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rx packet.Assembler
	if _, err := add(&rx, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	var last error
	for i := range p.Blocks {
		b := encodeBlock(t, p.Blocks[i])
		if i == 1 || i == 3 {
			b = corrupt(t, p, i)
		}
		_, last = add(&rx, b)
	}
	if !errors.Is(last, packet.ErrBlocksMissing) {
		t.Fatalf("first try: %v", last)
	}
	resp, err := rx.Response()
	if err != nil {
		t.Fatal(err)
	}
	var back packet.Assembler
	sack, err := feed(t, &back, resp)
	if err != nil {
		t.Fatal(err)
	}

	r, err := tx.Retry(&p, sack.Response)
	if err != nil {
		t.Fatal(err)
	}
	c := r.Header.ConfirmedDataHeader
	if c.FullMessageFlag || c.BlocksToFollow != 2 || c.SendSequenceNumber != 5 || len(r.Blocks) != 2 {
		t.Fatalf("retry header %+v with %d blocks", c, len(r.Blocks))
	}
	if !p.Header.ConfirmedDataHeader.FullMessageFlag || int(p.Header.ConfirmedDataHeader.BlocksToFollow) != len(p.Blocks) {
		t.Fatal("Retry changed the first try")
	}
	for i, want := range []uint8{1, 3} {
		if blk, ok := r.Blocks[i].(*pdu.Rate12Data); !ok || blk.ConfirmedDBSN() != want {
			t.Errorf("retry block %d: %v, want DBSN %d", i, r.Blocks[i], want)
		}
	}
	m, err := feed(t, &rx, r)
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after retry: %v %+v", err, m)
	}
}

// TestBuilder_RetryAll resends a whole packet, as after a NACK, to an
// Assembler that lost the first header.
func TestBuilder_RetryAll(t *testing.T) {
	t.Parallel()

	data := payload(48)
	var tx packet.Builder
	p, err := tx.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	r, err := tx.Retry(&p, nil)
	if err != nil || len(r.Blocks) != len(p.Blocks) || r.Header.ConfirmedDataHeader.FullMessageFlag {
		t.Fatalf("retry all: %v, %d blocks", err, len(r.Blocks))
	}
	var rx packet.Assembler
	m, err := feed(t, &rx, r)
	if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
		t.Fatalf("got %v %+v", err, m)
	}
}

func TestBuilder_RetryShortData(t *testing.T) {
	t.Parallel()

	for _, h := range []pdu.DataHeader{
		{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, SARQ: true, FullMessageFlag: true}},
		{Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, SARQ: true, FullMessageFlag: true}},
	} {
		var tx packet.Builder
		p, err := tx.Segment(h, payload(30), enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		var rd packet.ResponseData
		rd.Retry[0], rd.Retry[2] = true, true
		r, err := tx.Retry(&p, &rd)
		if err != nil || r.Header.AppendedBlocks != 2 || len(r.Blocks) != 2 {
			t.Fatalf("%v: %v, AB %d, %d blocks", h.Format, err, r.Header.AppendedBlocks, len(r.Blocks))
		}
		enc := encodeHeader(t, &r.Header)
		if enc.AppendedBlocks != 2 || enc.Format != h.Format {
			t.Errorf("%v: encoded AB %d", h.Format, enc.AppendedBlocks)
		}
	}
}

func TestBuilder_RetryErrors(t *testing.T) {
	t.Parallel()

	var b packet.Builder
	u, err := packet.Segment(unconfirmed(), payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Retry(&u, nil); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("unconfirmed: %v, want ErrFormat", err)
	}
	h := pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{SAP: enums.SAPShortData, FullMessageFlag: true}}
	d, err := packet.Segment(h, payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Retry(&d, nil); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("DD_HEAD without SARQ: %v, want ErrFormat", err)
	}
	c, err := packet.Segment(confirmed(), payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rd packet.ResponseData
	rd.Retry[100] = true
	if _, err := b.Retry(&c, &rd); !errors.Is(err, packet.ErrNoRetryBlocks) {
		t.Errorf("no flagged block: %v, want ErrNoRetryBlocks", err)
	}
}
