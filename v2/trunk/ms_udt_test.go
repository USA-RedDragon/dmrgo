package trunk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const peer = 3120009

func ahoyBurst(t testing.TB, kind enums.ServiceKind, src, dst uint32, als bool, blocks uint8) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
		ALS: als, UAB: blocks, ServiceKind: uint8(kind), TargetAddress: dst, SourceAddress: src}), testCC)
}

func ackFrom(t testing.TB, r enums.AckReason, src uint32) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckOutbound, pdu.AckOutboundPDU{
		ReasonCode: byte(r), TargetAddress: msAddr, AdditionalInfo: src}), testCC)
}

func download(t testing.TB, h pdu.UDTHeader, a *trunk.Appended) []layer2.RawBurst {
	t.Helper()
	h.UDTFormat, h.PadNibble, h.UAB = a.Format, a.PadNibble, uint8(a.Blocks-1)
	if h.Opcode == 0 {
		h.Opcode = pdu.UDTOpcodeOutboundHeader
	}
	blocks := make([][12]byte, a.Blocks)
	for i := range blocks {
		copy(blocks[i][:], a.Data[12*i:min(12*i+12, trunk.MaxAppendedOctets)])
	}
	pdu.SetMBCLastBlockCRC(blocks)
	out := make([]layer2.RawBurst, 0, 1+len(blocks))
	out = append(out, bsBurst(t, withPDU(t, &pdu.DataHeader{}, (*pdu.DataHeader).SetUDT, h), testCC))
	for _, b := range blocks {
		raw, err := layer2.BuildDataBurst(b, enums.DataTypeRate12, testCC, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func (r *msRig) feedAll(bursts []layer2.RawBurst) []trunk.Output {
	all := make([]trunk.Output, 0, trunk.MaxOutputs*len(bursts))
	for _, b := range bursts {
		all = append(all, r.feed(b, &idle)...)
	}
	return all
}

func decoded(t *testing.T, o *trunk.Output) *layer2.Burst {
	t.Helper()
	if o.Kind != trunk.OutputBurst {
		t.Fatalf("output %v, want Burst", o.Kind)
	}
	b, err := layer2.NewBurstFromBytes(o.Burst)
	if err != nil {
		t.Fatal(err)
	}
	if b.SyncPattern != enums.SyncMSSourcedData || b.SlotType.ColorCode != testCC {
		t.Fatalf("burst sync %v cc %d", b.SyncPattern, b.SlotType.ColorCode)
	}
	return b
}

func uploaded(t *testing.T, out []trunk.Output) (pdu.UDTHeader, trunk.Appended) {
	t.Helper()
	b := decoded(t, &out[0])
	dh, ok := b.Data.(*pdu.DataHeader)
	if !ok || dh.FEC.Uncorrectable {
		t.Fatalf("UDT header %T", b.Data)
	}
	h, ok := dh.AsUDT()
	if !ok {
		t.Fatalf("UDT header format %v", dh.Format())
	}
	a := trunk.Appended{Format: h.UDTFormat, PadNibble: h.PadNibble, Blocks: int(h.UAB) + 1}
	if len(out) < 1+a.Blocks {
		t.Fatalf("%d outputs for %d blocks", len(out), a.Blocks)
	}
	blocks := make([][12]byte, a.Blocks)
	for i := range a.Blocks {
		blk := decoded(t, &out[1+i])
		d, ok := blk.Data.(*pdu.Rate12Data)
		if !ok {
			t.Fatalf("block %d %T", i, blk.Data)
		}
		blocks[i] = d.Data
		n := 12
		if i == a.Blocks-1 {
			n = 10
		}
		copy(a.Data[12*i:], d.Data[:n])
		if out[1+i].Due != out[0].Due+time.Duration(i+1)*60*time.Millisecond || out[1+i].Slot != out[0].Slot {
			t.Errorf("block %d due %v slot %v", i, out[1+i].Due, out[1+i].Slot)
		}
	}
	if !pdu.MBCLastBlockCRCOK(blocks) {
		t.Error("UDT CRC")
	}
	return h, a
}

func ackSent(t *testing.T, o *trunk.Output) *pdu.AckInboundPDU {
	t.Helper()
	c := sent(t, o)
	a, ok := c.AsAckInbound()
	if !ok {
		t.Fatalf("CSBK %v, want C_ACKU", c.Opcode)
	}
	return &a
}

func onTSCC(t *testing.T, cfg *trunk.MSConfig) *msRig {
	t.Helper()
	if cfg == nil {
		cfg = &trunk.MSConfig{}
	}
	cfg.Address, cfg.NoRegistration = msAddr, true
	r := newRig(t, cfg)
	wantKinds(t, "C_ALOHA", r.feed(aloha(t), &idle), trunk.OutputSite)
	return r
}

func randSent(t *testing.T, r *msRig) *pdu.RandomAccessPDU {
	t.Helper()
	out := r.feed(aloha(t), &idle)
	wantKinds(t, "C_RAND", out, trunk.OutputBurst)
	c := sent(t, &out[0])
	ra, ok := c.AsRandomAccess()
	if !ok {
		t.Fatalf("CSBK %v, want C_RAND", c.Opcode)
	}
	return &ra
}

func TestMS_ShortDataSend(t *testing.T) {
	t.Parallel()
	for _, group := range []bool{false, true} {
		r := onTSCC(t, nil)
		var a trunk.Appended
		if err := a.SetText(enums.UDTISO8, []byte("short data over the TSCC")); err != nil {
			t.Fatal(err)
		}
		if err := r.ms.SendShortData(r.now, group, peer, &a, layer3.ShortDataServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := r.ms.SendShortData(r.now, group, peer, &a, layer3.ShortDataServiceOptions{}); !errors.Is(err, trunk.ErrBusy) {
			t.Errorf("second request %v", err)
		}
		rq := randSent(t, r)
		kind := enums.ServiceKindIndividualShortData
		if group {
			kind = enums.ServiceKindTalkgroupShortData
		}
		if rq.ServiceKind != uint8(kind) || rq.TargetAddress != peer || rq.Reserved != uint8(a.Blocks-1) {
			t.Fatalf("C_RAND %+v", rq)
		}
		wantKinds(t, "C_WACKD", r.feed(ack(t, enums.AckReasonWait), &idle))
		wantKinds(t, "C_AHOY to another MS", r.feed(ahoyBurst(t, enums.ServiceKindIndividualShortData, constants.GatewaySDMI, peer, false, 1), &idle))
		out := r.feed(ahoyBurst(t, enums.ServiceKindIndividualShortData, constants.GatewaySDMI, msAddr, false, 1), &idle)
		h, got := uploaded(t, out)
		if h.Opcode != pdu.UDTOpcodeInboundHeader || h.LLIDDestination != peer || h.LLIDSource != msAddr || h.Group != group ||
			h.SupplementaryFlag || h.OptionFlag || got != a || out[0].Due != r.now+60*time.Millisecond {
			t.Fatalf("upload %+v %+v", h, got)
		}
		final := enums.AckReasonMSAccepted
		if group {
			final = enums.AckReasonMessageAccepted
		}
		out = r.feed(ack(t, final), &idle)
		wantKinds(t, "final C_ACKD", out, trunk.OutputDelivered)
		if out[0].Ack != final || r.ms.Busy() {
			t.Errorf("Delivered %v, busy %v", out[0].Ack, r.ms.Busy())
		}
	}
}

func TestMS_ShortDataRefusedAndTimeout(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	var a trunk.Appended
	if err := a.SetDigits([]byte("911")); err != nil {
		t.Fatal(err)
	}
	if err := r.ms.SendShortData(r.now, false, peer, &trunk.Appended{}, layer3.ShortDataServiceOptions{}); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("empty data %v", err)
	}
	if err := r.ms.SendShortData(r.now, false, peer, &a, layer3.ShortDataServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	randSent(t, r)
	out := r.feed(ack(t, enums.AckReasonMSAwayRefused), &idle)
	wantKinds(t, "C_NACKD", out, trunk.OutputRefused)
	if out[0].Reason != trunk.ReasonNack || out[0].Ack != enums.AckReasonMSAwayRefused {
		t.Errorf("refused %+v", out[0])
	}
	if err := r.ms.SendShortData(r.now, false, peer, &a, layer3.ShortDataServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	randSent(t, r)
	wantKinds(t, "C_QACKD", r.feed(ack(t, enums.AckReasonQueuedForBusy), &idle))
	at, ok := r.ms.Deadline()
	if !ok || at > r.now+constants.TNPTimerDefault {
		t.Fatalf("deadline %v %v, want TNP_Timer", at, ok)
	}
	end := r.now + constants.TNPTimerDefault
	for r.now < end-60*time.Millisecond {
		wantKinds(t, "C_ALOHA while waiting", r.feed(aloha(t), &idle))
	}
	out = r.feed(aloha(t), &idle)
	wantKinds(t, "TNP_Timer", out, trunk.OutputRefused)
	if out[0].Reason != trunk.ReasonTimeout {
		t.Errorf("TNP_Timer reason %v", out[0].Reason)
	}
}

func TestMS_ShortDataReceive(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	var a trunk.Appended
	if err := a.SetText(enums.UDTUTF16BE, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	out := r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer}, &a))
	wantKinds(t, "UDT to the MS", out, trunk.OutputUDT, trunk.OutputBurst)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSAccepted) || k.TargetAddress != peer || k.AdditionalInfo != msAddr {
		t.Errorf("C_ACKU %+v", k)
	}
	got := udtOf(r.ms.UDT()).Appended()
	if text, err := got.Text(nil); err != nil || string(text) != "hello" {
		t.Errorf("text %q %v", text, err)
	}
	out = r.feedAll(download(t, pdu.UDTHeader{Group: true, LLIDDestination: 9, LLIDSource: peer}, &a))
	wantKinds(t, "UDT to a talkgroup", out, trunk.OutputUDT)
	out = r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: peer, LLIDSource: msAddr}, &a))
	wantKinds(t, "UDT to another MS", out, trunk.OutputUDT)
}

