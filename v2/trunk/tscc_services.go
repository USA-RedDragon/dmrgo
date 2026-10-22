package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	maxStuns    = 4
	maxStatus   = 4
	maxSupl     = 2
	maxIP       = 2
	rcRepeat    = 360 * time.Millisecond
	rcSends     = 6
	rcIdle      = 3 * frame
	protectSent = 4
	statusSends = 2
	svcEvents   = 2
)

type psRec struct {
	addr     uint32
	rq       uint8
	offset   uint8
	awakeEnd time.Duration
}

type stunOp struct {
	used  bool
	addr  uint32
	act   StunAction
	until time.Duration
}

type statusOp struct {
	used   bool
	caller uint32
	from   uint32
	called uint32
	status Status
	until  time.Duration
}

type rcOp struct {
	on     bool
	cmd    enums.RCCommand
	next   time.Duration
	left   int
	by     uint32
	end    bool
	reason Reason
}

type suplOp struct {
	used     bool
	download bool
	rand     pdu.RandomAccessPDU
	until    time.Duration
}

type ipOp struct {
	used   bool
	addr   uint32
	remove bool
	offset uint8
	until  time.Duration
}

type tsccSvc struct {
	ps      []psRec
	nPS     int
	stuns   [maxStuns]stunOp
	status  [maxStatus]statusOp
	rc      [2 * MaxPayloadChannels]rcOp
	start   [2 * MaxPayloadChannels]time.Duration
	seen    [2 * MaxPayloadChannels]bool
	vIdx    [2 * MaxPayloadChannels]uint8
	relayF  [2 * MaxPayloadChannels]bool
	protect [2 * MaxPayloadChannels]int
	protTo  [2 * MaxPayloadChannels]uint32
	supl    [maxSupl]suplOp
	ip      [maxIP]ipOp
	cs      pdu.CSBK
	rcPDU   pdu.ReverseChannel
	buf     layer2.RawBurst
}

func (t *TSCC) initSvc() {
	if t.cfg.Awake <= 0 {
		t.cfg.Awake = constants.TAwakeDefault
	}
	if t.cfg.PowerSave {
		t.svc.ps = make([]psRec, t.cfg.MaxRegistrations)
	}
}

func (t *TSCC) push(p [12]byte, addr uint32, reply bool) {
	if t.qLen == queueLen {
		return
	}
	t.queue[(t.qHead+t.qLen)%queueLen] = queued{p: p, reply: reply, addr: addr}
	t.qLen++
}

func (t *TSCC) enqueueTo(addr uint32, reply bool) {
	p, err := t.cs.Encode()
	if err != nil {
		return
	}
	t.push(p, addr, reply)
}

func (t *TSCC) sendCSBK(addr uint32, reply bool) {
	p, err := t.svc.cs.Encode()
	if err != nil {
		return
	}
	t.push(p, addr, reply)
}

func (t *TSCC) dequeue(now time.Duration) (p [12]byte, reply, ok bool) {
	for i := range t.qLen {
		q := t.queue[(t.qHead+i)%queueLen]
		if q.addr != 0 && !t.awake(q.addr, now) {
			continue
		}
		for j := i; j > 0; j-- {
			t.queue[(t.qHead+j)%queueLen] = t.queue[(t.qHead+j-1)%queueLen]
		}
		t.qHead = (t.qHead + 1) % queueLen
		t.qLen--
		return q.p, q.reply, true
	}
	return p, false, false
}

func (t *TSCC) psFind(addr uint32) int {
	for i := range t.svc.nPS {
		if t.svc.ps[i].addr == addr {
			return i
		}
	}
	return -1
}

func (t *TSCC) awake(addr uint32, now time.Duration) bool {
	i := t.psFind(addr)
	if i < 0 {
		return true
	}
	r := &t.svc.ps[i]
	if now < r.awakeEnd {
		return true
	}
	mask := uint16(1)<<r.rq - 1
	if (t.counter>>2)&mask != uint16(r.offset) {
		return false
	}
	r.awakeEnd = now + t.cfg.Awake - counterBlock
	return true
}

