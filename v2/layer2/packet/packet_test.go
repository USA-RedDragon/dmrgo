package packet_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func loadBursts(t *testing.T, path string) []*layer2.Burst {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || len(data)%33 != 0 {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []*layer2.Burst
	for i := 0; i < len(data); i += 33 {
		b, err := layer2.NewBurstFromBytes([33]byte(data[i:]))
		if err != nil {
			t.Fatalf("burst %d: %v", i/33, err)
		}
		out = append(out, b)
	}
	return out
}

func utf16le(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		out = append(out, byte(r), 0)
	}
	return out
}

func TestAssembler_Captures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file   string
		format pdu.Format
		size   int
		want   []byte
	}{
		{"d-sms", pdu.FormatUnconfirmed, 56, utf16le("TEST KI5VMF")},
		{"m-sms", pdu.FormatUnconfirmed, 62, utf16le("TEST KI5VMF")},
		{"h-sms", pdu.FormatShortDataDefined, 32, utf16le("TEST KI5VMF")},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			a := packet.NewAssembler(127)
			var msgs []*packet.Message
			for _, b := range loadBursts(t, "../testdata/"+tt.file+".bin") {
				m, err := addBurst(a, b)
				if err != nil {
					t.Fatalf("AddBurst: %v", err)
				}
				if m != nil {
					msgs = append(msgs, m)
				}
			}
			if len(msgs) != 1 {
				t.Fatalf("got %d messages", len(msgs))
			}
			m := msgs[0]
			if m.Header.Format != tt.format || m.DataType != enums.DataTypeRate12 || len(m.Data) != tt.size {
				t.Fatalf("format %d type %d len %d", m.Header.Format, m.DataType, len(m.Data))
			}
			if !bytes.Contains(m.Data, tt.want) {
				t.Errorf("data %x does not hold the text", m.Data)
			}
		})
	}
}

func TestAssembler_CaptureCRCFailure(t *testing.T) {
	t.Parallel()

	bursts := loadBursts(t, "../testdata/m-sms.bin")
	blk, ok := bursts[8].Data.(*pdu.Rate12Data)
	if !ok {
		t.Fatal("burst 8 is not rate 1/2")
	}
	blk.Data[3] ^= 0x01
	a := packet.NewAssembler(127)
	var got error
	var msg *packet.Message
	for _, b := range bursts {
		m, err := addBurst(a, b)
		if m != nil {
			msg, got = m, err
		}
	}
	if !errors.Is(got, packet.ErrMessageCRC) || !errors.Is(got, fec.ErrUncorrectable) || msg == nil || msg.CRC != 0x0b8c211b {
		t.Fatalf("err %v msg %+v", got, msg)
	}
}

// badHeader returns the U_HEAD of d-sms.bin (burst 5) with 24 bits flipped.
// It decodes as a C_HEAD whose CRC fails.
func badHeader(t *testing.T) *layer2.Burst {
	t.Helper()
	data, err := os.ReadFile("../testdata/d-sms.bin")
	if err != nil {
		t.Fatal(err)
	}
	raw := [33]byte(data[5*33:])
	raw[2] ^= 0xFF
	raw[3] ^= 0xFF
	raw[4] ^= 0xFF
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := b.Data.(*pdu.DataHeader)
	if !ok || !h.FEC.Uncorrectable || h.Format != pdu.FormatConfirmed {
		t.Fatalf("corrupt header decodes as %T %+v", b.Data, b.Data)
	}
	return b
}

func TestAssembler_HeaderCRCFailure(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	for i, b := range loadBursts(t, "../testdata/d-sms.bin") {
		if i == 5 {
			b = badHeader(t)
		}
		m, done, err := a.AddBurst(b)
		switch {
		case done:
			t.Fatalf("burst %d: message %+v", i, m)
		case i == 5 && (!errors.Is(err, packet.ErrHeaderCRC) || !errors.Is(err, fec.ErrUncorrectable)):
			t.Fatalf("header: %v, want ErrHeaderCRC", err)
		case i > 5 && !errors.Is(err, packet.ErrNoHeader):
			t.Fatalf("block %d: %v, want ErrNoHeader", i, err)
		}
	}
}