func TestMS_PollShortData(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	if err := r.ms.PollShortData(r.now, peer, enums.UDTNMEA, 5); !errors.Is(err, trunk.ErrUDTLength) {
		t.Errorf("5 blocks %v", err)
	}
	if err := r.ms.PollShortData(r.now, peer, enums.UDTNMEA, 1); err != nil {
		t.Fatal(err)
	}
	rq := randSent(t, r)
	if rq.ServiceKind != uint8(enums.ServiceKindShortDataPolling) || rq.TargetAddress != peer || rq.Reserved != 0 ||
		layer3.DecodePollingServiceOptions([1]byte{rq.ServiceOptions << 1}).Format != enums.UDTNMEA {
		t.Fatalf("C_RAND %+v", rq)
	}
	wantKinds(t, "C_WACKD", r.feed(ack(t, enums.AckReasonWait), &idle))
	wantKinds(t, "C_AHOY to the polled MS", r.feed(ahoyBurst(t, enums.ServiceKindShortDataPolling, msAddr, peer, false, 0), &idle))
	var a trunk.Appended
	if err := a.SetNMEAShort(&trunk.NMEAShort{North: true, Fix: true, LatDegrees: 41}); err != nil {
		t.Fatal(err)
	}
	out := r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer}, &a))
	wantKinds(t, "polled data", out, trunk.OutputUDT, trunk.OutputDelivered, trunk.OutputBurst)
	if k := ackSent(t, &out[2]); k.ReasonCode != byte(enums.AckReasonMSAccepted) || k.TargetAddress != peer {
		t.Errorf("C_ACKU %+v", k)
	}
	if r.ms.Busy() {
		t.Error("busy after the poll")
	}
}