func (t *TSCC) wake(addr uint32, now time.Duration) {
	if i := t.psFind(addr); i >= 0 {
		t.svc.ps[i].awakeEnd = now + t.cfg.Awake - counterBlock
	}
}

func (t *TSCC) psSet(now time.Duration, addr uint32, rq uint8) uint8 {
	s := &t.svc
	i := t.psFind(addr)
	if !t.cfg.PowerSave || rq == 0 {
		if i >= 0 {
			s.nPS--
			s.ps[i] = s.ps[s.nPS]
		}
		return 0
	}
	if i < 0 {
		if s.nPS == len(s.ps) {
			return 0
		}
		i = s.nPS
		s.nPS++
	}
	off := 1 + uint8(addr%(uint32(1)<<rq-1)&0x7F)
	s.ps[i] = psRec{addr: addr, rq: rq, offset: off, awakeEnd: now + t.cfg.Awake - counterBlock}
	return off
}

func (t *TSCC) ackFrom(addr uint32, r enums.AckReason, info uint8, from uint32) {
	s := &t.svc
	s.cs = pdu.CSBK{LastBlock: true}
	if s.cs.SetAckOutbound(pdu.AckOutboundPDU{ResponseInfo: info, ReasonCode: byte(r), TargetAddress: addr, AdditionalInfo: from}) == nil {
		t.sendCSBK(0, false)
	}
}

func (t *TSCC) refuseFrom(addr uint32, r enums.AckReason, from uint32, out []TSCCOutput) []TSCCOutput {
	t.ackFrom(addr, r, 0, from)
	return append(out, TSCCOutput{Kind: TSCCRefused, Address: addr, Ack: r})
}

func (t *TSCC) sendAhoy(a pdu.AhoyPDU, addr uint32, reply bool) {
	s := &t.svc
	s.cs = pdu.CSBK{LastBlock: true}
	if s.cs.SetAhoy(a) == nil {
		t.sendCSBK(addr, reply)
	}
}

func (t *TSCC) registration(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) []TSCCOutput {
	src := r.SourceAddress
	o := layer3.DecodeRegistrationServiceOptions([1]byte{r.ServiceOptions << 1})
	if o.IPInform {
		return t.ipAdvice(now, r, &o, out)
	}
	if !o.Register {
		if i := t.regIndex(src); i >= 0 {
			t.nRegs--
			t.regs[i] = t.regs[t.nRegs]
			out = append(out, TSCCOutput{Kind: TSCCDeregistered, Address: src})
		}
		t.psSet(now, src, 0)
		t.sendAck(src, enums.AckReasonRegAccepted)
		return out
	}
	if t.cfg.Authenticate {
		return t.challenge(now, src, r.ServiceOptions, o.PowerSave, out)
	}
	return t.admit(now, src, o.PowerSave, out)
}

func (t *TSCC) admit(now time.Duration, src uint32, ps uint8, out []TSCCOutput) []TSCCOutput {
	out, ok := t.register(src, out)
	if !ok {
		return t.refuse(src, enums.AckReasonRegRefused, out)
	}
	t.ackFrom(src, enums.AckReasonRegAccepted, t.psSet(now, src, ps), constants.GatewayREGI)
	return out
}

func (t *TSCC) register(src uint32, out []TSCCOutput) ([]TSCCOutput, bool) {
	if t.regIndex(src) >= 0 {
		return out, true
	}
	if t.nRegs == len(t.regs) {
		return out, false
	}
	t.regs[t.nRegs] = src
	t.nRegs++
	return append(out, TSCCOutput{Kind: TSCCRegistered, Address: src}), true
}

