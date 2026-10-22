package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// MaxDiversions is the size of the call diversion table of a TSCC (ETSI TS
// 102 361-4 clause 6.6.7).
const MaxDiversions = 16

const (
	maxUDTOps  = 4
	udtRepeats = 2
)

type udtOpKind uint8

const (
	opNone udtOpKind = iota
	opShortData
	opPoll
	opGatewayPoll
	opDivert
	opDGNA
	opGatewayData
	opDuplex
	opUSBD
)

type udtStage uint8

const (
	stUpload udtStage = iota
	stAck
)

type udtOp struct {
	kind    udtOpKind
	stage   udtStage
	caller  uint32
	called  uint32
	group   bool
	until   time.Duration
	svc     enums.ServiceKind
	gateway uint32
	opts    uint8
	emerg   bool
}

type diversion struct {
	used bool
	from uint32
	opts layer3.DiversionServiceOptions
	to   Appended
}

type tsccUDT struct {
	ops  [maxUDTOps]udtOp
	divs [MaxDiversions]diversion

	inSlot enums.Slot
	in     UDT
	inWant int
	inOn   bool
	inAt   enums.Slot
	got    UDT
	usbd   pdu.UnifiedSingleBlockData

	down     [5][12]byte
	downDT   [5]enums.DataType
	downN    int
	downI    int
	downSlot enums.Slot
	downAck  bool
	downHold bool

	hdr  pdu.UDTHeader
	poll pdu.UnifiedSingleBlockData
}

// UDT returns the UDT that the TSCC received for itself with the last
// TSCCUDT, such as the answer to PollShortData. It points into the TSCC
// and changes with the next TSCCUDT. (ETSI TS 102 361-4 clause 6.6.5.1.5)
func (t *TSCC) UDT() *UDT {
	if t == nil {
		return nil
	}

	return &t.u.got
}

// USBD returns the USBD Poll Response of the last TSCCUSBD. (ETSI TS 102
// 361-4 clause 6.6.11.1)
func (t *TSCC) USBD() pdu.UnifiedSingleBlockData {
	if t == nil {
		return pdu.UnifiedSingleBlockData{}
	}

	return t.u.usbd
}

// Diverted returns the call diversion that the MS addr set, with the
// services that it diverts and the diversion address. ok is false when the
// MS diverts no call. (ETSI TS 102 361-4 clause 6.6.7.1.1)
func (t *TSCC) Diverted(addr uint32) (opts layer3.DiversionServiceOptions, to Appended, ok bool) {
	if t == nil {
		return layer3.DiversionServiceOptions{}, Appended{}, false
	}

	if i := t.divIndex(addr); i >= 0 {
		d := &t.u.divs[i]
		return d.opts, d.to, true
	}
	return opts, to, false
}

// SendShortData sends the appended data a as UDT short data from the
// gateway SDMI to the talkgroup or MS target, in the download phase of
// ETSI TS 102 361-4 clause 6.6.4.1.4. An MS acknowledges it with
// TSCCDelivered. It returns ErrBusy while another download or the table of
// UDT transactions is full, ErrUDTLength for an a with Blocks out of range
// and a *dmrgo.ConfigError for a target out of range.
func (t *TSCC) SendShortData(now time.Duration, group bool, target uint32, a *Appended) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if _, err := a.size(); err != nil {
		return err
	}
	if target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	if t.u.downN > 0 {
		return ErrBusy
	}
	op := udtOp{kind: opGatewayData, stage: stAck, caller: constants.GatewaySDMI, called: target, group: group, until: now + t.cfg.AnswerTimeout}
	if !t.addOp(op) {
		return ErrBusy
	}
	t.download(a, target, constants.GatewaySDMI, group, false, !group, pdu.UDTOpcodeOutboundHeader, enums.Slot1)
	if group {
		t.dropOp(target)
	}
	return nil
}

