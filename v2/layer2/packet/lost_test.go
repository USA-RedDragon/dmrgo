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

// lost returns the Message of a.Lost, or nil when the packet did not
// complete.
func lost(a *packet.Assembler) (*packet.Message, error) {
	m, done, err := a.Lost()
	if !done {
		return nil, err
	}
	return &m, err
}

// sendLosing feeds the header and blocks of p to a, and calls Lost in place
// of each block whose index is in drop. It returns the result of the last
// block.
func sendLosing(t *testing.T, a *packet.Assembler, p packet.Packet, drop ...int) (*packet.Message, error) {
	t.Helper()
	if m, err := add(a, encodeHeader(t, &p.Header)); m != nil || err != nil {
		t.Fatalf("header: %v %v", m, err)
	}
	var m *packet.Message
	var err error
	for i, b := range p.Blocks {
		if slices.Contains(drop, i) {
			m, err = lost(a)
		} else {
			m, err = add(a, encodeBlock(t, b))
		}
		if (m != nil || err != nil) && i != len(p.Blocks)-1 {
			t.Fatalf("block %d: %v %v", i, m, err)
		}
	}
	return m, err
}

// TestAssembler_LostConfirmedBlock loses one block burst of a confirmed
// packet. Without Lost the Assembler would wait for the block forever and
// offer no SACK.
func TestAssembler_LostConfirmedBlock(t *testing.T) {
	t.Parallel()

	data := payload(60)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blocks) != 7 {
		t.Fatalf("%d blocks, want 7", len(p.Blocks))
	}
	rx := packet.NewAssembler(127)
	if m, err := sendLosing(t, rx, p, 2); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("last block: %v %v, want ErrBlocksMissing", m, err)
	}
	if got := rx.Missing(); !slices.Equal(got, []int{2}) {
		t.Fatalf("Missing = %v, want [2]", got)
	}
	resp, err := rx.Response()
	if err != nil {
		t.Fatal(err)
	}
	tx := packet.NewAssembler(127)
	sack, err := feed(t, tx, resp)
	if err != nil || sack.Response == nil || !slices.Equal(sack.Response.RetryBlocks(), []int{2}) {
		t.Fatalf("SACK %v %+v", err, sack)
	}

	h := p.Header
	c := *h.ConfirmedDataHeader
	c.BlocksToFollow = 1
	h.ConfirmedDataHeader = &c
	if m, err := add(rx, encodeHeader(t, &h)); m != nil || err != nil {
		t.Fatalf("retry header: %v %v", m, err)
	}
	m, err := add(rx, encodeBlock(t, p.Blocks[2]))
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after retry: %v %+v", err, m)
	}
}

// TestAssembler_LostUnconfirmedBlock loses a middle block and the last block
// of an unconfirmed packet. Each attempt ends at its last block with
// ErrMessageCRC.
func TestAssembler_LostUnconfirmedBlock(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(unconfirmed(), payload(40), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range []int{1, len(p.Blocks) - 1} {
		a := packet.NewAssembler(127)
		m, err := sendLosing(t, a, p, drop)
		if m == nil || !errors.Is(err, packet.ErrMessageCRC) {
			t.Fatalf("lose %d: %v %v, want the Message with ErrMessageCRC", drop, m, err)
		}
		if a.InProgress() {
			t.Fatalf("lose %d: still in progress", drop)
		}
	}
}

func TestAssembler_LostWithoutPacket(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	if m, err := lost(a); m != nil || err != nil {
		t.Fatalf("idle: %v %v", m, err)
	}
	p, err := packet.NewPacket(confirmed(), payload(30), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	b := encodeBlock(t, p.Blocks[0])
	r, _ := b.(*pdu.Rate12Data)
	r.Data[5] ^= 0x01
	if m, err := sendLosing(t, a, packet.Packet{Header: p.Header, Blocks: append([]layer2.Data{b}, p.Blocks[1:]...)}); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("bad block: %v %v", m, err)
	}
	if m, err := lost(a); m != nil || err != nil || !a.InProgress() {
		t.Fatalf("waiting for the retry: %v %v", m, err)
	}
}