func TestAssembler_InProgress(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	if a.InProgress() {
		t.Fatal("new Assembler in progress")
	}
	bursts := loadBursts(t, "../testdata/d-sms.bin")
	started, finished := false, false
	for i, b := range bursts {
		_, done, err := a.AddBurst(b)
		if err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		if a.InProgress() {
			started = true
		}
		if done {
			finished = true
			if a.InProgress() {
				t.Fatalf("burst %d: in progress after the message completed", i)
			}
		}
	}
	if !started || !finished {
		t.Fatalf("started %v, finished %v", started, finished)
	}

	for i, b := range bursts {
		if _, _, err := a.AddBurst(b); err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		if a.InProgress() {
			a.Reset()
			if a.InProgress() {
				t.Fatal("in progress after Reset")
			}
			return
		}
	}
	t.Fatal("no packet started on the second pass")
}

func TestAssembler_HeaderCRCFailureKeepsPacket(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	var msg *packet.Message
	for i, b := range loadBursts(t, "../testdata/d-sms.bin") {
		m, err := addBurst(a, b)
		if err != nil || m != nil && msg != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		if m != nil {
			msg = m
		}
		if i == 7 {
			if _, err := addBurst(a, badHeader(t)); !errors.Is(err, packet.ErrHeaderCRC) {
				t.Fatalf("header: %v, want ErrHeaderCRC", err)
			}
		}
	}
	if msg == nil || !bytes.Contains(msg.Data, utf16le("TEST KI5VMF")) {
		t.Fatalf("message %+v", msg)
	}
}

func encodeBlock(t *testing.T, d layer2.Data) layer2.Data {
	t.Helper()
	switch b := d.(type) {
	case *pdu.Rate12Data:
		bits, err := b.Encode()
		if err != nil {
			t.Fatal(err)
		}
		out := pdu.DecodeRate12Data(bits)
		return &out
	case *pdu.Rate34Data:
		bits, err := b.Encode()
		if err != nil {
			t.Fatal(err)
		}
		out := pdu.DecodeRate34Data(bits)
		return &out
	case *pdu.Rate1Data:
		bits, err := b.Encode()
		if err != nil {
			t.Fatal(err)
		}
		out := pdu.DecodeRate1Data(bits)
		return &out
	}
	t.Fatalf("unexpected block %T", d)
	return nil
}

func encodeHeader(t *testing.T, h *pdu.DataHeader) *pdu.DataHeader {
	t.Helper()
	bits, err := h.Encode()
	if err != nil {
		t.Fatal(err)
	}
	out := pdu.DecodeDataHeader(bits)
	res := out.FEC
	if res.Uncorrectable {
		t.Fatal("header CRC")
	}
	return &out
}

func feed(t *testing.T, a *packet.Assembler, p packet.Packet) (*packet.Message, error) {
	t.Helper()
	m, err := add(a, encodeHeader(t, &p.Header))
	if m != nil || err != nil {
		return m, err
	}
	for i, b := range p.Blocks {
		m, err = add(a, encodeBlock(t, b))
		if (m != nil || err != nil) && i != len(p.Blocks)-1 {
			t.Fatalf("block %d: %v %v", i, m, err)
		}
	}
	return m, err
}

func unconfirmed() pdu.DataHeader {
	return pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001, FullMessageFlag: true}}
}

func confirmed() pdu.DataHeader {
	return pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002,
		LLIDSource: 3120001, FullMessageFlag: true, SendSequenceNumber: 5, FragmentSequenceNumber: 8}}
}

func payload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i*7 + 3)
	}
	return p
}

func TestSegment_RoundTrip(t *testing.T) {
	t.Parallel()

	rates := []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1}
	for _, h := range []pdu.DataHeader{unconfirmed(), confirmed()} {
		for _, rate := range rates {
			for _, n := range []int{0, 1, 7, 8, 9, 20, 100, 300} {
				p, err := packet.NewPacket(h, payload(n), rate)
				if err != nil {
					t.Fatal(err)
				}
				per, _ := packet.BlockOctets(rate, h.Format == pdu.FormatConfirmed)
				blocks, pad := packet.Layout(n, per)
				if len(p.Blocks) != blocks || (blocks*per-pad-4) != n {
					t.Fatalf("n=%d blocks %d pad %d", n, len(p.Blocks), pad)
				}
				a := packet.NewAssembler(127)
				m, err := feed(t, a, p)
				if err != nil || m == nil || !bytes.Equal(m.Data, payload(n)) || m.DataType != rate {
					t.Fatalf("%s n=%d rate %d: err %v msg %+v", h.Format.String(), n, rate, err, m)
				}
			}
		}
	}
	if h := unconfirmed(); h.UnconfirmedDataHeader.BlocksToFollow != 0 {
		t.Error("Segment changed the caller's header")
	}
}

