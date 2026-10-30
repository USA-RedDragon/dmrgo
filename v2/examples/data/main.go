// Command data reassembles the SMS captures and segments confirmed and unconfirmed packets, with a selective retry.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

const (
	colorCode = 1
	src       = 3120001
	dst       = 3120002
)

func main() {
	_, file, _, ok := runtime.Caller(0)
	defDir := "testdata"
	if ok {
		defDir = filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
	}
	dataDir := flag.String("data", defDir, "directory with the burst captures")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "data:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, dataDir string) error {
	for _, name := range []string{"d-sms", "h-sms", "m-sms"} {
		if err := capture(w, filepath.Join(dataDir, name+".bin")); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := unconfirmed(w); err != nil {
		return fmt.Errorf("unconfirmed: %w", err)
	}
	if err := confirmed(w); err != nil {
		return fmt.Errorf("confirmed: %w", err)
	}
	return nil
}

func capture(w io.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw)%layer2.RawBurstSize != 0 {
		return fmt.Errorf("length %d is not a multiple of %d", len(raw), layer2.RawBurstSize)
	}
	a := packet.NewAssembler(127)
	n := 0
	b := layer2.NewBurst()
	for off := 0; off < len(raw); off += layer2.RawBurstSize {
		if err := b.DecodeFromBytes(layer2.RawBurst(raw[off:])); err != nil {
			return fmt.Errorf("burst %d: %w", off/layer2.RawBurstSize, err)
		}
		m, done, err := a.AddBurst(b)
		if err != nil {
			return fmt.Errorf("burst %d: %w", off/layer2.RawBurstSize, err)
		}
		if !done {
			continue
		}
		n++
		_, t, err := sms.Decode(nil, &m.Header, m.Data)
		if err != nil {
			return fmt.Errorf("message %d: %w", n, err)
		}
		fmt.Fprintf(w, "%s: %s %d -> %d, %s, %d octets, CRC %08x, text %q\n",
			filepath.Base(path), m.Header.Format(), t.Source, t.Destination, m.DataType, len(m.Data), m.CRC, t.Body)
	}
	if n != 1 {
		return fmt.Errorf("got %d messages, want 1", n)
	}
	return nil
}

func unconfirmed(w io.Writer) error {
	udp := pdu.UDPIPv4CompressedHeader{IPv4Identification: 0x1234, SPID: enums.SPIDTextMessage, DPID: enums.DPIDTextMessage}
	udpHeader, err := udp.Encode()
	if err != nil {
		return err
	}
	text := "Hello from dmrgo, unconfirmed SMS test"
	payload := append(udpHeader[:], text...)

	h, err := pdu.UnconfirmedDataHeader{
		SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: dst, LLIDSource: src, FullMessageFlag: true}.DataHeader()
	if err != nil {
		return err
	}
	p, err := packet.NewPacket(h, payload, enums.DataTypeRate12)
	if err != nil {
		return err
	}
	bursts, err := packet.Bursts(&p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	a := packet.NewAssembler(127)
	m, err := receive(a, bursts)
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, payload) {
		return fmt.Errorf("payload %x, want %x", m.Data, payload)
	}
	got := pdu.DecodeUDPIPv4CompressedHeader([9]byte(m.Data[:9]))
	u, ok := m.Header.AsUnconfirmed()
	if !ok {
		return fmt.Errorf("header format %s, want unconfirmed", m.Header.Format())
	}
	fmt.Fprintf(w, "unconfirmed: %d bursts, BF=%d POC=%d, port %d, text %q: ok\n",
		len(bursts), u.BlocksToFollow, u.PadOctetCount, uint16(ip.DPIDPort(got.DPID)), m.Data[9:])
	return nil
}

func confirmed(w io.Writer) error {
	lip := []byte("LIP: 41.878,-87.629,hdg=90,v=12")
	bursts, good, err := lossyConfirmed(lip)
	if err != nil {
		return err
	}
	rx, tx, err := requestRetry(w, bursts)
	if err != nil {
		return err
	}
	return completeRetry(w, rx, tx, lip, bursts[0], good)
}

