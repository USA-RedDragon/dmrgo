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

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const colorCode = 7

func decodeAs[T layer2.Data](raw layer2.RawBurst) (T, *layer2.Burst, error) {
	var zero T
	cc := enums.ColorCode(colorCode)
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(raw, &layer2.DecodeOptions{ColorCode: &cc}); err != nil {
		return zero, nil, err
	}
	d, ok := b.Data.(T)
	if !ok {
		return zero, nil, fmt.Errorf("decoded %T, want %T", b.Data, d)
	}
	return d, &b, nil
}

func roundTripCSBK(c *pdu.CSBK, src layer2.SyncSource) (*pdu.CSBK, *layer2.Burst, error) {
	raw, err := layer2.BuildBurst(c, colorCode, src)
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
		{LastBlock: true, Opcode: pdu.CSBKBroadcast, BroadcastPDU: &pdu.BroadcastPDU{Reg: true, Backoff: 2, SysIdentCode: code}},
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
		if err := got.FEC.Err(); err != nil {
			return fmt.Errorf("%s: %w", c.Opcode, err)
		}
		if b.SyncPattern != enums.SyncBSSourcedData {
			return fmt.Errorf("%s: sync %s", c.Opcode, b.SyncPattern)
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
	var got pdu.SysCode
	if got.Unpack(aloha.AlohaPDU.SysIdentCode); got != sc {
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
	built, err := layer2.BuildBurst(c, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	if enc != built {
		return errors.New("burst.Encode and BuildBurst differ")
	}
	fmt.Fprintf(w, "Burst.Encode matches BuildBurst for %s\n", c.Opcode)
	return nil
}

func opcode0x38(w io.Writer) error {
	mi := pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 77, TargetAddress: 9, SourceAddress: 3120001}
	c := &pdu.CSBK{LastBlock: true}
	if err := c.SetTalkgroupDataGrantMultiItem(mi); err != nil {
		return err
	}
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
	hdr := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &grant}
	cgap := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: colorCode,
		CdefParms: layer3.CdefParms{Channel: 0xFFF, TXMHz: 451, TXKHz: 1000, RXMHz: 456, RXKHz: 1000}}
	hraw, err := layer2.BuildBurst(&hdr, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	craw, err := layer2.BuildBurst(&cgap, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return err
	}

	gotHdr, _, err := decodeAs[*pdu.MBCHeader](hraw)
	if err != nil {
		return err
	}
	if gotHdr.FEC.Uncorrectable || gotHdr.TalkgroupVoiceGrantPDU == nil || *gotHdr.TalkgroupVoiceGrantPDU != grant {
		return fmt.Errorf("MBC header grant %+v, want %+v", gotHdr.TalkgroupVoiceGrantPDU, grant)
	}

	gotCont, _, err := decodeAs[*pdu.MBCContinuation](craw)
	if err != nil {
		return err
	}
	gotCGAP := gotCont.AsCGAP()
	if err := gotCGAP.FEC.Err(); err != nil {
		return fmt.Errorf("MBC last block CRC: %w", err)
	}
	if gotCGAP.CdefParms != cgap.CdefParms {
		return fmt.Errorf("CGAP %+v, want %+v", gotCGAP.CdefParms, cgap.CdefParms)
	}
	fmt.Fprintf(w, "MBC %s ch 0x%X tg %d: TX %.4f MHz RX %.4f MHz\n", gotHdr.Opcode, gotHdr.TalkgroupVoiceGrantPDU.PhysicalChannel,
		gotHdr.TalkgroupVoiceGrantPDU.TargetAddress, gotCGAP.CdefParms.TXFrequencyMHz(), gotCGAP.CdefParms.RXFrequencyMHz())
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
	var got pdu.ShortLC
	done := false
	for i, f := range fields {
		c := layer2.DecodeCACHField(f)
		if err := c.TACT.FEC.Err(); err != nil {
			return pdu.ShortLC{}, fmt.Errorf("CACH %d TACT: %w", i, err)
		}
		if got, done, err = asm.Add(c); err != nil {
			return pdu.ShortLC{}, err
		}
	}
	if !done {
		return pdu.ShortLC{}, errors.New("short LC incomplete after 4 CACH fields")
	}
	if err := got.FEC.Err(); err != nil {
		return got, fmt.Errorf("short LC %s: %w", got.SLCO, err)
	}
	if got.SLCO != slc.SLCO {
		return got, fmt.Errorf("short LC %s, want %s", got.SLCO, slc.SLCO)
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