func TestSegment_HeaderFields(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(unconfirmed(), payload(38+9), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	u := p.Header.UnconfirmedDataHeader
	if u.BlocksToFollow != 5 || u.PadOctetCount != 9 || p.Header.DataType() != enums.DataTypeDataHeader {
		t.Fatalf("BF %d POC %d", u.BlocksToFollow, u.PadOctetCount)
	}
	p, err = packet.NewPacket(confirmed(), payload(41), enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	c := p.Header.ConfirmedDataHeader
	if c.BlocksToFollow != 3 || c.PadOctetCount != 3 {
		t.Fatalf("BF %d POC %d", c.BlocksToFollow, c.PadOctetCount)
	}
	for i, b := range p.Blocks {
		r, ok := b.(*pdu.Rate34Data)
		if !ok || int(r.ConfirmedDBSN()) != i || !r.ConfirmedCRC9OK() {
			t.Errorf("block %d DBSN or CRC-9", i)
		}
	}
	last, ok := p.Blocks[2].(*pdu.Rate34Data)
	if !ok {
		t.Fatal("type")
	}
	ud := last.ConfirmedUserData()
	if ud[9] != 0 || ud[10] != 0 || ud[11] != 0 {
		t.Errorf("pad octets not zero: %x", ud)
	}
}

func TestSegment_Errors(t *testing.T) {
	t.Parallel()

	if _, err := packet.NewPacket(unconfirmed(), nil, enums.DataTypeCSBK); !errors.Is(err, packet.ErrRate) {
		t.Errorf("rate: %v", err)
	}
	if _, err := packet.NewPacket(pdu.DataHeader{Format: pdu.FormatUnconfirmed}, nil, enums.DataTypeRate12); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("nil sub-header: %v", err)
	}
	if _, err := packet.NewPacket(pdu.DataHeader{Format: pdu.FormatProprietary, ProprietaryHeader: &pdu.ProprietaryHeader{}}, nil, enums.DataTypeRate12); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("proprietary: %v", err)
	}
	if _, err := packet.NewPacket(unconfirmed(), payload(127*12-3), enums.DataTypeRate12); !errors.Is(err, packet.ErrTooLong) {
		t.Errorf("too long: %v", err)
	}
	if _, err := packet.NewPacket(unconfirmed(), payload(127*12-4), enums.DataTypeRate12); err != nil {
		t.Errorf("127 blocks: %v", err)
	}
	dd := pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{}}
	if _, err := packet.NewPacket(dd, payload(64*12), enums.DataTypeRate12); !errors.Is(err, packet.ErrTooLong) {
		t.Errorf("short data too long: %v", err)
	}
}

func TestSegment_ShortData(t *testing.T) {
	t.Parallel()

	for _, sarq := range []bool{false, true} {
		h := pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{
			SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2, DefinedData: 2, SARQ: sarq, FullMessageFlag: true}}
		p, err := packet.NewPacket(h, utf16le("hello"), enums.DataTypeRate12)
		if err != nil {
			t.Fatal(err)
		}
		a := packet.NewAssembler(127)
		m, err := feed(t, a, p)
		if err != nil || m.Header.DefinedDataHeader.AppendedBlocks != 2 || !bytes.HasPrefix(m.Data, utf16le("hello")) || len(m.Data) != map[bool]int{false: 20, true: 16}[sarq] {
			t.Fatalf("sarq %v: %v %+v", sarq, err, m)
		}
	}
	h := pdu.DataHeader{Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{SAP: 10, SourcePort: 1, DestinationPort: 2, FullMessageFlag: true}}
	p, err := packet.NewPacket(h, payload(30), enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	a := packet.NewAssembler(127)
	if m, err := feed(t, a, p); err != nil || !bytes.HasPrefix(m.Data, payload(30)) {
		t.Fatalf("raw: %v", err)
	}
}