// MassRegister sends a C_BCAST MassReg, which asks the MSs whose Mask least
// significant address bits match addr to register again within the
// TReg_Window of window. A window of 0 cancels the mass registration, and
// a Mask of 24 asks the MS addr alone. It returns a *dmrgo.ConfigError for a
// window over 15, a mask over 24 or an addr over 24 bits. (ETSI TS 102
// 361-4 clause 6.4.5)
func (t *TSCC) MassRegister(window, mask uint8, addr uint32) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if window > 15 || mask > 24 || addr > 0xFFFFFF {
		return errTarget
	}
	s := &t.svc
	s.cs = pdu.CSBK{LastBlock: true}
	if s.cs.SetBroadcast(pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementMassReg, BroadcastParms1: uint16(window)<<5 | uint16(mask),
		Reg: t.cfg.Reg, Backoff: uint8(t.cfg.Backoff), SysIdentCode: t.code, BroadcastParms2: addr}) == nil {
		t.sendCSBK(0, false)
	}
	return nil
}

// Stun asks the MS addr to stop, or with revive to give back, its user
// services, with a C_AHOY from STUNI. TSCCStun reports the answer. It
// returns ErrBusy when a stun or kill for addr runs or too many run, and a
// *dmrgo.ConfigError for an addr out of range. (ETSI TS 102 361-4 clause 6.4.9)
func (t *TSCC) Stun(now time.Duration, addr uint32, revive bool) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	act := StunStun
	if revive {
		act = StunRevive
	}
	return t.stun(now, addr, act)
}

// Kill asks the MS addr to disable itself for good, with a C_AHOY from
// KILLI, as Stun does. The MS authenticates the TSCC first, so it needs
// TSCCConfig.Auth (ETSI TS 102 361-4 clause 6.4.10).
func (t *TSCC) Kill(now time.Duration, addr uint32) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	return t.stun(now, addr, StunKill)
}

func (t *TSCC) stun(now time.Duration, addr uint32, act StunAction) error {
	if addr == 0 || addr > maxAddress {
		return errTarget
	}
	s := &t.svc
	free := -1
	for i := range s.stuns {
		switch {
		case s.stuns[i].used && s.stuns[i].addr == addr:
			return ErrBusy
		case !s.stuns[i].used && free < 0:
			free = i
		}
	}
	if free < 0 {
		return ErrBusy
	}
	s.stuns[free] = stunOp{used: true, addr: addr, act: act, until: now + t.cfg.AnswerTimeout}
	gw := uint32(constants.GatewaySTUNI)
	if act == StunKill {
		gw = constants.GatewayKILLI
	}
	t.sendAhoy(pdu.AhoyPDU{ServiceKindFlag: act == StunRevive, ServiceKind: uint8(enums.ServiceKindSupplementary), TargetAddress: addr, SourceAddress: gw}, addr, true)
	return nil
}

func (t *TSCC) stunIndex(addr uint32) int {
	for i := range t.svc.stuns {
		if t.svc.stuns[i].used && t.svc.stuns[i].addr == addr {
			return i
		}
	}
	return -1
}

func (t *TSCC) ackvit(now time.Duration, a *pdu.AckvitationPDU, out []TSCCOutput) []TSCCOutput {
	src := a.SourceAddress
	t.wake(src, now)
	i := t.stunIndex(src)
	if i < 0 || enums.ServiceKind(a.ServiceKind) != enums.ServiceKindSupplementary {
		return out
	}
	op := &t.svc.stuns[i]
	resp, ok := t.authResponse(src, a.TargetAddress)
	if !ok {
		op.used = false
		return append(out, TSCCOutput{Kind: TSCCStun, Address: src, Stun: op.act, Reason: ReasonAuthentication})
	}
	op.until = now + t.cfg.AnswerTimeout
	t.ackFrom(src, enums.AckReasonAuthResponseTS, 0, resp)
	return out
}

func (t *TSCC) authResponse(addr, challenge uint32) (uint32, bool) {
	if t.cfg.Auth == nil {
		return 0, false
	}
	r, ok := t.cfg.Auth.Response(addr, challenge)
	return r & 0xFFFFFF, ok
}

