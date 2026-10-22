package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBuilder_RetrySACK(t *testing.T) {
	t.Parallel()

	data := payload(70)
	tx := packet.NewBuilder(127)
	p, err := tx.Segment(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	if _, err := add(rx, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	var last error
	for i := range p.Blocks {
		b := encodeBlock(t, p.Blocks[i])
		if i == 1 || i == 3 {
			b = corrupt(t, p, i)
		}
		_, last = add(rx, b)
	}
	if !errors.Is(last, packet.ErrBlocksMissing) {
		t.Fatalf("first try: %v", last)
	}
	resp, err := rx.Response()
	if err != nil {
		t.Fatal(err)
	}
	back := packet.NewAssembler(127)
	sack, err := feed(t, back, resp)
	if err != nil {
		t.Fatal(err)
	}

	r, err := tx.Retry(&p, sack.Response)
	if err != nil {
		t.Fatal(err)
	}
	c := get(t, &r.Header, (*pdu.DataHeader).AsConfirmed)
	if c.FullMessageFlag || c.BlocksToFollow != 2 || c.SendSequenceNumber != 5 || len(r.Blocks) != 2 {
		t.Fatalf("retry header %+v with %d blocks", c, len(r.Blocks))
	}
	if first := get(t, &p.Header, (*pdu.DataHeader).AsConfirmed); !first.FullMessageFlag || int(first.BlocksToFollow) != len(p.Blocks) {
		t.Fatal("Retry changed the first try")
	}
	for i, want := range []uint8{1, 3} {
		if blk, ok := r.Blocks[i].(*pdu.Rate12Data); !ok || blk.ConfirmedDBSN() != want {
			t.Errorf("retry block %d: %v, want DBSN %d", i, r.Blocks[i], want)
		}
	}
	m, err := feed(t, rx, r)
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after retry: %v %+v", err, m)
	}
}

func TestBuilder_RetryAll(t *testing.T) {
	t.Parallel()

	data := payload(48)
	tx := packet.NewBuilder(127)
	p, err := tx.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	r, err := tx.Retry(&p, nil)
	if err != nil || len(r.Blocks) != len(p.Blocks) || get(t, &r.Header, (*pdu.DataHeader).AsConfirmed).FullMessageFlag {
		t.Fatalf("retry all: %v, %d blocks", err, len(r.Blocks))
	}
	rx := packet.NewAssembler(127)
	m, err := feed(t, rx, r)
	if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
		t.Fatalf("got %v %+v", err, m)
	}
}

func TestBuilder_RetryShortData(t *testing.T) {
	t.Parallel()

	for _, h := range []pdu.DataHeader{
		header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, SARQ: true, FullMessageFlag: true}),
		header(t, (*pdu.DataHeader).SetRawData, pdu.RawDataHeader{SAP: enums.SAPShortData, AppendedBlocks: 1, LLIDDestination: 1, LLIDSource: 2, SARQ: true, FullMessageFlag: true}),
	} {
		tx := packet.NewBuilder(127)
		p, err := tx.Segment(h, payload(30), enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		var rd packet.ResponseData
		rd.Retry[0], rd.Retry[2] = true, true
		r, err := tx.Retry(&p, &rd)
		ab, _ := r.Header.BlocksToFollow()
		if err != nil || ab != 2 || len(r.Blocks) != 2 {
			t.Fatalf("%v: %v, AB %d, %d blocks", h.Format(), err, ab, len(r.Blocks))
		}
		enc := encodeHeader(t, &r.Header)
		if ab, _ := enc.BlocksToFollow(); ab != 2 || enc.Format() != h.Format() || fullFlag(enc) {
			t.Errorf("%v: encoded AB %d", h.Format(), ab)
		}
	}
}

func TestBuilder_RetryWholeMessage(t *testing.T) {
	t.Parallel()

	for _, h := range []pdu.DataHeader{
		header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, ResponseRequested: true, FullMessageFlag: true}),
		header(t, (*pdu.DataHeader).SetRawData, pdu.RawDataHeader{SAP: enums.SAPShortData, AppendedBlocks: 1, LLIDDestination: 1, LLIDSource: 2, ResponseRequested: true, FullMessageFlag: true}),
	} {
		data := payload(30)
		tx := packet.NewBuilder(127)
		p, err := tx.Segment(h, data, enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		r, err := tx.Retry(&p, nil)
		if err != nil {
			t.Fatalf("%v: %v", h.Format(), err)
		}
		if r.Header != p.Header || len(r.Blocks) != len(p.Blocks) {
			t.Fatalf("%v: header %+v with %d blocks, want the first try", h.Format(), r.Header, len(r.Blocks))
		}
		for i := range p.Blocks {
			if r.Blocks[i] != p.Blocks[i] {
				t.Errorf("%v: block %d is not block %d of the first try", h.Format(), i, i)
			}
		}
		m, err := feed(t, packet.NewAssembler(127), r)
		if err != nil || m == nil || !bytes.Equal(m.Data[:len(data)], data) {
			t.Fatalf("%v: got %v %+v", h.Format(), err, m)
		}
		var rd packet.ResponseData
		rd.Retry[0] = true
		if _, err := tx.Retry(&p, &rd); !errors.Is(err, packet.ErrFormat) {
			t.Errorf("%v: selective retry without SARQ: %v, want ErrFormat", h.Format(), err)
		}
	}
}

func TestBuilder_RetryErrors(t *testing.T) {
	t.Parallel()

	b := packet.NewBuilder(127)
	u, err := packet.NewPacket(unconfirmed(), payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Retry(&u, nil); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("unconfirmed: %v, want ErrFormat", err)
	}
	h := header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{SAP: enums.SAPShortData, FullMessageFlag: true})
	d, err := packet.NewPacket(h, payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Retry(&d, nil); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("DD_HEAD without SARQ: %v, want ErrFormat", err)
	}
	c, err := packet.NewPacket(confirmed(), payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rd packet.ResponseData
	rd.Retry[100] = true
	if _, err := b.Retry(&c, &rd); !errors.Is(err, packet.ErrNoRetryBlocks) {
		t.Errorf("no flagged block: %v, want ErrNoRetryBlocks", err)
	}
}