// AssignGroups sends the DGNA talkgroups of a from the gateway DGNAI to
// the MS target, in the UDT outbound phase of ETSI TS 102 361-4 clause
// 6.6.8.2.2. a is in the address or the mixed format. The MS acknowledges
// it with TSCCDelivered. It returns the errors of SendShortData, and
// ErrUDTFormat for another format.
func (t *TSCC) AssignGroups(now time.Duration, target uint32, a *Appended) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if a == nil {
		return errNil
	}
	if a.Format != enums.UDTAddress && a.Format != enums.UDTMixed {
		return ErrUDTFormat
	}
	if _, err := a.size(); err != nil {
		return err
	}
	if target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	if t.u.downN > 0 {
		return ErrBusy
	}
	if !t.addOp(udtOp{kind: opGatewayData, stage: stAck, caller: constants.GatewayDGNAI, called: target, until: now + t.cfg.AnswerTimeout}) {
		return ErrBusy
	}
	t.download(a, target, constants.GatewayDGNAI, false, false, true, pdu.UDTOpcodeDGNAOutboundHeader, enums.Slot1)
	return nil
}

// PollShortData polls the MS target for UDT short data in the given number
// of appended blocks with a C_AHOY from SDMI (ETSI TS 102 361-4 clause
// 6.6.5.1.5). TSCCUDT reports the answer, which UDT returns, and
// TSCCRefused a C_NACKU. It returns ErrBusy when the table of UDT
// transactions is full, ErrUDTLength for blocks out of range and a
// *dmrgo.ConfigError for a target out of range.
func (t *TSCC) PollShortData(now time.Duration, target uint32, blocks int) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if blocks < 1 || blocks > 4 {
		return ErrUDTLength
	}
	if target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	if !t.addOp(udtOp{kind: opGatewayPoll, stage: stAck, caller: constants.GatewaySDMI, called: target, until: now + t.cfg.AnswerTimeout}) {
		return ErrBusy
	}
	t.sendAhoy(pdu.AhoyPDU{UAB: uab(blocks), ServiceKind: uint8(enums.ServiceKindShortDataPolling), TargetAddress: target,
		SourceAddress: constants.GatewaySDMI}, 0, true)
	return nil
}

// PollUSBD sends a USBD Poll Request of the Service Type st to the MS
// target, which answers after the Response Delay rd, 0 to 3 (ETSI TS 102
// 361-4 clause 6.6.11.1). TSCCUSBD reports the Poll Response, which USBD
// returns, and TSCCRefused a C_NACKU or no answer. It returns ErrBusy
// while another download runs or the table of UDT transactions is full,
// and a *dmrgo.ConfigError for a target or rd out of range.
func (t *TSCC) PollUSBD(now time.Duration, target uint32, st pdu.ServiceType, rd uint8) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	r := USBDPollRequest{ResponseDelay: rd, Target: target}
	p, err := r.Encode()
	if err != nil || target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	t.u.poll = pdu.UnifiedSingleBlockData{ServiceType: st, Payload: p}
	b, err := t.u.poll.Encode()
	if err != nil {
		return errTarget
	}
	if t.u.downN > 0 || !t.addOp(udtOp{kind: opUSBD, stage: stAck, caller: constants.GatewayTSI, called: target, until: now + t.cfg.AnswerTimeout}) {
		return ErrBusy
	}
	t.u.down[0], t.u.downDT[0], t.u.downN, t.u.downI, t.u.downSlot, t.u.downAck = b, enums.DataTypeUnifiedSingleBlock, 1, 0, enums.SlotUnknown, rd == 0
	return nil
}

func (t *TSCC) addOp(op udtOp) bool {
	for i := range t.u.ops {
		if t.u.ops[i].kind == opNone {
			t.u.ops[i] = op
			return true
		}
	}
	return false
}

func (t *TSCC) dropOp(called uint32) {
	for i := range t.u.ops {
		if t.u.ops[i].kind != opNone && t.u.ops[i].called == called {
			t.u.ops[i] = udtOp{}
		}
	}
}

func (t *TSCC) divIndex(addr uint32) int {
	for i := range t.u.divs {
		if t.u.divs[i].used && t.u.divs[i].from == addr {
			return i
		}
	}
	return -1
}