func (t *TSCC) svcAnswer(now time.Duration, a *pdu.AckInboundPDU, out []TSCCOutput) ([]TSCCOutput, bool) {
	s := &t.svc
	src, r := a.AdditionalInfo, enums.AckReason(a.ReasonCode)
	t.wake(src, now)
	switch a.TargetAddress {
	case constants.GatewaySTUNI, constants.GatewayKILLI:
		i := t.stunIndex(src)
		if i < 0 {
			return out, true
		}
		op := &s.stuns[i]
		op.used = false
		o := TSCCOutput{Kind: TSCCStun, Address: src, Stun: op.act, Ack: r}
		if r == enums.AckReasonRecipientRefused {
			o.Reason = ReasonAuthentication
		}
		return append(out, o), true
	case constants.GatewaySUPLI:
		for i := range s.supl {
			op := &s.supl[i]
			if op.used && op.download && op.rand.TargetAddress == src {
				if r.Type() == enums.AckNACK {
					op.used = false
					return t.refuse(op.rand.SourceAddress, r, out), true
				}
				return t.suplContinue(now, op, out), true
			}
		}
		return out, true
	}
	for i := range s.status {
		op := &s.status[i]
		if !op.used || op.called != src || op.from != a.TargetAddress {
			continue
		}
		op.used = false
		if op.caller != 0 {
			for range statusSends {
				t.ackFrom(op.caller, r, a.ResponseInfo, src)
			}
		}
		st := op.status
		if st == StatusPoll {
			st = Status(a.ResponseInfo)
		}
		return append(out, TSCCOutput{Kind: TSCCStatus, Grant: Grant{Service: enums.ServiceKindStatusTransport, Target: src, Source: op.from}, Status: st, Ack: r}), true
	}
	return out, false
}

func (t *TSCC) svcInbound(now time.Duration, out []TSCCOutput) ([]TSCCOutput, bool) {
	d, ok := t.b.Data.(*pdu.CSBK)
	if !ok {
		return out, false
	}
	if d.FID != enums.FIDStandardized {
		return out, true
	}
	if a, ok := d.AsAckvitation(); ok {
		return t.ackvit(now, &a, out), true
	}
	if a, ok := d.AsAckInbound(); ok {
		return t.svcAnswer(now, &a, out)
	}
	if r, ok := d.AsRandomAccess(); ok {
		return t.svcRandom(now, &r, out)
	}
	return out, false
}

func (t *TSCC) svcRandom(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) ([]TSCCOutput, bool) {
	t.wake(r.SourceAddress, now)
	switch enums.ServiceKind(r.ServiceKind) {
	case enums.ServiceKindStatusTransport:
		if r.ServiceOptions&0x20 != 0 && t.suplStart(now, r) {
			return out, true
		}
		return t.statusRandom(now, r, out), true
	case enums.ServiceKindIndividualVoice, enums.ServiceKindTalkgroupVoice:
		if r.ServiceOptions&0x10 != 0 && t.suplStart(now, r) {
			return out, true
		}
	default:
	}
	return out, false
}

func (t *TSCC) statusRandom(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) []TSCCOutput {
	src, target := r.SourceAddress, r.TargetAddress
	o := layer3.DecodeStatusServiceOptions([1]byte{r.ServiceOptions << 1})
	st := Status(o.StatusHigh)<<2 | Status(r.Reserved&3)
	if st == StatusInterrupt || st == StatusTerminate {
		return t.interruptRequest(now, src, target, o.Group, st == StatusTerminate, out)
	}
	if t.cfg.Reg && t.regIndex(src) < 0 {
		return t.refuse(src, enums.AckReasonMSNotRegistered, out)
	}
	if o.Group {
		if st == StatusPoll {
			return t.refuse(src, enums.AckReasonNotSupported, out)
		}
		t.statusAhoy(o.Group, target, src, st, r.ServiceOptions)
		t.ackFrom(src, enums.AckReasonMessageAccepted, 0, 0)
		return append(out, TSCCOutput{Kind: TSCCStatus, Grant: Grant{Service: enums.ServiceKindStatusTransport, Group: true, Target: target, Source: src}, Status: st, Ack: enums.AckReasonMessageAccepted})
	}
	for i := range t.svc.status {
		if t.svc.status[i].used && t.svc.status[i].caller == src {
			return out
		}
	}
	if t.cfg.Reg && t.regIndex(target) < 0 {
		return t.refuse(src, enums.AckReasonNoregMSAwayRefused, out)
	}
	if !t.statusStart(now, src, src, target, st, r.ServiceOptions) {
		return t.refuse(src, enums.AckReasonSysBusyRefused, out)
	}
	t.sendAck(src, enums.AckReasonWait)
	return out
}