func TestMS_Polled(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	out := r.feed(ahoyBurst(t, enums.ServiceKindShortDataPolling, peer, msAddr, false, 0), &idle)
	wantKinds(t, "poll without data", out, trunk.OutputPolled, trunk.OutputBurst)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSNotSupported) || k.TargetAddress != peer {
		t.Errorf("C_NACKU %+v", k)
	}
	var a trunk.Appended
	if err := a.SetBinary([]byte{0xDE, 0xAD}, 16); err != nil {
		t.Fatal(err)
	}
	r.ms.SetPollResponse(&a)
	out = r.feed(ahoyBurst(t, enums.ServiceKindShortDataPolling, constants.GatewaySDMI, msAddr, false, 0), &idle)
	if out[0].Kind != trunk.OutputPolled || out[0].Grant.Source != constants.GatewaySDMI {
		t.Fatalf("outputs %v", kinds(out))
	}
	h, got := uploaded(t, out[1:])
	if h.LLIDDestination != constants.GatewaySDMI || h.Opcode != pdu.UDTOpcodeInboundHeader || got != a {
		t.Errorf("poll response %+v", h)
	}
	r.ms.SetPollResponse(nil)
	out = r.feed(ahoyBurst(t, enums.ServiceKindShortDataPolling, peer, msAddr, false, 0), &idle)
	wantKinds(t, "poll after clear", out, trunk.OutputPolled, trunk.OutputBurst)
}