func diverts(o *layer3.DiversionServiceOptions, k enums.ServiceKind) bool {
	switch k {
	case enums.ServiceKindIndividualVoice, enums.ServiceKindDuplexVoice:
		return o.Voice
	case enums.ServiceKindIndividualPacketData, enums.ServiceKindDuplexPacketData:
		return o.PacketData
	case enums.ServiceKindIndividualShortData:
		return o.ShortData
	case enums.ServiceKindStatusTransport:
		return o.Status
	case enums.ServiceKindTalkgroupVoice, enums.ServiceKindTalkgroupPacketData, enums.ServiceKindTalkgroupShortData, enums.ServiceKindShortDataPolling,
		enums.ServiceKindCallDiversion, enums.ServiceKindCallAnswer, enums.ServiceKindSupplementary, enums.ServiceKindRegistration, enums.ServiceKindCancelCall:
	}
	return false
}

func (t *TSCC) udtRandom(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) ([]TSCCOutput, bool) {
	src, kind := r.SourceAddress, enums.ServiceKind(r.ServiceKind)
	switch kind {
	case enums.ServiceKindIndividualShortData, enums.ServiceKindTalkgroupShortData, enums.ServiceKindShortDataPolling, enums.ServiceKindCallDiversion,
		enums.ServiceKindDuplexVoice, enums.ServiceKindDuplexPacketData:
	case enums.ServiceKindSupplementary:
		if r.TargetAddress != constants.GatewayDGNAI {
			return out, false
		}
	case enums.ServiceKindIndividualVoice, enums.ServiceKindIndividualPacketData:
		if i := t.divIndex(r.TargetAddress); i >= 0 && diverts(&t.u.divs[i].opts, kind) {
			return t.divertedCall(src, i, out), true
		}
		return out, false
	default:
		return out, false
	}
	if t.cfg.Reg && t.regIndex(src) < 0 {
		return t.refuse(src, enums.AckReasonMSNotRegistered, out), true
	}
	blocks := r.Reserved&0b11 + 1
	switch kind {
	case enums.ServiceKindCallDiversion:
		return t.divertRequest(now, r, out), true
	case enums.ServiceKindSupplementary:
		if !t.addOp(udtOp{kind: opDGNA, caller: src, until: now + constants.TNPTimerDefault}) {
			return t.refuse(src, enums.AckReasonSysBusyRefused, out), true
		}
		t.sendAhoy(pdu.AhoyPDU{UAB: r.Reserved >> 2 & 0b11, ServiceKind: uint8(kind), TargetAddress: src, SourceAddress: constants.GatewayDGNAI}, 0, true)
		return out, true
	case enums.ServiceKindTalkgroupShortData:
		if !t.addOp(udtOp{kind: opShortData, caller: src, called: r.TargetAddress, group: true, until: now + constants.TNPTimerDefault}) {
			return t.refuse(src, enums.AckReasonSysBusyRefused, out), true
		}
		t.sendAhoy(pdu.AhoyPDU{UAB: blocks - 1, ServiceKind: uint8(enums.ServiceKindIndividualShortData), TargetAddress: src,
			SourceAddress: constants.GatewaySDMI}, 0, true)
		return out, true
	case enums.ServiceKindIndividualShortData, enums.ServiceKindShortDataPolling, enums.ServiceKindDuplexVoice, enums.ServiceKindDuplexPacketData:
	default:
		return out, false
	}
	target := r.TargetAddress
	if target >= constants.GatewayPSTNI {
		return t.refuse(src, enums.AckReasonNotSupported, out), true
	}
	if i := t.divIndex(target); i >= 0 {
		if kind == enums.ServiceKindShortDataPolling {
			return t.refuse(src, enums.AckReasonDivCauseFail, out), true
		}
		if diverts(&t.u.divs[i].opts, kind) {
			return t.divertedCall(src, i, out), true
		}
	}
	if t.cfg.Reg && t.regIndex(target) < 0 {
		return t.refuse(src, enums.AckReasonNoregMSAwayRefused, out), true
	}
	op := udtOp{caller: src, called: target, until: now + constants.TNPTimerDefault, svc: kind, opts: r.ServiceOptions}
	a := pdu.AhoyPDU{ServiceKind: uint8(kind), TargetAddress: src, SourceAddress: constants.GatewaySDMI, UAB: blocks - 1}
	switch kind {
	case enums.ServiceKindIndividualShortData:
		op.kind = opShortData
	case enums.ServiceKindShortDataPolling:
		op.kind, op.stage = opPoll, stAck
		a = pdu.AhoyPDU{ServiceOptsMirror: r.ServiceOptions, UAB: blocks - 1, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: src}
	default:
		op.kind, op.stage, op.until = opDuplex, stAck, now+t.cfg.AnswerTimeout
		op.emerg = r.ServiceOptions&0b1000000 != 0
		a = pdu.AhoyPDU{ServiceOptsMirror: r.ServiceOptions, ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: src}
	}
	if !t.addOp(op) {
		return t.refuse(src, enums.AckReasonSysBusyRefused, out), true
	}
	if op.kind != opShortData {
		t.sendAck(src, enums.AckReasonWait)
	}
	t.sendAhoy(a, 0, true)
	return out, true
}