func (t *TSCC) statusAhoy(group bool, target, source uint32, st Status, opts uint8) {
	a := pdu.AhoyPDU{ServiceOptsMirror: opts&0x60 | uint8(st)>>2, Group: group, UAB: uint8(st) & 3,
		ServiceKind: uint8(enums.ServiceKindStatusTransport), TargetAddress: target, SourceAddress: source}
	if group {
		for range statusSends {
			t.sendAhoy(a, 0, false)
		}
		return
	}
	t.sendAhoy(a, target, true)
}

func (t *TSCC) statusStart(now time.Duration, caller, from, target uint32, st Status, opts uint8) bool {
	for i := range t.svc.status {
		op := &t.svc.status[i]
		if op.used {
			continue
		}
		*op = statusOp{used: true, caller: caller, from: from, called: target, status: st, until: now + t.cfg.AnswerTimeout}
		t.statusAhoy(false, target, from, st, opts)
		return true
	}
	return false
}

// SendStatus sends the status s from source, an MS or a gateway such as
// DISPATI, to the talkgroup or MS target with a C_AHOY. StatusPoll asks an
// MS for its status. TSCCStatus reports the answer of an MS. It returns
// ErrBusy when too many status calls run, and a *dmrgo.ConfigError for an
// address out of range or a poll of a talkgroup. (ETSI TS 102 361-4 clause
// 6.6.6)
func (t *TSCC) SendStatus(now time.Duration, group bool, target, source uint32, s Status) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if target == 0 || target > 0xFFFFFF || source == 0 || source > 0xFFFFFF || s > StatusPoll || group && s == StatusPoll {
		return errTarget
	}
	o, _ := layer3.StatusServiceOptions{Group: group, StatusHigh: uint8(s) >> 2}.Field()
	if group {
		t.statusAhoy(true, target, source, s, o)
		return nil
	}
	if !t.statusStart(now, 0, source, target, s, o) {
		return ErrBusy
	}
	return nil
}

func (t *TSCC) interruptRequest(now time.Duration, src, target uint32, group, end bool, out []TSCCOutput) []TSCCOutput {
	i := -1
	for j := range 2 * t.cfg.PayloadLen {
		cs := &t.calls[j]
		g := &cs.grant
		if cs.active && cs.clears == 0 && g.Group == group && (g.Target == target || !group && g.Source == target) {
			i = j
			break
		}
	}
	if !t.cfg.TransmitInterrupt || i < 0 {
		return t.refuseFrom(src, enums.AckReasonNotSupported, constants.GatewayTSI, out)
	}
	t.ackFrom(src, enums.AckReasonMessageAccepted, 0, constants.GatewayTSI)
	reason := ReasonNone
	if end {
		reason = ReasonInterrupt
	}
	t.startRC(now, i, enums.RCCeaseTransmissionCommand, src, end, reason)
	return out
}

func (t *TSCC) startRC(now time.Duration, i int, cmd enums.RCCommand, by uint32, end bool, reason Reason) {
	left := rcSends
	if cmd < enums.RCCeaseTransmissionCommand {
		left = 1
	}
	t.svc.rc[i] = rcOp{on: true, cmd: cmd, next: now, left: left, by: by, end: end, reason: reason}
}