func usbdPoll(t testing.TB, st pdu.ServiceType, rd uint8, target uint32) layer2.RawBurst {
	t.Helper()
	req := trunk.USBDPollRequest{ResponseDelay: rd, Target: target}
	p, err := req.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return bsBurst(t, &pdu.UnifiedSingleBlockData{ServiceType: st, Payload: p}, testCC)
}

func TestMS_USBDPoll(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	wantKinds(t, "poll of another MS", r.feed(usbdPoll(t, pdu.ServiceTypeLIP, 0, peer), &idle))
	out := r.feed(usbdPoll(t, pdu.ServiceTypeLIP, 0, msAddr), &idle)
	wantKinds(t, "poll without response", out, trunk.OutputPolled, trunk.OutputBurst)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSNotSupported) {
		t.Errorf("C_NACKU %+v", k)
	}
	lip := trunk.USBDLIPResponse{Longitude: 90, Latitude: -45, Velocity: 3}
	hash, err := pdu.AddressHash(msAddr)
	if err != nil {
		t.Fatal(err)
	}
	lip.HashedSource = hash
	p, err := lip.Encode()
	if err != nil {
		t.Fatal(err)
	}
	r.ms.SetUSBDResponse(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP, Payload: p})
	for rd := range uint8(4) {
		out = r.feed(usbdPoll(t, pdu.ServiceTypeLIP, rd, msAddr), &idle)
		wantKinds(t, "poll with response", out, trunk.OutputPolled, trunk.OutputBurst)
		if out[1].Due != r.now+time.Duration(1+rd)*60*time.Millisecond {
			t.Errorf("RD %d: due %v at %v", rd, out[1].Due, r.now)
		}
		b := decoded(t, &out[1])
		u, ok := b.Data.(*pdu.UnifiedSingleBlockData)
		if !ok || u.FEC.Uncorrectable || u.ServiceType != pdu.ServiceTypeLIP {
			t.Fatalf("response %T", b.Data)
		}
		if back := trunk.DecodeUSBDLIPResponse(u.Payload); back != lip {
			t.Errorf("LIP response %+v", back)
		}
	}
	out = r.feed(usbdPoll(t, pdu.ServiceTypeManufacturerSpecific1, 0, msAddr), &idle)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSNotSupported) {
		t.Errorf("other service type %+v", k)
	}
	r.ms.SetUSBDResponse(nil)
	out = r.feed(usbdPoll(t, pdu.ServiceTypeLIP, 0, msAddr), &idle)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSNotSupported) {
		t.Errorf("after clear %+v", k)
	}
}

func TestMS_Divert(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	var a trunk.Appended
	if err := a.SetAddresses(false, []uint32{peer}); err != nil {
		t.Fatal(err)
	}
	opts := layer3.DiversionServiceOptions{Set: true, Voice: true, ShortData: true}
	for _, c := range []struct {
		gw   uint32
		opts layer3.DiversionServiceOptions
		a    *trunk.Appended
	}{{constants.GatewayTSI, opts, &a}, {constants.GatewayMSI, layer3.DiversionServiceOptions{}, &a}, {constants.GatewayMSI, opts, nil}, {peer, opts, &a}} {
		if err := r.ms.Divert(r.now, c.gw, c.opts, c.a); err == nil {
			t.Errorf("Divert %x %+v accepted", c.gw, c.opts)
		}
	}
	if err := r.ms.Divert(r.now, constants.GatewayMSI, opts, &a); err != nil {
		t.Fatal(err)
	}
	rq := randSent(t, r)
	if rq.ServiceKind != uint8(enums.ServiceKindCallDiversion) || rq.TargetAddress != constants.GatewayMSI || rq.ProxyFlag ||
		layer3.DecodeDiversionServiceOptions([1]byte{rq.ServiceOptions << 1}) != opts {
		t.Fatalf("C_RAND %+v", rq)
	}
	out := r.feed(ahoyBurst(t, enums.ServiceKindCallDiversion, constants.GatewayDIVERTI, msAddr, false, 0), &idle)
	h, got := uploaded(t, out)
	if !h.OptionFlag || h.LLIDDestination != constants.GatewayMSI || h.SupplementaryFlag || got != a {
		t.Fatalf("diversion upload %+v", h)
	}
	out = r.feed(ackFrom(t, enums.AckReasonMessageAccepted, constants.GatewayMSI), &idle)
	wantKinds(t, "diversion accepted", out, trunk.OutputDelivered)

	if err := r.ms.Divert(r.now, constants.GatewayTSI, layer3.DiversionServiceOptions{Voice: true}, nil); err != nil {
		t.Fatal(err)
	}
	if rq := randSent(t, r); rq.TargetAddress != constants.GatewayTSI || rq.Reserved != 0 {
		t.Fatalf("clear C_RAND %+v", rq)
	}
	wantKinds(t, "clear accepted", r.feed(ack(t, enums.AckReasonMessageAccepted), &idle), trunk.OutputDelivered)

	var v6 trunk.Appended
	v6.SetIP([16]byte{0x20, 0x01}, true)
	if err := r.ms.Divert(r.now, constants.GatewayIPI, opts, &v6); err != nil {
		t.Fatal(err)
	}
	if rq := randSent(t, r); !rq.ProxyFlag || rq.Reserved != 1 {
		t.Errorf("IPv6 C_RAND %+v", rq)
	}
}