func TestAssembler_Bursts(t *testing.T) {
	t.Parallel()

	p, err := packet.NewPacket(unconfirmed(), []byte("Hello from dmrgo"), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	pack := func(info [12]byte, dt enums.DataType) *layer2.Burst {
		raw, err := layer2.BuildDataBurst(info, dt, 1, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		b, err := layer2.NewBurstFromBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	hb, _ := p.Header.Encode()
	bursts := make([]*layer2.Burst, 0, 1+len(p.Blocks))
	bursts = append(bursts, pack(hb, enums.DataTypeDataHeader))
	for _, blk := range p.Blocks {
		r, _ := blk.(*pdu.Rate12Data)
		bits, _ := r.Encode()
		bursts = append(bursts, pack(bits, enums.DataTypeRate12))
	}
	a := packet.NewAssembler(127)
	var got *packet.Message
	for _, b := range bursts {
		m, err := addBurst(a, b)
		if err != nil {
			t.Fatal(err)
		}
		if m != nil {
			got = m
		}
	}
	if got == nil || string(got.Data) != "Hello from dmrgo" {
		t.Fatalf("got %+v", got)
	}
	if m, err := addBurst(a, &layer2.Burst{}); m != nil || err != nil {
		t.Errorf("voice burst: %v %v", m, err)
	}
	bad := layer2.Burst{Kind: bursts[2].Kind, Data: bursts[2].Data, FEC: bursts[2].FEC}
	bad.FEC.PDU.Uncorrectable = true
	a.Reset()
	var last error
	for _, b := range []*layer2.Burst{bursts[0], bursts[1], &bad} {
		_, last = addBurst(a, b)
	}
	if !errors.Is(last, packet.ErrMessageCRC) {
		t.Errorf("uncorrectable block: %v", last)
	}
}

func TestAssembler_SelectiveRetry(t *testing.T) {
	t.Parallel()

	data := []byte("LIP: 41.878,-87.629,hdg=90,v=12 and some more text")
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	rx := packet.NewAssembler(127)
	if m, err := add(rx, encodeHeader(t, &p.Header)); m != nil || err != nil {
		t.Fatal(m, err)
	}
	var err2 error
	for i, b := range p.Blocks {
		blk := encodeBlock(t, b)
		if i == 1 || i == 3 {
			r, _ := blk.(*pdu.Rate34Data)
			r.Data[7] ^= 0x10
		}
		_, err2 = add(rx, blk)
	}
	if !errors.Is(err2, packet.ErrBlocksMissing) || !errors.Is(err2, fec.ErrUncorrectable) {
		t.Fatalf("err %v", err2)
	}
	if got := rx.Missing(); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("missing %v", got)
	}
	resp, err := rx.Response()
	if err != nil {
		t.Fatal(err)
	}
	rh := resp.Header.ResponsePacketHeader
	if r, err := packet.ResponseOf(rh); err != nil || r != packet.ResponseSACK || rh.ResponseStatus != 5 || rh.LLIDDestination != 3120001 || rh.LLIDSource != 3120002 || len(resp.Blocks) != 1 {
		t.Fatalf("response %+v", rh)
	}

	tx := packet.NewAssembler(127)
	sack, err := feed(t, tx, resp)
	if err != nil || sack.Response == nil {
		t.Fatalf("SACK: %v", err)
	}
	retry := sack.Response.RetryBlocks()
	if len(retry) != 2 || retry[0] != 1 || retry[1] != 3 {
		t.Fatalf("retry %v", retry)
	}
	retryAndACK(t, rx, tx, p, data, retry)
}

func retryAndACK(t *testing.T, rx, tx *packet.Assembler, p packet.Packet, data []byte, retry []int) {
	t.Helper()
	rh2 := p.Header
	c := *rh2.ConfirmedDataHeader
	c.BlocksToFollow = uint8(len(retry))
	rh2.ConfirmedDataHeader = &c
	if m, err := add(rx, encodeHeader(t, &rh2)); m != nil || err != nil {
		t.Fatal(m, err)
	}
	if m, err := add(rx, encodeBlock(t, p.Blocks[1])); m != nil || err != nil {
		t.Fatal(m, err)
	}
	m, err := add(rx, encodeBlock(t, p.Blocks[3]))
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("after retry: %v %+v", err, m)
	}

	ack, err := packet.NewResponse(p.Header.ConfirmedDataHeader, packet.ResponseACK, nil)
	if err != nil || len(ack.Blocks) != 0 {
		t.Fatal(err)
	}
	am, err := add(tx, encodeHeader(t, &ack.Header))
	if err != nil || am == nil {
		t.Fatalf("ACK: %v %+v", err, am)
	}
	if r, err := packet.ResponseOf(am.Header.ResponsePacketHeader); err != nil || r != packet.ResponseACK {
		t.Fatalf("ACK: %v %v", err, r)
	}
}

func TestAssembler_Errors(t *testing.T) {
	t.Parallel()

	a := packet.NewAssembler(127)
	if _, err := add(a, &pdu.Rate12Data{}); !errors.Is(err, packet.ErrNoHeader) {
		t.Errorf("no header: %v", err)
	}
	if _, err := a.Response(); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("response: %v", err)
	}
	if m, err := add(a, &pdu.CSBK{}); m != nil || err != nil {
		t.Errorf("CSBK: %v", err)
	}
	p, _ := packet.NewPacket(unconfirmed(), payload(30), enums.DataTypeRate12)
	if _, err := add(a, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	if _, err := add(a, encodeBlock(t, p.Blocks[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := add(a, &pdu.Rate34Data{}); !errors.Is(err, packet.ErrRateChange) {
		t.Errorf("rate change: %v", err)
	}
	prop := &pdu.DataHeader{Format: pdu.FormatProprietary, ProprietaryHeader: &pdu.ProprietaryHeader{SAP: 9, MFID: 0x10}}
	if _, err := add(a, prop); !errors.Is(err, packet.ErrNoHeader) {
		t.Errorf("lone P_HEAD: %v", err)
	}
}

func TestAssembler_ProprietaryHeader(t *testing.T) {
	t.Parallel()

	h := unconfirmed()
	h.UnconfirmedDataHeader.SAP = enums.SAPProprietaryPacketData
	p, err := packet.NewPacket(h, payload(20), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	u := *p.Header.UnconfirmedDataHeader
	u.BlocksToFollow++
	p.Header.UnconfirmedDataHeader = &u
	a := packet.NewAssembler(127)
	if _, err := add(a, encodeHeader(t, &p.Header)); err != nil {
		t.Fatal(err)
	}
	if _, err := add(a, encodeHeader(t, &pdu.DataHeader{Format: pdu.FormatProprietary, ProprietaryHeader: &pdu.ProprietaryHeader{SAP: 9, MFID: 0x10}})); err != nil {
		t.Fatal(err)
	}
	var m *packet.Message
	for _, b := range p.Blocks {
		if m, err = add(a, encodeBlock(t, b)); err != nil {
			t.Fatal(err)
		}
	}
	if m == nil || m.Proprietary == nil || m.Proprietary.MFID != 0x10 || !bytes.Equal(m.Data, payload(20)) {
		t.Fatalf("got %+v", m)
	}
}

func TestResponseData(t *testing.T) {
	t.Parallel()

	var rd packet.ResponseData
	rd.Retry[0], rd.Retry[5], rd.Retry[63] = true, true, true
	blocks, err := rd.Encode(64)
	if err != nil || len(blocks) != 1 {
		t.Fatal(err)
	}
	if blocks[0].Data[0] != 0xDE || blocks[0].Data[7] != 0x7F {
		t.Errorf("flags %x", blocks[0].Data)
	}
	got, err := packet.DecodeResponseData(blocks...)
	if err != nil || got != rd {
		t.Fatalf("decode: %v", err)
	}
	short, _ := rd.Encode(6)
	if short[0].Data[7] != 0xFF {
		t.Errorf("flags past the packet must be 1: %x", short[0].Data)
	}
	rd.Retry[126] = true
	two, err := rd.Encode(127)
	if err != nil || len(two) != 2 {
		t.Fatal(err)
	}
	got, err = packet.DecodeResponseData(two...)
	if err != nil || got != rd || len(got.RetryBlocks()) != 4 {
		t.Fatalf("two blocks: %v %v", err, got.RetryBlocks())
	}
	two[0].Data[0] ^= 0x01
	bad, err := packet.DecodeResponseData(two...)
	if !errors.Is(err, packet.ErrResponseData) || !errors.Is(err, fec.ErrUncorrectable) || bad.Retry[0] == rd.Retry[0] || bad.Retry[126] != rd.Retry[126] {
		t.Errorf("bad CRC: %v, flags %v; want the decoded flags and an uncorrectable error", err, bad.RetryBlocks())
	}
	if _, err := packet.DecodeResponseData(); !errors.Is(err, packet.ErrResponseData) {
		t.Errorf("no blocks: %v", err)
	}
	if _, err := rd.Encode(128); err == nil {
		t.Error("128 blocks")
	}
}

func TestResponseCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		r          packet.Response
		class, typ uint8
		name       string
	}{
		{packet.ResponseACK, 0, 1, "ACK"},
		{packet.ResponseNACKIllegalFormat, 1, 0, "NACK Illegal Format"},
		{packet.ResponseNACKPacketCRC, 1, 1, "NACK Packet CRC"},
		{packet.ResponseNACKMemoryFull, 1, 2, "NACK Memory Full"},
		{packet.ResponseNACKFSNOutOfSequence, 1, 3, "NACK FSN Out Of Sequence"},
		{packet.ResponseNACKUndeliverable, 1, 4, "NACK Undeliverable"},
		{packet.ResponseNACKOutOfSequence, 1, 5, "NACK Out Of Sequence"},
		{packet.ResponseNACKInvalidUser, 1, 6, "NACK Invalid User"},
		{packet.ResponseSACK, 2, 0, "SACK"},
	}
	for _, tt := range tests {
		r, err := packet.NewResponseCode(tt.class, tt.typ)
		if tt.r.Class() != tt.class || tt.r.Type() != tt.typ || err != nil || r != tt.r || tt.r.String() != tt.name {
			t.Errorf("%s: class %d type %d", tt.name, tt.r.Class(), tt.r.Type())
		}
	}
	if r, err := packet.NewResponseCode(3, 7); err != nil || r.String() != "Unknown Response (class 3, type 7)" {
		t.Error("unknown name")
	}
	var rangeErr *dmrgo.FieldRangeError
	if _, err := packet.NewResponseCode(4, 0); !errors.As(err, &rangeErr) || rangeErr.Field != "Class" || rangeErr.Bits != 2 {
		t.Errorf("class 4: error %v, want the FieldRangeError of Class", err)
	}
	if _, err := packet.NewResponseCode(0, 8); !errors.As(err, &rangeErr) || rangeErr.Field != "Type" || rangeErr.Bits != 3 {
		t.Errorf("type 8: error %v, want the FieldRangeError of Type", err)
	}
	if _, err := packet.ResponseOf(&pdu.ResponsePacketHeader{ResponseType: 8}); !errors.As(err, &rangeErr) || rangeErr.Field != "Type" {
		t.Errorf("ResponseOf with type 8: error %v, want the FieldRangeError of Type", err)
	}
	h := confirmed()
	r, err := packet.NewResponse(h.ConfirmedDataHeader, packet.ResponseNACKFSNOutOfSequence, nil)
	if err != nil || r.Header.ResponsePacketHeader.ResponseStatus != 0 || r.Header.ResponsePacketHeader.ResponseClass != 1 {
		t.Errorf("FSN NACK status: %+v", r.Header.ResponsePacketHeader)
	}
}

func TestMessageCRC(t *testing.T) {
	t.Parallel()

	data := payload(36)
	ref := func(d []byte) uint32 {
		var r uint32
		for i := 0; i < len(d); i += 2 {
			for _, o := range []byte{d[i+1], d[i]} {
				for b := 7; b >= 0; b-- {
					top := r>>31 ^ uint32(o>>b)&1
					r <<= 1
					if top == 1 {
						r ^= 0x04C11DB7
					}
				}
			}
		}
		return r
	}
	if packet.MessageCRC(data) != ref(data) {
		t.Fatal("CRC differs from the bitwise B.3.9 reference")
	}
	full := packet.AppendMessageCRC(data)
	c := ref(data)
	if !bytes.Equal(full[36:], []byte{byte(c), byte(c >> 8), byte(c >> 16), byte(c >> 24)}) || !packet.CheckMessageCRC(full) {
		t.Fatal("CRC octet order")
	}
	if packet.CheckMessageCRC(full[:3]) {
		t.Error("short buffer")
	}
}

func add(a *packet.Assembler, d layer2.Data) (*packet.Message, error) {
	m, done, err := a.Add(d)
	if !done {
		return nil, err
	}
	return &m, err
}

func addBurst(a *packet.Assembler, b *layer2.Burst) (*packet.Message, error) {
	m, done, err := a.AddBurst(b)
	if !done {
		return nil, err
	}
	return &m, err
}
