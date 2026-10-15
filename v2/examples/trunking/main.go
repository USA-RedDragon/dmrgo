// Command trunking builds a Tier III control channel (CSBKs, an MBC channel
// grant and the CACH Short LC system parameters) from generated PDUs, puts it
// on air as bursts and decodes it back.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const colorCode = 7

func pack96(bits [96]bit.Bit) [12]byte {
	var p [12]byte
	copy(p[:], bit.Pack(bits[:]))
	return p
}

func unpack96(p [12]byte) [96]bit.Bit {
	var bits [96]bit.Bit
	copy(bits[:], bit.Unpack(p[:]))
	return bits
}

func buildBurst(bits [96]bit.Bit, dt enums.DataType, src layer2.SyncSource) (layer2.RawBurst, error) {
	p := pack96(bits)
	if dt != enums.DataTypeMBCContinuation && !pdu.BlockCRCOK(p, dt) {
		return layer2.RawBurst{}, fmt.Errorf("%s: block CRC does not check", dt)
	}
	return layer2.BuildDataBurst(p, dt, colorCode, src)
}

func decodeAs[T layer2.Data](raw layer2.RawBurst) (T, *layer2.Burst, error) {
	var zero T
	cc := uint8(colorCode)
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(raw, layer2.DecodeOptions{ColorCode: &cc}); err != nil {
		return zero, nil, err
	}
	d, ok := b.Data.(T)
	if !ok {
		return zero, nil, fmt.Errorf("decoded %T, want %T", b.Data, d)
	}
	return d, &b, nil
}

func roundTripCSBK(c *pdu.CSBK, src layer2.SyncSource) (*pdu.CSBK, *layer2.Burst, error) {
	bits, err := c.Encode()
	if err != nil {
		return nil, nil, err
	}
	raw, err := buildBurst(bits, enums.DataTypeCSBK, src)
	if err != nil {
		return nil, nil, err
	}
	return decodeAs[*pdu.CSBK](raw)
}

func outboundCSBKs(w io.Writer, sc pdu.SysCode) error {
	code, err := sc.Pack()
	if err != nil {
		return err
	}
	csbks := []*pdu.CSBK{
		{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{
			TSCCAS: true, SiteTSSync: true, Version: 1, NRandWait: 5, Reg: true, Backoff: 3, SysIdentCode: code}},
		{LastBlock: true, Opcode: pdu.CSBKBroadcast, CBroadcastPDU: &pdu.CBroadcastPDU{Reg: true, Backoff: 2, SysIdentCode: code}},
		{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
			PhysicalChannel: 42, LogicalChannel: true, TargetAddress: 9, SourceAddress: 3120001}},
		{LastBlock: true, Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{
			PhysicalChannel: 43, TargetAddress: 3120002, SourceAddress: 3120001}},
		{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{TargetAddress: 3120002, SourceAddress: 3120001}},
		{LastBlock: true, Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &pdu.AckOutboundPDU{ReasonCode: 0x62, TargetAddress: 3120002}},
		{LastBlock: true, Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{Mask: 3, Reg: true, Backoff: 4, PhysicalChannel: 12, MSAddress: 3120002}},
	}
	for _, c := range csbks {
		got, b, err := roundTripCSBK(c, layer2.SyncSourceBS)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Opcode, err)
		}
		if got.FEC.Uncorrectable || b.SyncPattern != enums.SyncBSSourcedData {
			return fmt.Errorf("%s: FEC %+v, sync %s", c.Opcode, got.FEC, b.SyncPattern)
		}
		want, _ := c.Encode()
		back, err := got.Encode()
		if err != nil || back != want {
			return fmt.Errorf("%s: decoded %v, want %v", c.Opcode, got, c)
		}
	}
	fmt.Fprintf(w, "outbound CSBKs: %d round trips ok\n", len(csbks))

	aloha, _, err := roundTripCSBK(csbks[0], layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	got := pdu.UnpackSysCode(aloha.AlohaPDU.SysIdentCode)
	if got != sc {
		return fmt.Errorf("C_ALOHA SYScode %+v, want %+v", got, sc)
	}
	fmt.Fprintf(w, "C_ALOHA SYScode 0x%04x: model %d net %d site %d par %d\n", code, got.Model, got.Net, got.Site, got.Par)

	return encodeMatchesBuild(w, csbks[2])
}

