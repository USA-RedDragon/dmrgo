package packet_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func loadRaw(t *testing.T, path string) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || len(data)%33 != 0 {
		t.Fatalf("read %s: %v", path, err)
	}
	out := make([]layer2.RawBurst, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		out = append(out, layer2.RawBurst(data[i:]))
	}
	return out
}

// deepCopy returns h with each sub-header copied.
func deepCopy(h pdu.DataHeader) pdu.DataHeader {
	v := reflect.ValueOf(&h).Elem()
	for i := range v.NumField() {
		if f := v.Field(i); f.Kind() == reflect.Pointer && !f.IsNil() {
			c := reflect.New(f.Type().Elem())
			c.Elem().Set(f.Elem())
			f.Set(c)
		}
	}
	return h
}

// TestAssembler_ReusedBurst decodes each capture twice into one Burst and
// checks that the first Message keeps its data and header when the
// Assembler does not reuse its storage.
func TestAssembler_ReusedBurst(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"d-sms", "m-sms", "h-sms"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			raws := loadRaw(t, "../testdata/"+file+".bin")
			raws = append(raws, raws...)
			for _, reuse := range []bool{false, true} {
				a := packet.Assembler{Reuse: reuse}
				var burst layer2.Burst
				var msgs []packet.Message
				var copies []packet.Message
				for _, raw := range raws {
					if err := burst.DecodeFromBytes(raw); err != nil {
						t.Fatal(err)
					}
					m, done, err := a.AddBurst(&burst)
					if err != nil {
						t.Fatal(err)
					}
					if done {
						msgs = append(msgs, m)
						copies = append(copies, packet.Message{Data: bytes.Clone(m.Data), Header: deepCopy(m.Header)})
					}
				}
				if len(msgs) != 2 || !bytes.Contains(copies[0].Data, utf16le("TEST KI5VMF")) || !bytes.Equal(copies[0].Data, copies[1].Data) {
					t.Fatalf("reuse %v: %d messages", reuse, len(msgs))
				}
				if reuse {
					continue
				}
				if !bytes.Equal(msgs[0].Data, copies[0].Data) || !reflect.DeepEqual(deepCopy(msgs[0].Header), copies[0].Header) {
					t.Errorf("first Message changed after the second packet")
				}
			}
		})
	}
}

// TestAssembler_HeaderInPlace adds headers whose sub-header the caller then
// changes in place, as a reused decoder does.
func TestAssembler_HeaderInPlace(t *testing.T) {
	t.Parallel()

	for _, reuse := range []bool{false, true} {
		p, err := packet.Segment(confirmed(), payload(40), enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		a := packet.Assembler{Reuse: reuse}
		h := p.Header
		c := *h.ConfirmedDataHeader
		h.ConfirmedDataHeader = &c
		if _, _, err := a.Add(&h); err != nil {
			t.Fatal(err)
		}
		first, ok := p.Blocks[0].(*pdu.Rate12Data)
		if !ok {
			t.Fatalf("block 0 is %T", p.Blocks[0])
		}
		bad := *first
		bad.Data[5] ^= 1
		var last error
		for _, b := range append([]layer2.Data{&bad}, p.Blocks[1:]...) {
			_, _, last = a.Add(b)
		}
		if !errors.Is(last, packet.ErrBlocksMissing) {
			t.Fatalf("reuse %v: %v", reuse, last)
		}
		c.LLIDSource++
		c.BlocksToFollow = 2
		if _, _, err := a.Add(&h); err != nil {
			t.Fatal(err)
		}
		if got := a.Missing(); len(got) != 2 {
			t.Errorf("reuse %v: header of another source taken as a retry, missing %v", reuse, got)
		}
		resp, err := a.Response()
		if err != nil || resp.Header.ResponsePacketHeader.LLIDDestination != c.LLIDSource {
			t.Errorf("reuse %v: response %v %+v", reuse, err, resp.Header.ResponsePacketHeader)
		}
	}
}

func TestAssembler_ReuseRetry(t *testing.T) {
	t.Parallel()

	data := []byte("LIP: 41.878,-87.629,hdg=90,v=12 and some more text")
	p, err := packet.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.Assembler{Reuse: true}
	tx := packet.Assembler{Reuse: true}
	for range 2 {
		if _, _, err := rx.Add(encodeHeader(t, &p.Header)); err != nil {
			t.Fatal(err)
		}
		var last error
		for i, b := range p.Blocks {
			blk := encodeBlock(t, b)
			if i == 1 || i == 3 {
				r, _ := blk.(*pdu.Rate34Data)
				r.Data[7] ^= 0x10
			}
			_, _, last = rx.Add(blk)
		}
		if !errors.Is(last, packet.ErrBlocksMissing) {
			t.Fatal(last)
		}
		missing := rx.AppendMissing([]int{-1})
		if len(missing) != 3 || missing[0] != -1 || missing[1] != 1 || missing[2] != 3 {
			t.Fatalf("missing %v", missing)
		}
		resp, err := rx.Response()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := tx.Add(encodeHeader(t, &resp.Header)); err != nil {
			t.Fatal(err)
		}
		sack, done, err := tx.Add(encodeBlock(t, resp.Blocks[0]))
		if err != nil || !done || sack.Response == nil {
			t.Fatalf("SACK: %v %v", done, err)
		}
		retry := sack.Response.AppendRetryBlocks(nil)
		retryAndACK(t, &rx, &tx, p, data, retry)
	}
}

func TestBuilder(t *testing.T) {
	t.Parallel()

	var b packet.Builder
	for range 2 {
		for _, rate := range []enums.DataType{enums.DataTypeRate1, enums.DataTypeRate12, enums.DataTypeRate34} {
			for _, h := range []pdu.DataHeader{unconfirmed(), confirmed()} {
				for _, n := range []int{0, 30, 300} {
					want, err := packet.Segment(h, payload(n), rate)
					if err != nil {
						t.Fatal(err)
					}
					got, err := b.Segment(h, payload(n), rate)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("%s %s n=%d: Builder.Segment differs: %v", h.Format.String(), rate.String(), n, err)
					}
				}
			}
		}
	}
	if _, err := b.Segment(unconfirmed(), nil, enums.DataTypeCSBK); !errors.Is(err, packet.ErrRate) {
		t.Errorf("rate: %v", err)
	}

	to := confirmed().ConfirmedDataHeader
	to.BlocksToFollow = 90
	var rd packet.ResponseData
	rd.Retry[3], rd.Retry[80] = true, true
	for _, r := range []packet.Response{packet.ResponseSACK, packet.ResponseACK, packet.ResponseSACK, packet.ResponseNACKFSNOutOfSequence} {
		want, err := packet.NewResponse(to, r, &rd)
		if err != nil {
			t.Fatal(err)
		}
		got, err := b.NewResponse(to, r, &rd)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: Builder.NewResponse differs: %v", r, err)
		}
	}
	to.BlocksToFollow = 200
	if _, err := b.NewResponse(to, packet.ResponseSACK, nil); !errors.Is(err, packet.ErrTooLong) {
		t.Errorf("200 blocks: %v", err)
	}
}