func TestMS_CallDiverted(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	if err := r.ms.Call(r.now, false, peer, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	randSent(t, r)
	var a trunk.Appended
	if err := a.SetAddresses(false, []uint32{3120077}); err != nil {
		t.Fatal(err)
	}
	out := r.feedAll(download(t, pdu.UDTHeader{SupplementaryFlag: true, LLIDDestination: msAddr, LLIDSource: constants.GatewayDIVERTI}, &a))
	wantKinds(t, "diverted address", out, trunk.OutputUDT, trunk.OutputRefused)
	if out[1].Reason != trunk.ReasonDiverted {
		t.Errorf("reason %v", out[1].Reason)
	}
	got := udtOf(r.ms.UDT()).Appended()
	if l, err := got.Addresses(); err != nil || l.Addresses[0] != 3120077 {
		t.Errorf("diverted to %+v %v", l, err)
	}
}

func TestMS_DGNA(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	var a trunk.Appended
	if err := a.SetAddresses(true, []uint32{501, 502, 503, 504}); err != nil {
		t.Fatal(err)
	}
	out := r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &a))
	wantKinds(t, "C_DGNAHD", out, trunk.OutputUDT, trunk.OutputGroups, trunk.OutputBurst)
	if k := ackSent(t, &out[2]); k.ReasonCode != byte(enums.AckReasonMSAccepted) || k.TargetAddress != peer {
		t.Errorf("C_ACKU %+v", k)
	}
	g := r.ms.Groups()
	if g.Groups[0] != 501 || g.Groups[3] != 504 || g.Groups[4] != 0 || !g.HasOneKey || g.OneKey != 501 {
		t.Fatalf("groups %+v", g)
	}
	var alias trunk.Appended
	if err := alias.SetMixed(false, 777, []byte("Fire Ops")); err != nil {
		t.Fatal(err)
	}
	r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &alias))
	if err := alias.SetMixed(true, 502, []byte("Command\x7f")); err != nil {
		t.Fatal(err)
	}
	r.feedAll(download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &alias))
	g = r.ms.Groups()
	if g.Groups[trunk.MaxDynamicGroups-1] != 777 || g.OneKey != 502 || string(r.ms.GroupAlias(nil, 777)) != "Fire Ops" ||
		string(r.ms.GroupAlias(nil, 502)) != "Command" || len(r.ms.GroupAlias(nil, 9)) != 0 {
		t.Fatalf("aliases %+v", g)
	}
	out = r.feed(tvGrant(t, 777, 5, false), &idle)
	wantKinds(t, "grant to a DGNA talkgroup", out, trunk.OutputGrant, trunk.OutputTune)

	r2 := onTSCC(t, nil)
	if err := r2.ms.AssignGroups(r2.now, peer, &trunk.Appended{Format: enums.UDTBCD, Blocks: 1}); !errors.Is(err, trunk.ErrUDTFormat) {
		t.Errorf("BCD DGNA %v", err)
	}
	if err := r2.ms.AssignGroups(r2.now, peer, &a); err != nil {
		t.Fatal(err)
	}
	rq := randSent(t, r2)
	if rq.ServiceKind != uint8(enums.ServiceKindSupplementary) || rq.TargetAddress != constants.GatewayDGNAI || rq.Reserved != 1<<2 {
		t.Fatalf("C_RAND %+v", rq)
	}
	out = r2.feed(ahoyBurst(t, enums.ServiceKindSupplementary, constants.GatewayDGNAI, msAddr, false, 1), &idle)
	h, got := uploaded(t, out)
	if h.Opcode != pdu.UDTOpcodeDGNAInboundHeader || h.LLIDDestination != peer || !h.ResponseRequested || got != a {
		t.Fatalf("C_DGNAHU %+v", h)
	}
	wantKinds(t, "final C_ACKD", r2.feed(ackFrom(t, enums.AckReasonMSAccepted, peer), &idle), trunk.OutputDelivered)
}

