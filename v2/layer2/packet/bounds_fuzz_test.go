package packet_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	maxBoundBlocks = 127
	maxBoundData   = 3048
	maxRetryMap    = (len(packet.ResponseData{}.Retry) + 7) / 8
)

func runPacketBounds(t *testing.T, data []byte, rateSel byte, conf bool, retry []byte) {
	t.Helper()
	rate := []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1}[rateSel%3]
	h := unconfirmed()
	if conf {
		h = confirmed()
	}
	b := packet.NewBuilder(maxBoundBlocks)
	p, err := b.Segment(h, data, rate)
	if len(p.Blocks) > maxBoundBlocks {
		t.Fatalf("Segment returned %d blocks", len(p.Blocks))
	}
	if err != nil {
		return
	}
	var rd packet.ResponseData
	retry = retry[:min(len(retry), maxRetryMap)]
	for i := range rd.Retry {
		rd.Retry[i] = i/8 < len(retry) && retry[i/8]>>(i%8)&1 != 0
	}
	q, _ := b.Retry(&p, &rd)
	if len(q.Blocks) > maxBoundBlocks {
		t.Fatalf("Retry returned %d blocks", len(q.Blocks))
	}
	a := packet.NewAssembler(maxBoundBlocks)
	feed := func(blocks []pdu.Rate12Data) {
		for i := range blocks {
			m, _, _ := a.Add(&blocks[i])
			if len(m.Data) > maxBoundData {
				t.Fatalf("Add returned %d bytes of data", len(m.Data))
			}
		}
	}
	m, _, _ := a.Add(&p.Header)
	if len(m.Data) > maxBoundData {
		t.Fatalf("Add returned %d bytes of data", len(m.Data))
	}
	for i, blk := range p.Blocks {
		if i%5 == int(rateSel)%5 && conf {
			continue
		}
		m, _, _ := a.Add(blk)
		if len(m.Data) > maxBoundData {
			t.Fatalf("Add returned %d bytes of data", len(m.Data))
		}
	}
	if n := len(a.Missing()); n > maxBoundBlocks {
		t.Fatalf("Missing returned %d block numbers", n)
	}
	if r, err := a.Response(); err == nil && len(r.Blocks) > maxBoundBlocks {
		t.Fatalf("Response returned %d blocks", len(r.Blocks))
	}
	if conf {
		to, _ := p.Header.AsConfirmed()
		ack, _ := b.NewResponse(&to, packet.ResponseSACK, &rd)
		if len(ack.Blocks) > maxBoundBlocks {
			t.Fatalf("NewResponse returned %d blocks", len(ack.Blocks))
		}
		enc, err := rd.AppendEncode(nil, len(p.Blocks))
		if err == nil {
			feed(enc)
		}
	}
	m, _, _ = a.Lost()
	if len(m.Data) > maxBoundData {
		t.Fatalf("Lost returned %d bytes of data", len(m.Data))
	}
}

func TestPacketBounds(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 11, 100, 1000, 3000, 4000} {
		for r := range byte(3) {
			runPacketBounds(t, payload(n), r, false, nil)
			runPacketBounds(t, payload(n), r, true, []byte{0x0A})
		}
	}
}

func FuzzPacketBounds(f *testing.F) {
	f.Add(uint16(40), payload(40), byte(0), byte(0), true, []byte{0x02})
	f.Add(uint16(3000), payload(maxPattern), byte(7*maxPattern%256), byte(2), false, []byte{})
	buf := make([]byte, maxPayload)
	f.Fuzz(func(t *testing.T, size uint16, pattern []byte, step, rateSel byte, conf bool, retry []byte) {
		runPacketBounds(t, fillPattern(buf[:int(size)%maxPayload], pattern[:min(len(pattern), maxPattern)], step), rateSel, conf, retry)
	})
}

func fillPattern(data, pattern []byte, step byte) []byte {
	clear(data)
	for i := 0; len(pattern) > 0 && i < len(data); i += len(pattern) {
		n := copy(data[i:], pattern)
		for j := range n {
			data[i+j] += byte(i/len(pattern)) * step
		}
	}
	return data
}