func (t *TSCC) divertRequest(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) []TSCCOutput {
	src := r.SourceAddress
	o := layer3.DecodeDiversionServiceOptions([1]byte{r.ServiceOptions << 1})
	switch r.TargetAddress {
	case constants.GatewayTSI, constants.GatewayDIVERTI:
		if o.Set {
			return t.refuse(src, enums.AckReasonNotSupported, out)
		}
		for i := range t.u.divs {
			d := &t.u.divs[i]
			if !d.used || r.TargetAddress == constants.GatewayTSI && d.from != src {
				continue
			}
			if r.TargetAddress == constants.GatewayDIVERTI {
				l, err := d.to.Addresses()
				if err != nil || l.Addresses[0] != src {
					continue
				}
			}
			d.opts.Voice = d.opts.Voice && !o.Voice
			d.opts.PacketData = d.opts.PacketData && !o.PacketData
			d.opts.ShortData = d.opts.ShortData && !o.ShortData
			d.opts.Status = d.opts.Status && !o.Status
			if !d.opts.Voice && !d.opts.PacketData && !d.opts.ShortData && !d.opts.Status {
				d.used = false
			}
			out = append(out, TSCCOutput{Kind: TSCCDiversion, Address: d.from})
		}
		t.ackFrom(src, enums.AckReasonMessageAccepted, 0, r.TargetAddress)
		return out
	case constants.GatewayMSI, constants.GatewayGPI:
	default:
		return t.refuse(src, enums.AckReasonTransientSysRefused, out)
	}
	if !o.Set || !t.addOp(udtOp{kind: opDivert, caller: src, gateway: r.TargetAddress, opts: r.ServiceOptions, until: now + constants.TNPTimerDefault}) {
		return t.refuse(src, enums.AckReasonSysBusyRefused, out)
	}
	t.sendAhoy(pdu.AhoyPDU{ServiceKind: uint8(enums.ServiceKindCallDiversion), TargetAddress: src, SourceAddress: constants.GatewayDIVERTI}, 0, true)
	return out
}

func (t *TSCC) divertedCall(caller uint32, i int, out []TSCCOutput) []TSCCOutput {
	if t.u.downN > 0 {
		return t.refuse(caller, enums.AckReasonSysBusyRefused, out)
	}
	t.download(&t.u.divs[i].to, caller, constants.GatewayDIVERTI, false, true, false, pdu.UDTOpcodeOutboundHeader, t.u.inSlot)
	return out
}

func (t *TSCC) download(a *Appended, to, from uint32, group, supp, ack bool, op pdu.UDTOpcode, slot enums.Slot) {
	u := &t.u
	u.hdr = pdu.UDTHeader{
		Group: group, ResponseRequested: ack, SAP: enums.SAPUnifiedDataTransport, UDTFormat: a.Format, LLIDDestination: to,
		LLIDSource: from, PadNibble: a.PadNibble, UAB: uab(a.Blocks), SupplementaryFlag: supp, Opcode: op,
	}
	var dh pdu.DataHeader
	if dh.SetUDT(u.hdr) != nil {
		return
	}
	p, err := dh.Encode()
	if err != nil {
		return
	}
	u.down[0], u.downDT[0] = p, enums.DataTypeDataHeader
	for i := range a.Blocks {
		u.down[1+i] = [12]byte{}
		copy(u.down[1+i][:], a.Data[12*i:min(12*i+12, MaxAppendedOctets)])
		u.downDT[1+i] = enums.DataTypeRate12
	}
	pdu.SetMBCLastBlockCRC(u.down[1 : 1+a.Blocks])
	if slot == enums.SlotUnknown {
		slot = enums.Slot1
	}
	u.downN, u.downI, u.downSlot, u.downAck, u.downHold = 1+a.Blocks, 0, slot, ack, false
}