func encodeMatchesBuild(w io.Writer, c *pdu.CSBK) error {
	b := layer2.Burst{
		Kind:        layer2.KindData,
		SyncPattern: enums.SyncBSSourcedData,
		SlotType:    pdu.SlotType{ColorCode: colorCode, DataType: enums.DataTypeCSBK},
		Data:        c,
	}
	enc, err := b.Encode()
	if err != nil {
		return err
	}
	bits, err := c.Encode()
	if err != nil {
		return err
	}
	built, err := buildBurst(bits, enums.DataTypeCSBK, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	if enc != built {
		return errors.New("burst.Encode and BuildDataBurst differ")
	}
	fmt.Fprintf(w, "Burst.Encode matches BuildDataBurst for %s\n", c.Opcode)
	return nil
}

func opcode0x38(w io.Writer) error {
	mi := pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 77, TargetAddress: 9, SourceAddress: 3120001}
	payload, err := mi.Encode()
	if err != nil {
		return err
	}
	c := &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupDataGrantMultiItem, RawPayload: payload}
	got, _, err := roundTripCSBK(c, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	if got.Hint0x38 != pdu.Opcode0x38TalkgroupDataGrantMultiItem {
		return fmt.Errorf("opcode 0x38 hint %d from a BS-sourced burst", got.Hint0x38)
	}
	back, ok := got.AsTalkgroupDataGrantMultiItem()
	if !ok || back != mi {
		return fmt.Errorf("TD_GRANT_MI %+v, want %+v", back, mi)
	}
	fmt.Fprintf(w, "opcode 0x38 from BS: TD_GRANT_MI ch %d tg %d src %d\n", back.PhysicalChannel, back.TargetAddress, back.SourceAddress)
	return nil
}

func randomAccess(w io.Writer) error {
	c := &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKRandomAccess, RandomAccessPDU: &pdu.RandomAccessPDU{
		TargetAddress: 9, SourceAddress: 3120001}}
	got, b, err := roundTripCSBK(c, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	if b.SyncPattern != enums.SyncMSSourcedData || got.RandomAccessPDU == nil || *got.RandomAccessPDU != *c.RandomAccessPDU {
		return fmt.Errorf("C_RAND: sync %s, PDU %v", b.SyncPattern, got.RandomAccessPDU)
	}
	fmt.Fprintf(w, "inbound %s ok, sync %s\n", got.Opcode, b.SyncPattern)
	return nil
}

func mbcGrant(w io.Writer) error {
	grant := pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: 9, SourceAddress: 3120001}
	gbits, err := grant.Encode()
	if err != nil {
		return err
	}
	hdr := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant}
	copy(hdr.Data[:], bit.Pack(gbits[:]))
	hbits, err := hdr.Encode()
	if err != nil {
		return err
	}
	cgap := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: colorCode,
		CdefParms: layer3.CdefParms{Channel: 0xFFF, TXMHz: 451, TXKHz: 1000, RXMHz: 456, RXKHz: 1000}}
	cbits, err := cgap.Encode()
	if err != nil {
		return err
	}
	conts := [][96]bit.Bit{cbits}
	pdu.SetMBCLastBlockCRC(conts)

	hraw, err := buildBurst(hbits, enums.DataTypeMBCHeader, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	craw, err := buildBurst(conts[0], enums.DataTypeMBCContinuation, layer2.SyncSourceBS)
	if err != nil {
		return err
	}

	gotHdr, _, err := decodeAs[*pdu.MBCHeader](hraw)
	if err != nil {
		return err
	}
	var hdata [64]bit.Bit
	copy(hdata[:], bit.Unpack(gotHdr.Data[:]))
	gotGrant, res := pdu.DecodeTalkgroupVoiceGrantPDU(hdata)
	if gotHdr.FEC.Uncorrectable || res.Uncorrectable || gotGrant != grant {
		return fmt.Errorf("MBC header grant %+v, want %+v", gotGrant, grant)
	}

	if _, _, err := decodeAs[*pdu.MBCContinuation](craw); err != nil {
		return err
	}
	p, res := craw.BPTCPayload()
	rx := unpack96(p)
	if res.Uncorrectable || !pdu.CheckMBCLastBlockCRC(rx) {
		return errors.New("MBC last block CRC does not check")
	}
	gotCGAP, res := pdu.DecodeCGAPContinuation(rx)
	if res.Uncorrectable || gotCGAP.CdefParms != cgap.CdefParms {
		return fmt.Errorf("CGAP %+v, want %+v", gotCGAP.CdefParms, cgap.CdefParms)
	}
	fmt.Fprintf(w, "MBC %s ch 0x%X tg %d: TX %.4f MHz RX %.4f MHz\n", gotHdr.Opcode, gotGrant.PhysicalChannel,
		gotGrant.TargetAddress, gotCGAP.CdefParms.TXFrequencyMHz(), gotCGAP.CdefParms.RXFrequencyMHz())
	return nil
}

