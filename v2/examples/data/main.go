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

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
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

// capture feeds every burst of a capture to an Assembler and prints each
// reassembled message.
func capture(w io.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw)%33 != 0 {
		return fmt.Errorf("length %d is not a multiple of 33", len(raw))
	}
	var a packet.Assembler
	n := 0
	for off := 0; off < len(raw); off += 33 {
		b, err := layer2.NewBurstFromBytes([33]byte(raw[off:]))
		if err != nil {
			return fmt.Errorf("burst %d: %w", off/33, err)
		}
		m, done, err := a.AddBurst(b)
		if err != nil {
			return fmt.Errorf("burst %d: %w", off/33, err)
		}
		if !done {
			continue
		}
		n++
		dstID, srcID, _ := m.Header.Addresses()
		fmt.Fprintf(w, "%s: %s %d -> %d, %s, %d octets, CRC %08x, text %q\n",
			filepath.Base(path), m.Header.Format, srcID, dstID, m.DataType, len(m.Data), m.CRC, utf16Text(m.Data))
	}
	if n != 1 {
		return fmt.Errorf("got %d messages, want 1", n)
	}
	return nil
}

// utf16Text returns the longest run of UTF-16LE printable ASCII in p. The SMS
// text follows vendor headers that differ between the captures.
func utf16Text(p []byte) string {
	var best, cur []byte
	for i := 0; i+1 < len(p); i += 2 {
		if p[i+1] == 0 && p[i] >= 0x20 && p[i] < 0x7f {
			cur = append(cur, p[i])
			continue
		}
		if len(cur) > len(best) {
			best = cur
		}
		cur = nil
	}
	if len(cur) > len(best) {
		best = cur
	}
	return string(best)
}

// unconfirmed sends a text message behind a UDP/IPv4 compressed header as an
// unconfirmed rate 1/2 packet and reassembles it from the coded bursts.
func unconfirmed(w io.Writer) error {
	udp := pdu.UDPIPv4CompressedHeader{IPv4Identification: 0x1234, SPID: enums.SPIDTextMessage, DPID: enums.DPIDTextMessage}
	udpBits, err := udp.Encode()
	if err != nil {
		return err
	}
	text := "Hello from dmrgo, unconfirmed SMS test"
	payload := append(bit.Pack(udpBits[:]), text...)

	h := pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: dst, LLIDSource: src, FullMessageFlag: true}}
	p, err := packet.Segment(h, payload, enums.DataTypeRate12)
	if err != nil {
		return err
	}
	bursts, err := packet.Bursts(p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	var a packet.Assembler
	m, err := receive(&a, bursts)
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, payload) {
		return fmt.Errorf("payload %x, want %x", m.Data, payload)
	}
	got := pdu.DecodeUDPIPv4CompressedHeader([72]bit.Bit(bit.Unpack(m.Data[:9])))
	u := m.Header.UnconfirmedDataHeader
	fmt.Fprintf(w, "unconfirmed: %d bursts, BF=%d POC=%d, port %d, text %q: ok\n",
		len(bursts), u.BlocksToFollow, u.PadOctetCount, enums.DPIDToPort(got.DPID), m.Data[9:])
	return nil
}

// confirmed sends a confirmed rate 3/4 packet with one block that passes the
// trellis decoder but fails its CRC-9. The receiver asks for it again with a
// SACK, the sender retries it, and the receiver acknowledges the packet.
func confirmed(w io.Writer) error {
	lip := []byte("LIP: 41.878,-87.629,hdg=90,v=12")
	h := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: dst, LLIDSource: src,
		FullMessageFlag: true, ReSynchronizeFlag: true, SendSequenceNumber: 3, FragmentSequenceNumber: 8}}
	p, err := packet.Segment(h, lip, enums.DataTypeRate34)
	if err != nil {
		return err
	}
	bursts, err := packet.Bursts(p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	good := bursts[2]
	blk, ok := p.Blocks[1].(*pdu.Rate34Data)
	if !ok {
		return fmt.Errorf("block 1 is %T", p.Blocks[1])
	}
	bad := *blk
	bad.Data[7] ^= 0x10
	if bursts[2], err = layer2.BuildRate34DataBurst(bad.Data, colorCode, layer2.SyncSourceMS); err != nil {
		return err
	}

	var rx packet.Assembler
	if _, err := receive(&rx, bursts); !errors.Is(err, packet.ErrBlocksMissing) {
		return fmt.Errorf("corrupted packet: %w, want %w", err, packet.ErrBlocksMissing)
	}
	sack, err := rx.Response()
	if err != nil {
		return err
	}
	sackBursts, err := packet.Bursts(sack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	var tx packet.Assembler
	rm, err := receive(&tx, sackBursts)
	if err != nil {
		return err
	}
	if rm.Response == nil {
		return errors.New("SACK carries no response data")
	}
	retry := rm.Response.RetryBlocks()
	fmt.Fprintf(w, "confirmed: %d bursts, missing %v, %s asks for %v\n",
		len(bursts), rx.Missing(), packet.ResponseOf(rm.Header.ResponsePacketHeader), retry)
	if !slices.Equal(retry, []int{1}) {
		return fmt.Errorf("retry %v, want [1]", retry)
	}

	m, err := receive(&rx, []layer2.RawBurst{bursts[0], good})
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, lip) {
		return fmt.Errorf("payload %q, want %q", m.Data, lip)
	}
	ack, err := packet.NewResponse(m.Header.ConfirmedDataHeader, packet.ResponseACK, nil)
	if err != nil {
		return err
	}
	ackBursts, err := packet.Bursts(ack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	am, err := receive(&tx, ackBursts)
	if err != nil {
		return err
	}
	ackDst, ackSrc, _ := am.Header.Addresses()
	fmt.Fprintf(w, "confirmed: retry done, %q, %s %d -> %d: ok\n",
		m.Data, packet.ResponseOf(am.Header.ResponsePacketHeader), ackSrc, ackDst)
	return nil
}

// receive decodes bursts and adds them to a until it returns a message or an
// error.
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