// Interrupt sends the RC command cmd to the MS that transmits on slot s of
// the payload channel ch, in the embedded signalling of the other slot of
// the carrier. For a cease command or request the TSCC repeats it every
// 360 ms until the transmission stops, and reports the end with
// TSCCInterrupt; it sends a power control command once. It returns
// ErrNoCall for a slot without a call and a *dmrgo.ConfigError for a reserved
// command. (ETSI TS 102 361-4 clauses 6.4.14 and 6.4.15.1)
func (t *TSCC) Interrupt(now time.Duration, ch uint16, s enums.Slot, cmd enums.RCCommand) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if cmd > enums.RCCeaseTransmissionRequest {
		return errTarget
	}
	i := t.callIndex(ch, s)
	if i < 0 || !t.calls[i].active || t.calls[i].clears > 0 {
		return ErrNoCall
	}
	t.startRC(now, i, cmd, 0, false, ReasonNone)
	return nil
}

func (t *TSCC) svcRelay(i int) {
	s := &t.svc
	b := t.b
	switch {
	case b.Kind == layer2.KindVoice && b.SyncPattern != enums.SyncNone:
		s.vIdx[i] = 0
	case b.Kind == layer2.KindVoice:
		s.vIdx[i]++
	default:
		s.vIdx[i] = 6
	}
	s.relayF[i] = s.vIdx[i] == 5
}

func (t *TSCC) svcEmit(idx int) {
	if idx == 0 {
		return
	}
	s := &t.svc
	c := idx - 1
	k := 2*c + int(t.slot-enums.Slot1)
	j := 2*c + 1 - int(t.slot-enums.Slot1)
	now := t.next - slotLength
	sp, d := s.buf.Sync()
	synced := d <= layer2.DefaultSyncThreshold
	voiceF := !synced && s.relayF[k]
	s.relayF[k] = false
	idle := synced && sp == enums.SyncBSSourcedData && s.buf.SlotType().DataType == enums.DataTypeIdle
	if idle && s.protect[k] > 0 {
		s.protect[k]--
		s.cs = pdu.CSBK{LastBlock: true}
		if s.cs.SetProtect(pdu.ProtectPDU{ProtectKind: uint8(enums.ProtectEnablePTTOneMS), TargetAddress: s.protTo[k], SourceAddress: constants.GatewayTSI}) != nil {
			return
		}
		if p, err := s.cs.Encode(); err == nil {
			if b, err := layer2.BuildDataBurst(p, enums.DataTypeCSBK, t.cfg.ColorCode, layer2.SyncSourceBS); err == nil {
				s.buf = b
			}
		}
		return
	}
	op := &s.rc[j]
	if !op.on || op.left == 0 || now < op.next || !idle && !voiceF {
		return
	}
	s.rcPDU = pdu.ReverseChannel{RCCommand: op.cmd}
	data, err := layer2.EncodeEmbeddedRC(&s.rcPDU)
	if err != nil || s.buf.SetEMB(pdu.EmbeddedSignalling{ColorCode: t.cfg.ColorCode, PreemptionAndPowerControlIndicator: true, LCSS: enums.LCSSSingleFragment}) != nil {
		return
	}
	s.buf.SetEmbeddedData(data)
	op.next, op.left = now+rcRepeat, op.left-1
}