func (t *TSCC) udtNext(now time.Duration) (p [12]byte, dt enums.DataType, reply, ok bool) {
	u := &t.u
	if u.downN == 0 || u.downSlot != enums.SlotUnknown && u.downSlot != t.slot ||
		u.downI == 0 && !u.hdr.Group && !t.awake(u.hdr.LLIDDestination, now) {
		return p, dt, false, false
	}
	p, dt = u.down[u.downI], u.downDT[u.downI]
	u.downI++
	last := u.downI == u.downN
	if last {
		u.downN = 0
	}
	return p, dt, last && u.downAck, true
}

func (t *TSCC) udtInbound(now time.Duration, s enums.Slot, out []TSCCOutput) ([]TSCCOutput, bool) {
	t.u.inSlot = s
	u := &t.u
	switch d := t.b.Data.(type) {
	case *pdu.DataHeader:
		h, ok := d.AsUDT()
		u.inOn = false
		if !ok || h.Opcode != pdu.UDTOpcodeInboundHeader && h.Opcode != pdu.UDTOpcodeDGNAInboundHeader {
			return out, true
		}
		u.in = UDT{Header: h}
		u.inWant, u.inOn, u.inAt = int(h.UAB)+1, true, s
		t.wake(h.LLIDSource, now)
		return out, true
	case *pdu.Rate12Data:
		if !u.inOn || s != u.inAt {
			return out, true
		}
		in := &u.in
		in.blocks[in.n] = d.Data
		copy(in.Data[in.n*12:], d.Data[:])
		in.n++
		if in.n < u.inWant {
			return out, true
		}
		u.inOn = false
		in.Len = in.n*12 - 2
		in.CRCOK = pdu.MBCLastBlockCRCOK(in.blocks[:in.n])
		return t.uploaded(now, out), true
	case *pdu.UnifiedSingleBlockData:
		u.usbd = *d
		for i := range u.ops {
			if u.ops[i].kind == opUSBD {
				addr := u.ops[i].called
				u.ops[i] = udtOp{}
				return append(out, TSCCOutput{Kind: TSCCUSBD, Address: addr}), true
			}
		}
		return append(out, TSCCOutput{Kind: TSCCUSBD}), true
	}
	return out, false
}

func (t *TSCC) findOp(kind udtOpKind, stage udtStage, caller uint32) int {
	for i := range t.u.ops {
		o := &t.u.ops[i]
		if o.kind == kind && o.stage == stage && (caller == 0 || o.caller == caller) {
			return i
		}
	}
	return -1
}

func (t *TSCC) uploaded(now time.Duration, out []TSCCOutput) []TSCCOutput {
	in := &t.u.in
	h := &in.Header
	src := h.LLIDSource
	if h.Opcode == pdu.UDTOpcodeInboundHeader && (h.LLIDDestination == constants.GatewayIPI || h.LLIDDestination == constants.GatewaySUPLI) {
		return t.svcUploaded(now, out)
	}
	if h.Opcode == pdu.UDTOpcodeDGNAInboundHeader {
		i := t.findOp(opDGNA, stUpload, src)
		if i < 0 {
			return out
		}
		return t.relayUpload(now, i, h.LLIDDestination, false, pdu.UDTOpcodeDGNAOutboundHeader, out)
	}
	if h.OptionFlag {
		return t.divertUploaded(src, out)
	}
	if i := t.findOp(opShortData, stUpload, src); i >= 0 {
		o := &t.u.ops[i]
		return t.relayUpload(now, i, o.called, o.group, pdu.UDTOpcodeOutboundHeader, out)
	}
	for i := range t.u.ops {
		o := &t.u.ops[i]
		if o.called != src || o.stage != stAck || o.kind != opPoll && o.kind != opGatewayPoll {
			continue
		}
		if !in.CRCOK {
			return out
		}
		if o.kind == opGatewayPoll {
			t.u.got = *in
			*o = udtOp{}
			return append(out, TSCCOutput{Kind: TSCCUDT, Address: src})
		}
		if t.u.downN > 0 {
			return out
		}
		a := in.Appended()
		o.until = now + t.cfg.AnswerTimeout
		t.download(&a, o.caller, src, false, false, true, pdu.UDTOpcodeOutboundHeader, t.u.inSlot)
		return out
	}
	return out
}