func dxGrant(t testing.TB, data bool, target, source uint32) layer2.RawBurst {
	t.Helper()
	if data {
		return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetDuplexPrivateDataGrant, pdu.DuplexPrivateDataGrantPDU{
			PhysicalChannel: 7, TargetAddress: target, SourceAddress: source}), testCC)
	}
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetDuplexPrivateVoiceGrant, pdu.DuplexPrivateVoiceGrantPDU{
		PhysicalChannel: 7, LogicalChannel: true, TargetAddress: target, SourceAddress: source}), testCC)
}

func TestMS_Duplex(t *testing.T) {
	t.Parallel()
	for _, data := range []bool{false, true} {
		r := onTSCC(t, nil)
		kind := enums.ServiceKindDuplexVoice
		var err error
		if data {
			kind = enums.ServiceKindDuplexPacketData
			err = r.ms.CallDuplexData(r.now, peer, layer3.DataServiceOptions{})
		} else {
			err = r.ms.CallDuplex(r.now, peer, layer3.VoiceServiceOptions{})
		}
		if err != nil {
			t.Fatal(err)
		}
		if rq := randSent(t, r); rq.ServiceKind != uint8(kind) || rq.TargetAddress != peer {
			t.Fatalf("C_RAND %+v", rq)
		}
		wantKinds(t, "grant of the called MS", r.feed(dxGrant(t, data, peer, msAddr), &idle), trunk.OutputGrant)
		out := r.feed(dxGrant(t, data, msAddr, peer), &idle)
		wantKinds(t, "grant of the calling MS", out, trunk.OutputGrant, trunk.OutputTune)
		if out[1].Tune.Channel != 7 || r.ms.Busy() || r.ms.State() != trunk.StatePayload {
			t.Fatalf("tune %+v busy %v", out[1].Tune, r.ms.Busy())
		}
		out = r.ms.EndCall(r.now, r.out[:0])
		if len(out) != constants.NMaint+2 {
			t.Fatalf("EndCall %v", kinds(out))
		}
		if mt, ok := sent(t, &out[0]).AsMaintenance(); !ok || mt.TargetAddress != peer {
			t.Errorf("duplex P_MAINT %v %+v", ok, mt)
		}
	}
	for _, no := range []bool{false, true} {
		r := onTSCC(t, &trunk.MSConfig{NoDuplex: no})
		out := r.feed(ahoyBurst(t, enums.ServiceKindDuplexVoice, peer, msAddr, false, 0), &idle)
		wantKinds(t, "duplex radio check", out, trunk.OutputAhoy, trunk.OutputBurst)
		want := enums.AckReasonMSAccepted
		if no {
			want = enums.AckReasonMSDuplexNotSupported
		}
		if k := ackSent(t, &out[1]); k.ReasonCode != byte(want) || k.TargetAddress != peer {
			t.Errorf("NoDuplex %v: %+v", no, k)
		}
	}
}

