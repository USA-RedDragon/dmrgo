package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func txPool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	var p []layer2.RawBurst
	var rd packet.ResponseData
	rd.Retry[0], rd.Retry[3] = true, true
	for _, r := range []packet.Response{packet.ResponseACK, packet.ResponseSACK, packet.ResponseNACKPacketCRC,
		packet.ResponseNACKOutOfSequence, packet.ResponseNACKMemoryFull} {
		for ns := range uint8(8) {
			ch := confirmedHeader(t, ns)
			h := confirmedOf(t, &ch)
			h.BlocksToFollow = 7
			resp, err := packet.NewResponse(&h, r, &rd)
			if err != nil {
				t.Fatal(err)
			}
			b, err := packet.Bursts(&resp, 0, layer2.SyncSourceBS)
			if err != nil {
				t.Fatal(err)
			}
			p = append(p, b...)
		}
	}
	return p
}

func txMessages(t testing.TB) []*data.Message {
	t.Helper()
	u := &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		LLIDDestination: dstID, LLIDSource: srcID}), Rate: enums.DataTypeRate1, Payload: payload(30)}
	dd := &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
		LLIDDestination: dstID, LLIDSource: srcID, SARQ: true}), Rate: enums.DataTypeRate12, Payload: payload(20)}
	return []*data.Message{message(t, 60), message(t, 10), u, dd, message(t, 500)}
}

func checkTxScript(t *testing.T, in []byte, p []layer2.RawBurst, msgs []*data.Message) {
	t.Helper()
	get := func(i int) byte {
		if i < len(in) {
			return in[i]
		}
		return 0
	}
	c0, c1, c2 := get(0), get(1), get(2)
	s, err := data.NewSender(&data.SenderConfig{
		Window: int(c2 % 10),
		Access: enums.Access(c0 & 3), Preambles: int(c0 >> 2 & 7), Retries: int(c1 & 7),
		Resync: c0&0x20 != 0, Seed: uint64(c1), Holdoff: time.Duration(c1>>3) * 50 * time.Millisecond,
		IdleSearch: time.Duration(int8(c0&0xC0)) * time.Millisecond, MaxBlocks: int(c1 >> 6 * 16),
	})
	if err != nil {
		return
	}
	out := make([]data.Output, 0, data.MaxOutputs)
	var now time.Duration
	active := 0
	for i := 3; i+2 < len(in) && i < 3+3*512; i += 3 {
		k, step, arg := in[i]%6, in[i+1], in[i+2]
		now += time.Duration(step) * time.Millisecond
		out = out[:0]
		switch k {
		case 0:
			if s.Send(now, msgs[int(arg)%len(msgs)]) == nil {
				active++
			}
		case 1:
			if at, ok := s.Deadline(); ok && at > now {
				now = at
			}
			out = s.Tick(now, out)
		case 2:
			s.Channel(now, arg&1 != 0)
		case 3:
			raw := p[int(arg)%len(p)]
			if arg&0x80 != 0 {
				raw[int(arg)%33] ^= 0x10
			}
			out = s.Burst(now, &raw, layer2.Hint{}, out)
		case 4:
			out = s.Cancel(now, out)
		case 5:
			s.Reset()
			active = 0
		}
		if len(out) > data.MaxOutputs {
			t.Fatalf("op %d: %d outputs", i, len(out))
		}
		for _, o := range out {
			if o.Kind != data.OutputDone {
				continue
			}
			if active == 0 {
				t.Fatalf("op %d: a message ended twice", i)
			}
			active--
		}
		if (active == 0) != (s.State() == data.SenderIdle) {
			t.Fatalf("op %d: state %v with %d messages", i, s.State(), active)
		}
	}
}

func txSeeds() [][]byte {
	seeds := make([][]byte, 0, 3)
	for _, cfg := range [][3]byte{{0, 0, 0}, {0x2A, 0x13, 3}, {0xC2, 0xFF, 8}} {
		s := make([]byte, 0, 3+3*40)
		s = append(s, cfg[0], cfg[1], cfg[2])
		for i := range 40 {
			switch {
			case i%10 == 0:
				s = append(s, 0, 1, byte(i/10))
			case i%7 == 3:
				s = append(s, 3, 20, byte(i))
			case i%11 == 5:
				s = append(s, 2, 5, byte(i))
			default:
				s = append(s, 1, 60, 0)
			}
		}
		seeds = append(seeds, s)
	}
	return seeds
}

func FuzzSender(f *testing.F) {
	p, msgs := txPool(f), txMessages(f)
	for _, s := range txSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		checkTxScript(t, in, p, msgs)
	})
}
