package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestAssembler_AcceptBadCRCCapture(t *testing.T) {
	t.Parallel()

	for _, accept := range []bool{false, true} {
		bursts := loadBursts(t, "../testdata/m-sms.bin")
		blk, ok := bursts[8].Data.(*pdu.Rate12Data)
		if !ok {
			t.Fatal("burst 8 is not rate 1/2")
		}
		blk.Data[3] ^= 0x01
		a := packet.NewAssembler(127)
		a.AcceptBadCRC = accept
		var msg *packet.Message
		var got error
		for _, b := range bursts {
			if m, err := addBurst(a, b); m != nil {
				msg, got = m, err
			}
		}
		if msg == nil || !msg.BadCRC || len(msg.Data) != 62 {
			t.Fatalf("accept %v: message %+v", accept, msg)
		}
		if accept && got != nil || !accept && !errors.Is(got, packet.ErrMessageCRC) {
			t.Errorf("accept %v: error %v", accept, got)
		}
	}
}

func TestAssembler_AcceptBadCRCConfirmed(t *testing.T) {
	t.Parallel()

	data := payload(60)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	send := func(a *packet.Assembler, lose int) (*packet.Message, error) {
		if _, err := add(a, encodeHeader(t, &p.Header)); err != nil {
			t.Fatal(err)
		}
		var m *packet.Message
		var err error
		for i := range p.Blocks {
			var b layer2.Data
			switch i {
			case lose:
				m, err = lost(a)
				continue
			case 2:
				b = corrupt(t, p, i)
			default:
				b = encodeBlock(t, p.Blocks[i])
			}
			m, err = add(a, b)
		}
		return m, err
	}

	a := packet.NewAssembler(127)
	a.AcceptBadCRC = true
	m, err := send(a, -1)
	if err != nil || m == nil || !m.BadCRC {
		t.Fatalf("bad CRC-9: %v %+v", err, m)
	}
	want := bytes.Clone(data)
	want[2*10+2] ^= 0x08
	if !bytes.Equal(m.Data, want) {
		t.Fatalf("data %x, want %x", m.Data, want)
	}

	a.Reset()
	if m, err := send(a, 4); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("lost block: %v %v, want ErrBlocksMissing", m, err)
	}
	r := laterTry(t, p, []int{2, 4})
	m, err = feed(t, a, r)
	if err != nil || m == nil || m.BadCRC || !bytes.Equal(m.Data, data) {
		t.Fatalf("later try with good copies: %v %+v", err, m)
	}

	strict := packet.NewAssembler(127)
	if m, err := send(strict, -1); m != nil || !errors.Is(err, packet.ErrBlocksMissing) {
		t.Fatalf("without AcceptBadCRC: %v %v, want ErrBlocksMissing", m, err)
	}
}

func TestAssembler_AcceptBadCRCHeader(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	a.AcceptBadCRC = true
	if _, err := addBurst(a, badHeader(t)); !errors.Is(err, packet.ErrHeaderCRC) {
		t.Fatalf("bad header: %v, want ErrHeaderCRC", err)
	}
	p, err := packet.NewPacket(confirmed(), payload(20), enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	m, err := feed(t, a, p)
	if err != nil || m == nil || m.BadCRC {
		t.Fatalf("good packet: %v %+v", err, m)
	}
}

func TestAssembler_AllBlocksLost(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(unconfirmed(), payload(10), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	all := make([]int, len(p.Blocks))
	for i := range all {
		all[i] = i
	}
	for _, accept := range []bool{false, true} {
		a := packet.NewAssembler(127)
		a.AcceptBadCRC = accept
		m, err := sendLosing(t, a, p, all...)
		if m == nil || !m.BadCRC || !errors.Is(err, packet.ErrMessageCRC) {
			t.Fatalf("AcceptBadCRC %v: got %v %+v", accept, err, m)
		}
	}
}