func (t *TSCC) relayUpload(now time.Duration, i int, to uint32, group bool, op pdu.UDTOpcode, out []TSCCOutput) []TSCCOutput {
	o := &t.u.ops[i]
	if !t.u.in.CRCOK {
		caller := o.caller
		*o = udtOp{}
		return t.refuse(caller, enums.AckReasonUDTUploadCRCError, out)
	}
	if t.u.downN > 0 {
		return out
	}
	a := t.u.in.Appended()
	t.download(&a, to, o.caller, group, false, !group, op, t.u.inSlot)
	o.called, o.stage, o.until = to, stAck, now+t.cfg.AnswerTimeout
	if !group {
		return out
	}
	caller := o.caller
	*o = udtOp{}
	for range udtRepeats {
		t.ackFrom(caller, enums.AckReasonMessageAccepted, 0, to)
	}
	return append(out, TSCCOutput{Kind: TSCCDelivered, Address: caller, Ack: enums.AckReasonMessageAccepted, Call: Grant{Group: true, Target: to, Source: caller}})
}

func (t *TSCC) divertUploaded(src uint32, out []TSCCOutput) []TSCCOutput {
	i := t.findOp(opDivert, stUpload, src)
	if i < 0 {
		return out
	}
	o := t.u.ops[i]
	t.u.ops[i] = udtOp{}
	a := t.u.in.Appended()
	if _, err := a.Addresses(); err != nil || !t.u.in.CRCOK {
		return t.refuse(src, enums.AckReasonTransientSysRefused, out)
	}
	k := t.divIndex(src)
	if k < 0 {
		for j := range t.u.divs {
			if !t.u.divs[j].used {
				k = j
				break
			}
		}
	}
	if k < 0 {
		return t.refuse(src, enums.AckReasonSysBusyRefused, out)
	}
	t.u.divs[k] = diversion{used: true, from: src, opts: layer3.DecodeDiversionServiceOptions([1]byte{o.opts << 1}), to: a}
	t.ackFrom(src, enums.AckReasonMessageAccepted, 0, o.gateway)
	return append(out, TSCCOutput{Kind: TSCCDiversion, Address: src})
}

func (t *TSCC) udtAck(now time.Duration, a *pdu.AckInboundPDU, out []TSCCOutput) ([]TSCCOutput, bool) {
	from := a.AdditionalInfo
	r := enums.AckReason(a.ReasonCode)
	for i := range t.u.ops {
		o := &t.u.ops[i]
		if o.kind == opNone || o.stage != stAck || o.called != from && (o.kind != opPoll || o.caller != from) {
			continue
		}
		op := *o
		*o = udtOp{}
		if r.Type() == enums.AckNACK {
			if op.caller >= constants.GatewayPSTNI {
				return append(out, TSCCOutput{Kind: TSCCRefused, Address: from, Ack: r}), true
			}
			return t.refuse(op.caller, r, out), true
		}
		switch op.kind {
		case opDuplex:
			return t.duplexGrant(now, &op, out), true
		case opPoll:
			if from != op.caller {
				*o = op
				return out, true
			}
			return append(out, TSCCOutput{Kind: TSCCDelivered, Address: op.caller, Ack: r, Call: Grant{Target: op.called, Source: op.caller}}), true
		case opGatewayData:
			return append(out, TSCCOutput{Kind: TSCCDelivered, Address: op.caller, Ack: r, Call: Grant{Target: op.called, Source: op.caller}}), true
		case opNone, opShortData, opGatewayPoll, opDivert, opDGNA, opUSBD:
		}
		for range udtRepeats {
			t.ackFrom(op.caller, enums.AckReasonMSAccepted, 0, op.called)
		}
		return append(out, TSCCOutput{Kind: TSCCDelivered, Address: op.caller, Ack: enums.AckReasonMSAccepted, Call: Grant{Target: op.called, Source: op.caller}}), true
	}
	return out, false
}