func TestMS_DuplexSlotOfOwnGrant(t *testing.T) {
	t.Parallel()
	for _, caller := range []bool{true, false} {
		for _, ts2 := range []bool{false, true} {
			r := onTSCC(t, nil)
			if caller {
				if err := r.ms.CallDuplex(r.now, peer, layer3.VoiceServiceOptions{}); err != nil {
					t.Fatal(err)
				}
				randSent(t, r)
			} else {
				wantKinds(t, "duplex C_AHOY", r.feed(ahoyBurst(t, enums.ServiceKindDuplexVoice, peer, msAddr, false, 0), &idle), trunk.OutputAhoy, trunk.OutputBurst)
			}
			want := enums.Slot1
			if ts2 {
				want = enums.Slot2
			}
			other := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetDuplexPrivateVoiceGrant, pdu.DuplexPrivateVoiceGrantPDU{
				PhysicalChannel: 7, LogicalChannel: !ts2, CallDirection: !caller, TargetAddress: peer, SourceAddress: msAddr}), testCC)
			r.feed(other, &idle)
			own := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetDuplexPrivateVoiceGrant, pdu.DuplexPrivateVoiceGrantPDU{
				PhysicalChannel: 7, LogicalChannel: ts2, CallDirection: caller, TargetAddress: msAddr, SourceAddress: peer}), testCC)
			out := r.feed(own, &idle)
			wantKinds(t, "own PV_GRANT_DX", out, trunk.OutputGrant, trunk.OutputTune)
			if out[0].Grant.Slot != want || out[1].Tune.Slot != want {
				t.Fatalf("caller %v LCN %v: grant slot %v, tune slot %v, want %v", caller, ts2, out[0].Grant.Slot, out[1].Tune.Slot, want)
			}
			out = r.ms.EndCall(r.now, r.out[:0])
			if len(out) == 0 || out[0].Kind != trunk.OutputBurst || out[0].Slot != want {
				t.Errorf("caller %v LCN %v: P_MAINT %+v", caller, ts2, out)
			}
		}
	}
}

func pvGrant(t testing.TB, target, source uint32, emergency bool) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetPrivateVoiceGrant, pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 6, Emergency: emergency, TargetAddress: target, SourceAddress: source}), testCC)
}

func TestMS_Ambient(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	if err := r.ms.CallAmbient(r.now, peer, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if rq := randSent(t, r); rq.ServiceKind != uint8(enums.ServiceKindIndividualVoice) || rq.Reserved != 0b0010 {
		t.Fatalf("ALS C_RAND %+v", rq)
	}

	r = onTSCC(t, nil)
	out := r.feed(ahoyBurst(t, enums.ServiceKindIndividualVoice, peer, msAddr, true, 0), &idle)
	if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMSNotSupported) {
		t.Errorf("ALS without support %+v", k)
	}

	for _, emergency := range []bool{false, true} {
		r = onTSCC(t, &trunk.MSConfig{Ambient: true})
		out = r.feed(ahoyBurst(t, enums.ServiceKindIndividualVoice, peer, msAddr, true, 0), &idle)
		wantKinds(t, "ALS radio check", out, trunk.OutputAhoy, trunk.OutputBurst)
		if k := ackSent(t, &out[1]); k.ReasonCode != byte(enums.AckReasonMessageAccepted) {
			t.Errorf("ALS C_ACKU %+v", k)
		}
		out = r.feed(pvGrant(t, msAddr, peer, emergency), &idle)
		wantKinds(t, "ALS grant", out, trunk.OutputGrant, trunk.OutputTune, trunk.OutputAmbient)
		life := constants.TALSDefault
		if emergency {
			life = constants.TALSEDefault
		}
		at, ok := r.ms.Deadline()
		if !ok || at > r.now+life {
			t.Fatalf("deadline %v", at)
		}
		end := r.now + life
		fill, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, testCC, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		for r.now < end-60*time.Millisecond {
			wantKinds(t, "ALS payload", r.feed(fill, &idle))
		}
		out = r.feed(fill, &idle)
		if len(out) != constants.NMaint+2 || out[constants.NMaint].Kind != trunk.OutputCallEnd {
			t.Fatalf("T_ALS end %v", kinds(out))
		}
		c := sent(t, &out[0])
		if mt, ok := c.AsMaintenance(); !ok || mt.TargetAddress != peer || mt.SourceAddress != msAddr {
			t.Errorf("T_ALS end %v %+v", c.Opcode, mt)
		}
	}

	r = onTSCC(t, &trunk.MSConfig{Ambient: true})
	r.feed(ahoyBurst(t, enums.ServiceKindIndividualVoice, peer, msAddr, true, 0), &idle)
	for end := r.now + constants.TALSRequestLifeSpanDefault; r.now <= end; {
		r.feed(aloha(t), &idle)
	}
	out = r.feed(pvGrant(t, msAddr, peer, false), &idle)
	wantKinds(t, "grant after T_ALS_REQUEST_LIFE_SPAN", out, trunk.OutputGrant, trunk.OutputTune)
}