func lossyConfirmed(lip []byte) ([]layer2.RawBurst, layer2.RawBurst, error) {
	h, err := pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: dst, LLIDSource: src,
		FullMessageFlag: true, ReSynchronizeFlag: true, SendSequenceNumber: 3, FragmentSequenceNumber: 8}.DataHeader()
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	p, err := packet.NewPacket(h, lip, enums.DataTypeRate34)
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	bursts, err := packet.Bursts(&p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	good := bursts[2]
	blk, ok := p.Blocks[1].(*pdu.Rate34Data)
	if !ok {
		return nil, layer2.RawBurst{}, fmt.Errorf("block 1 is %T", p.Blocks[1])
	}
	bad := *blk
	bad.Data[7] ^= 0x10
	if bursts[2], err = layer2.BuildRate34DataBurst(bad.Data, colorCode, layer2.SyncSourceMS); err != nil {
		return nil, layer2.RawBurst{}, err
	}
	return bursts, good, nil
}

func requestRetry(w io.Writer, bursts []layer2.RawBurst) (*packet.Assembler, *packet.Assembler, error) {
	rx := packet.NewAssembler(127)
	if _, err := receive(rx, bursts); !errors.Is(err, packet.ErrBlocksMissing) {
		return nil, nil, fmt.Errorf("corrupted packet: %w, want %w", err, packet.ErrBlocksMissing)
	}
	sack, err := rx.Response()
	if err != nil {
		return nil, nil, err
	}
	sackBursts, err := packet.Bursts(&sack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return nil, nil, err
	}
	tx := packet.NewAssembler(127)
	rm, err := receive(tx, sackBursts)
	if err != nil {
		return nil, nil, err
	}
	if rm.Response == nil {
		return nil, nil, errors.New("SACK carries no response data")
	}
	retry := rm.Response.RetryBlocks()
	sackCode, err := responseOf(&rm.Header)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(w, "confirmed: %d bursts, missing %v, %s requests %v\n",
		len(bursts), rx.Missing(), sackCode, retry)
	if !slices.Equal(retry, []int{1}) {
		return nil, nil, fmt.Errorf("retry %v, want [1]", retry)
	}
	return rx, tx, nil
}

func completeRetry(w io.Writer, rx, tx *packet.Assembler, lip []byte, header, good layer2.RawBurst) error {
	m, err := receive(rx, []layer2.RawBurst{header, good})
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, lip) {
		return fmt.Errorf("payload %q, want %q", m.Data, lip)
	}
	ch, ok := m.Header.AsConfirmed()
	if !ok {
		return fmt.Errorf("header format %s, want confirmed", m.Header.Format())
	}
	ack, err := packet.NewResponse(&ch, packet.ResponseACK, nil)
	if err != nil {
		return err
	}
	ackBursts, err := packet.Bursts(&ack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	am, err := receive(tx, ackBursts)
	if err != nil {
		return err
	}
	ackDst, ackSrc, _ := am.Header.Addresses()
	ackCode, err := responseOf(&am.Header)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "confirmed: retry done, %q, %s %d -> %d: ok\n",
		m.Data, ackCode, ackSrc, ackDst)
	return nil
}

func receive(a *packet.Assembler, bursts []layer2.RawBurst) (packet.Message, error) {
	for i, raw := range bursts {
		var b layer2.Burst
		if err := b.DecodeFromBytes(raw); err != nil {
			return packet.Message{}, fmt.Errorf("burst %d: %w", i, err)
		}
		m, done, err := a.AddBurst(&b)
		if err != nil || done {
			return m, err
		}
	}
	return packet.Message{}, errors.New("packet incomplete")
}

func responseOf(h *pdu.DataHeader) (packet.Response, error) {
	r, ok := h.AsResponse()
	if !ok {
		return 0, fmt.Errorf("header format %s, want response", h.Format())
	}
	return packet.ResponseOf(&r)
}