func (t *TSCC) duplexGrant(now time.Duration, op *udtOp, out []TSCCOutput) []TSCCOutput {
	for c := range t.cfg.PayloadLen {
		if t.calls[2*c].active || t.calls[2*c+1].active {
			continue
		}
		ch := t.cfg.Payload[c]
		data := op.svc == enums.ServiceKindDuplexPacketData
		g := Grant{Service: op.svc, Target: op.caller, Source: op.called, Channel: ch, Slot: enums.Slot1, ColorCode: t.cfg.ColorCode, HasColorCode: true,
			Emergency: op.emerg, Opcode: pdu.CSBKDuplexPrivateVoiceGrant}
		if data {
			g.Opcode = pdu.CSBKDuplexPrivateDataGrant
		}
		t.calls[2*c] = callSlot{active: true, grant: g, last: now}
		g2 := g
		g2.Target, g2.Source, g2.Slot = op.called, op.caller, enums.Slot2
		t.calls[2*c+1] = callSlot{active: true, grant: g2, last: now}
		for range t.cfg.GrantRepeats {
			t.duplexSend(&g2, false)
			t.duplexSend(&g, true)
		}
		return append(out, TSCCOutput{Kind: TSCCCallStart, Call: g})
	}
	return t.refuse(op.caller, enums.AckReasonSysBusyRefused, out)
}

func (t *TSCC) duplexSend(g *Grant, caller bool) {
	ts2 := g.Slot == enums.Slot2
	t.cs = pdu.CSBK{LastBlock: true}
	var err error
	if g.Service == enums.ServiceKindDuplexPacketData {
		err = t.cs.SetDuplexPrivateDataGrant(pdu.DuplexPrivateDataGrantPDU{PhysicalChannel: g.Channel, LogicalChannel: ts2, Emergency: g.Emergency, CallDirection: caller,
			TargetAddress: g.Target, SourceAddress: g.Source})
	} else {
		err = t.cs.SetDuplexPrivateVoiceGrant(pdu.DuplexPrivateVoiceGrantPDU{PhysicalChannel: g.Channel, LogicalChannel: ts2, Emergency: g.Emergency, CallDirection: caller,
			TargetAddress: g.Target, SourceAddress: g.Source})
	}
	if err == nil {
		t.enqueue(false)
	}
}

func duplex(g *Grant) bool {
	return g.Service == enums.ServiceKindDuplexVoice || g.Service == enums.ServiceKindDuplexPacketData
}

func (t *TSCC) relaySlot(i int) int {
	if duplex(&t.calls[i].grant) {
		return i ^ 1
	}
	return i
}

func (t *TSCC) udtTimers(now time.Duration, out []TSCCOutput) []TSCCOutput {
	for i := range t.u.ops {
		o := &t.u.ops[i]
		if o.kind == opNone || now < o.until {
			continue
		}
		op := *o
		*o = udtOp{}
		r := enums.AckReasonMSAwayRefused
		if op.stage == stUpload {
			r = enums.AckReasonRefusedUnknownTS
		}
		if op.caller >= constants.GatewayPSTNI {
			out = append(out, TSCCOutput{Kind: TSCCRefused, Address: op.called, Ack: r})
			continue
		}
		out = t.refuse(op.caller, r, out)
	}
	for i := 0; i < 2*t.cfg.PayloadLen; i += 2 {
		a, b := &t.calls[i], &t.calls[i+1]
		if !a.active || !b.active || !duplex(&a.grant) {
			continue
		}
		switch {
		case a.clears > 0 && b.clears == 0:
			out = t.startClear(i+1, a.endReason, out)
		case b.clears > 0 && a.clears == 0:
			out = t.startClear(i, b.endReason, out)
		}
	}
	return out
}