func (t *TSCC) svcTimers(now time.Duration, out []TSCCOutput) []TSCCOutput {
	s := &t.svc
	limit := len(out) + svcEvents
	for i := range 2 * t.cfg.PayloadLen {
		cs := &t.calls[i]
		if !cs.active {
			s.seen[i], s.rc[i].on, s.protect[i] = false, false, 0
			continue
		}
		if !s.seen[i] {
			s.seen[i], s.start[i] = true, now
		}
		if t.cfg.MaxCall > 0 && cs.clears == 0 && !s.rc[i].on && now-s.start[i] >= t.cfg.MaxCall {
			if cs.inHang || !cs.started || now-cs.last >= rcIdle {
				out = t.startClear(i, ReasonCallTimer, out)
				continue
			}
			t.startRC(now, i, enums.RCCeaseTransmissionCommand, 0, true, ReasonCallTimer)
		}
		if len(out) < limit {
			out = t.rcCheck(now, i, out)
		}
	}
	for i := range s.stuns {
		if op := &s.stuns[i]; op.used && now >= op.until && len(out) < limit {
			op.used = false
			out = append(out, TSCCOutput{Kind: TSCCStun, Address: op.addr, Stun: op.act, Reason: ReasonTimeout})
		}
	}
	for i := range s.status {
		op := &s.status[i]
		if !op.used || now < op.until || len(out) >= limit {
			continue
		}
		op.used = false
		if op.caller != 0 {
			t.ackFrom(op.caller, enums.AckReasonMSAwayRefused, 0, op.called)
		}
		out = append(out, TSCCOutput{Kind: TSCCStatus, Grant: Grant{Service: enums.ServiceKindStatusTransport, Target: op.called, Source: op.from}, Status: op.status, Reason: ReasonTimeout})
	}
	for i := range s.supl {
		if op := &s.supl[i]; op.used && now >= op.until && len(out)+2 <= limit {
			out = t.suplContinue(now, op, out)
		}
	}
	for i := range s.ip {
		if op := &s.ip[i]; op.used && now >= op.until {
			op.used = false
			t.ackFrom(op.addr, enums.AckReasonIPConnectionFailed, 0, constants.GatewayIPI)
		}
	}
	return out
}

func (t *TSCC) rcCheck(now time.Duration, i int, out []TSCCOutput) []TSCCOutput {
	s := &t.svc
	op := &s.rc[i]
	cs := &t.calls[i]
	if !op.on || cs.clears > 0 {
		return out
	}
	if op.cmd < enums.RCCeaseTransmissionCommand {
		op.on = op.left > 0
		return out
	}
	ceased := cs.inHang || !cs.started || now-cs.last >= rcIdle
	switch {
	case ceased:
		op.on = false
		out = append(out, TSCCOutput{Kind: TSCCInterrupt, Address: op.by, Grant: cs.grant})
		if op.end {
			return t.startClear(i, op.reason, out)
		}
		if op.by != 0 {
			s.protect[i], s.protTo[i] = protectSent, op.by
		}
	case op.left == 0 && now >= op.next:
		op.on = false
		out = append(out, TSCCOutput{Kind: TSCCInterrupt, Address: op.by, Grant: cs.grant, Reason: ReasonTimeout})
		if op.reason == ReasonCallTimer {
			return t.startClear(i, ReasonCallTimer, out)
		}
	}
	return out
}

func (t *TSCC) ipAdvice(now time.Duration, r *pdu.RandomAccessPDU, o *layer3.RegistrationServiceOptions, out []TSCCOutput) []TSCCOutput {
	src := r.SourceAddress
	var off uint8
	if o.Register {
		var ok bool
		if out, ok = t.register(src, out); !ok {
			return t.refuse(src, enums.AckReasonRegRefused, out)
		}
		off = t.psSet(now, src, o.PowerSave)
	}
	if !t.cfg.IP {
		return t.refuseFrom(src, enums.AckReasonIPConnectionFailed, constants.GatewayIPI, out)
	}
	free := -1
	for i := range t.svc.ip {
		op := &t.svc.ip[i]
		if op.used && op.addr == src {
			free = i
			break
		}
		if !op.used && free < 0 {
			free = i
		}
	}
	if free < 0 {
		return t.refuseFrom(src, enums.AckReasonIPConnectionFailed, constants.GatewayIPI, out)
	}
	t.svc.ip[free] = ipOp{used: true, addr: src, remove: !o.Register, offset: off, until: now + t.cfg.AnswerTimeout}
	t.sendAhoy(pdu.AhoyPDU{ServiceOptsMirror: r.ServiceOptions, UAB: 1, ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: src, SourceAddress: constants.GatewayIPI}, 0, true)
	return out
}