func TestAppendBursts(t *testing.T) {
	t.Parallel()

	p, err := packet.Segment(confirmed(), payload(50), enums.DataTypeRate1)
	if err != nil {
		t.Fatal(err)
	}
	want, err := packet.Bursts(p, 3, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	prefix := layer2.RawBurst{1}
	got, err := packet.AppendBursts([]layer2.RawBurst{prefix}, &p, 3, layer2.SyncSourceMS)
	if err != nil || len(got) != 1+len(want) || got[0] != prefix || !reflect.DeepEqual(got[1:], want) {
		t.Fatalf("AppendBursts: %v", err)
	}
	p.Blocks = append(p.Blocks, &pdu.CSBK{})
	got, err = packet.AppendBursts(got[:1], &p, 3, layer2.SyncSourceMS)
	if !errors.Is(err, packet.ErrRate) || len(got) != 1 {
		t.Errorf("CSBK block: %v, %d bursts", err, len(got))
	}
}

func TestResponseData_Append(t *testing.T) {
	t.Parallel()

	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[100] = true, true
	want, err := rd.Encode(127)
	if err != nil {
		t.Fatal(err)
	}
	prefix := pdu.Rate12Data{Data: [12]byte{9}}
	got, err := rd.AppendEncode([]pdu.Rate12Data{prefix}, 127)
	if err != nil || len(got) != 3 || got[0] != prefix || got[1] != *want[0] || got[2] != *want[1] {
		t.Fatalf("AppendEncode: %v %v", err, got)
	}
	if got, err := rd.AppendEncode(got[:1], -1); err == nil || len(got) != 1 {
		t.Errorf("-1 blocks: %v", err)
	}
	if got := rd.AppendRetryBlocks([]int{7}); len(got) != 3 || got[0] != 7 || got[1] != 2 || got[2] != 100 {
		t.Errorf("AppendRetryBlocks: %v", got)
	}

	bad := *want[0]
	bad.Data[0] ^= 0x80
	_, err = packet.DecodeResponseData(&bad, want[1])
	if err == nil || err.Error() != "invalid response data blocks: fec: uncorrectable" {
		t.Errorf("bad CRC: %v", err)
	}
	var a packet.Assembler
	if _, err := a.Response(); err == nil || err.Error() != "header format does not carry segmented data: no confirmed packet in progress" {
		t.Errorf("Response: %v", err)
	}
}

// TestAssembler_ResponseLength sends a C_RHEAD with three blocks, which is
// not a valid C_RDATA length.
func TestAssembler_ResponseLength(t *testing.T) {
	t.Parallel()

	h := pdu.DataHeader{Format: pdu.FormatResponsePacket, ResponsePacketHeader: &pdu.ResponsePacketHeader{BlocksToFollow: 3, ResponseClass: 2}}
	var a packet.Assembler
	if _, _, err := a.Add(encodeHeader(t, &h)); err != nil {
		t.Fatal(err)
	}
	var err error
	var done bool
	for range 3 {
		_, done, err = a.Add(&pdu.Rate12Data{})
	}
	if !done || !errors.Is(err, packet.ErrResponseData) {
		t.Errorf("done %v err %v", done, err)
	}
}