func cachSysParms(w io.Writer) error {
	codes := []pdu.SysCode{
		{Model: pdu.NetworkModelTiny, Net: 300, Site: 5},
		{Model: pdu.NetworkModelSmall, Net: 100, Site: 20},
		{Model: pdu.NetworkModelLarge, Net: 9, Site: 200},
		{Model: pdu.NetworkModelHuge, Net: 2, Site: 1000},
	}
	var asm layer2.ShortLCAssembler
	n := 0
	for _, sc := range codes {
		var cs pdu.ShortLCCSysParms
		var ps pdu.ShortLCPSysParms
		if err := cs.SetSysCode(sc); err != nil {
			return err
		}
		if err := ps.SetSysCode(sc); err != nil {
			return err
		}
		cs.Reg, cs.CommonSlotCounter, ps.CommonSlotCounter = true, 123, 123
		for _, slc := range []*pdu.ShortLC{
			{SLCO: enums.SLCOCSysParms, CSysParms: &cs},
			{SLCO: enums.SLCOPSysParms, PSysParms: &ps},
		} {
			got, err := cachRoundTrip(&asm, slc)
			if err != nil {
				return err
			}
			var back pdu.SysCode
			if got.CSysParms != nil {
				back = got.CSysParms.SysCode()
			} else if got.PSysParms != nil {
				back = got.PSysParms.SysCode()
			}
			if back != sc {
				return fmt.Errorf("%s: SYScode %+v, want %+v", slc.SLCO, back, sc)
			}
			n++
		}
	}
	fmt.Fprintf(w, "CACH Short LC: %d C_SYS_Parms/P_SYS_Parms round trips ok over 4 models\n", n)
	return nil
}

func cachRoundTrip(asm *layer2.ShortLCAssembler, slc *pdu.ShortLC) (pdu.ShortLC, error) {
	fields, err := layer2.EncodeShortLCToCACH(slc, pdu.TACT{AccessType: true})
	if err != nil {
		return pdu.ShortLC{}, err
	}
	done := false
	for i, f := range fields {
		c := layer2.DecodeCACH(layer2.CACHDeinterleave(f))
		if c.FEC.Uncorrectable {
			return pdu.ShortLC{}, fmt.Errorf("CACH %d: TACT uncorrectable", i)
		}
		if done, err = asm.AddFragment(c.TACT.LCSS, c.Payload); err != nil {
			return pdu.ShortLC{}, err
		}
	}
	if !done {
		return pdu.ShortLC{}, errors.New("short LC incomplete after 4 CACH fields")
	}
	got, err := asm.Complete()
	if err != nil {
		return got, err
	}
	if got.FEC.Uncorrectable || got.SLCO != slc.SLCO {
		return got, fmt.Errorf("short LC %s: FEC %+v", got.SLCO, got.FEC)
	}
	return got, nil
}

func run(w io.Writer, _ string) error {
	sc := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 0x55, Site: 0x13, Par: 1}
	if err := outboundCSBKs(w, sc); err != nil {
		return err
	}
	for _, step := range []func(io.Writer) error{opcode0x38, randomAccess, mbcGrant, cachSysParms} {
		if err := step(w); err != nil {
			return err
		}
	}
	fmt.Fprintln(w, "all checks passed")
	return nil
}

func main() {
	def := filepath.Join("..", "..", "layer2", "testdata")
	if _, file, _, ok := runtime.Caller(0); ok {
		def = filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
	}
	dataDir := flag.String("data", def, "directory with the burst captures")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