func (t *TSCC) svcUploaded(now time.Duration, out []TSCCOutput) []TSCCOutput {
	s := &t.svc
	u := &t.u.in
	src, crcOK := u.Header.LLIDSource, u.CRCOK
	switch u.Header.LLIDDestination {
	case constants.GatewayIPI:
		for i := range s.ip {
			op := &s.ip[i]
			if !op.used || op.addr != src {
				continue
			}
			op.used = false
			if !crcOK || u.Header.UDTFormat != 6 {
				t.ackFrom(src, enums.AckReasonIPConnectionFailed, 0, constants.GatewayIPI)
				return out
			}
			o := TSCCOutput{Kind: TSCCIPAdvice, Address: src, IPv6: u.Header.UAB > 0, Remove: op.remove}
			copy(o.IP[:], u.blocks[0][:])
			if o.IPv6 {
				copy(o.IP[12:], u.blocks[1][:4])
			} else {
				clear(o.IP[4:])
			}
			t.ackFrom(src, enums.AckReasonRegAccepted, op.offset, constants.GatewayIPI)
			return append(out, o)
		}
	case constants.GatewaySUPLI:
		for i := range s.supl {
			op := &s.supl[i]
			if !op.used || op.download || op.rand.SourceAddress != src {
				continue
			}
			if !crcOK {
				return t.suplContinue(now, op, out)
			}
			return t.suplDownload(now, op, out)
		}
	}
	return out
}

func (t *TSCC) suplStart(now time.Duration, r *pdu.RandomAccessPDU) bool {
	if !t.cfg.Supplementary {
		return false
	}
	free := -1
	for i := range t.svc.supl {
		op := &t.svc.supl[i]
		if op.used && op.rand.SourceAddress == r.SourceAddress {
			if op.download {
				return true
			}
			free = i
			break
		}
		if !op.used && free < 0 {
			free = i
		}
	}
	if free < 0 {
		return false
	}
	t.svc.supl[free] = suplOp{used: true, rand: *r, until: now + t.cfg.AnswerTimeout}
	t.sendAhoy(pdu.AhoyPDU{UAB: r.Reserved >> 2, ServiceKind: r.ServiceKind, TargetAddress: r.SourceAddress, SourceAddress: constants.GatewaySUPLI}, 0, true)
	return true
}

func (t *TSCC) suplDownload(now time.Duration, op *suplOp, out []TSCCOutput) []TSCCOutput {
	r := &op.rand
	group := enums.ServiceKind(r.ServiceKind) == enums.ServiceKindTalkgroupVoice ||
		enums.ServiceKind(r.ServiceKind) == enums.ServiceKindStatusTransport && r.ServiceOptions&0x40 != 0
	if t.u.downN > 0 {
		return t.suplContinue(now, op, out)
	}
	a := t.u.in.Appended()
	t.download(&a, r.TargetAddress, constants.GatewaySUPLI, group, true, !group, pdu.UDTOpcodeOutboundHeader, t.slot.Other())
	t.u.downHold = true
	if group {
		return t.suplContinue(now, op, out)
	}
	op.download, op.until = true, now+t.cfg.AnswerTimeout
	return out
}

func (t *TSCC) suplContinue(now time.Duration, op *suplOp, out []TSCCOutput) []TSCCOutput {
	r := op.rand
	op.used = false
	if enums.ServiceKind(r.ServiceKind) == enums.ServiceKindStatusTransport {
		r.ServiceOptions &^= 0x20
	} else {
		r.ServiceOptions &^= 0x10
	}
	r.Reserved &^= 0b1100
	if o, done := t.svcRandom(now, &r, out); done {
		return o
	}
	return t.random(now, &r, out)
}
