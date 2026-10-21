package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	reqStatus request = iota + 8
	reqInterrupt
	reqIP
)

// MaxSupplementary is the most octets of supplementary_user data: four
// appended UDT blocks less the CRC (ETSI TS 102 361-4 clause 6.4.13).
const MaxSupplementary = 4*12 - 2

const (
	counterBlock = 120 * time.Millisecond
	psFrame      = 4 * counterBlock
	statusHold   = time.Second
)

type msSvc struct {
	auth    Authenticator
	noStun  bool
	noKill  bool
	stunned bool
	killed  bool

	stunAct   StunAction
	stunWait  bool
	stunChal  uint32
	stunGw    uint32
	stunUntil time.Duration

	status     Status
	lastStat   Grant
	lastStatV  Status
	lastStatAt time.Duration
	hasStat    bool

	raExtra uint8

	intCall Grant
	intOn   bool
	intEnd  bool

	ip       [16]byte
	ipv6     bool
	ipRemove bool

	supl    [MaxSupplementary]byte
	suplLen int
	suplFmt uint8

	psRQ       uint8
	psOffset   uint8
	awake      time.Duration
	awakeEnd   time.Duration
	asleep     bool
	wakeAt     time.Duration
	counter    uint16
	counterAt  time.Duration
	hasCounter bool

	massOn      bool
	massStarted bool
	massAt      time.Duration
	massEnd     time.Duration

	txAckvit pdu.AckvitationPDU
	up       Appended
}

func (m *MS) initSvc(cfg *MSConfig) error {
	if cfg.PowerSave > 7 {
		return errPowerSave
	}
	s := &m.svc
	s.auth, s.noStun, s.noKill, s.stunned, s.killed = cfg.Auth, cfg.NoStun, cfg.NoKill, cfg.Stunned, cfg.Killed
	s.psRQ, s.awake = cfg.PowerSave, cfg.Awake
	if s.awake <= 0 {
		s.awake = constants.TAwakeDefault
	}
	m.f.noFollow, m.off = s.stunned, s.killed
	return nil
}

func (m *MS) resetSvc() {
	s := &m.svc
	s.stunWait, s.intOn, s.asleep, s.hasCounter, s.massOn, s.hasStat = false, false, false, false, false, false
	s.psOffset, s.suplLen = 0, 0
	m.f.noFollow, m.off = s.stunned, s.killed
}

// Stunned reports whether a TSCC stunned the MS: it then refuses the user
// services and follows no calls (ETSI TS 102 361-4 clause 6.4.9).
func (m *MS) Stunned() bool {
	return m.svc.stunned
}

// Killed reports whether a TSCC killed the MS (ETSI TS 102 361-4 clause
// 6.4.10).
func (m *MS) Killed() bool {
	return m.svc.killed
}

// Asleep reports whether the MS sleeps in power save until its next power
// save frame (ETSI TS 102 361-4 clause 6.4.7).
func (m *MS) Asleep() bool {
	return m.svc.asleep
}

// PowerSaveOffset returns the PowerSave_Offset that the TSCC gave the MS
// with its registration, or 0 when the MS uses no power save (ETSI TS 102
// 361-4 clause 6.4.7.2.1).
func (m *MS) PowerSaveOffset() uint8 {
	return m.svc.psOffset
}

// SetStatus sets the status with which the MS answers a status poll. It
// is 0 after NewMS. It returns a *dmrgo.ConfigError for a status over 126. (ETSI
// TS 102 361-4 clause 6.6.6.2.2.1.5)
func (m *MS) SetStatus(s Status) error {
	if s >= StatusPoll {
		return errStatus
	}
	m.svc.status = s
	return nil
}

// SetSupplementary sets the supplementary_user data of the next request
// that asks for it: a SendStatus, or a Call or CallData with IsSuppData.
// The MS uploads it when the TSCC asks for it with a C_AHOY from SUPLI.
// format is the UDT_Format of the data. Empty data clears it. It returns a
// *dmrgo.ConfigError for data over MaxSupplementary octets or a format over 15.
// (ETSI TS 102 361-4 clause 6.4.13)
func (m *MS) SetSupplementary(format uint8, data []byte) error {
	if len(data) > MaxSupplementary || format > 15 {
		return errSupplementary
	}
	s := &m.svc
	s.suplFmt, s.suplLen = format, copy(s.supl[:], data)
	return nil
}

// SendStatus sends the status s to the talkgroup or MS target by random
// access. The outputs follow from Burst and Tick: OutputAccepted when the
// TSCC delivered it, or OutputRefused. It returns the errors of Call,
// ErrStunned for a stunned MS, and a *dmrgo.ConfigError for a status over 126.
// (ETSI TS 102 361-4 clause 6.6.6.1)
func (m *MS) SendStatus(now time.Duration, group bool, target uint32, s Status) error {
	if s >= StatusPoll {
		return errStatus
	}
	return m.statusRequest(now, reqStatus, group, target, s)
}

// PollStatus asks the MS target for its status by random access. The
// outputs follow from Burst and Tick: OutputStatus with the status of
// target, or OutputRefused. It returns the errors of SendStatus. (ETSI TS
// 102 361-4 clause 6.6.6.2)
func (m *MS) PollStatus(now time.Duration, target uint32) error {
	return m.statusRequest(now, reqStatus, false, target, StatusPoll)
}

func (m *MS) statusRequest(now time.Duration, req request, group bool, target uint32, st Status) error {
	o := layer3.StatusServiceOptions{Group: group, IsSuppData: m.svc.suplLen > 0, StatusHigh: uint8(st) >> 2}
	field, _ := o.Field()
	if err := m.request(now, enums.ServiceStatusTransport, target, field, false); err != nil {
		return err
	}
	m.req = req
	m.svc.raExtra |= uint8(st) & 3
	return nil
}

func (m *MS) svcStart(options uint8) {
	m.svc.raExtra, m.svc.asleep = m.suplExtra(options), false
}

func (m *MS) suplExtra(options uint8) uint8 {
	s := &m.svc
	var v uint8
	if s.suplLen > 0 {
		v = uint8((s.suplLen+1)/12) << 2
	}
	switch m.kind {
	case enums.ServiceIndividualVoice, enums.ServiceTalkgroupVoice, enums.ServiceIndividualPacketData, enums.ServiceTalkgroupPacketData:
		if options&0x10 != 0 {
			return v
		}
	case enums.ServiceStatusTransport:
		if options&0x20 != 0 {
			return v
		}
	default:
	}
	return 0
}

// Interrupt asks the TSCC to stop the transmission on the voice call that
// the MS follows on a payload channel. The MS leaves the payload channel, sends StatusInterrupt to
// the talkgroup of the call, or to the other MS of an individual call, by
// random access, and returns to the payload channel after the answer of
// the TSCC: OutputAccepted, or OutputRefused. With terminate it sends
// StatusTerminate to end the call, as the MS that set up an ALS call does,
// and stays on the TSCC. It returns ErrNoCall when the MS is not on the
// payload channel of a voice call, ErrStunned and ErrBusy. It allocates
// only when dst has no room for MaxOutputs outputs. (ETSI TS 102 361-4
// clauses 6.4.14.3.3 and 6.4.14.3.4)
func (m *MS) Interrupt(now time.Duration, terminate bool, dst []Output) ([]Output, error) {
	dst = m.timers(now, dst)
	g := m.f.call
	switch {
	case m.off || m.f.state != StatePayload || g.Data():
		return dst, ErrNoCall
	case m.svc.stunned:
		return dst, ErrStunned
	case m.req != reqNone:
		return dst, ErrBusy
	}
	target := g.Target
	if !g.Group && g.Target == m.addr {
		target = g.Source
	}
	st := StatusInterrupt
	if terminate {
		st = StatusTerminate
	}
	field, _ := layer3.StatusServiceOptions{Group: g.Group, StatusHigh: uint8(st) >> 2}.Field()
	m.ownCall = false
	dst = m.f.leave(now, ReasonInterrupt, dst)
	m.f.cleared, m.f.clearedAt, m.f.hasClear = g, now, true
	m.kind = enums.ServiceStatusTransport
	m.startAccess(now, reqInterrupt, target, field, false)
	m.svc.raExtra = uint8(st) & 3
	m.svc.intCall, m.svc.intOn, m.svc.intEnd = g, true, terminate
	return dst, nil
}

func (m *MS) resume(now time.Duration, out []Output) []Output {
	s := &m.svc
	if !s.intOn || m.req == reqInterrupt {
		return out
	}
	s.intOn, m.f.hasClear = false, false
	if s.intEnd || m.f.state != StateControl {
		return out
	}
	g := s.intCall
	m.f.call, m.f.lastBurst, m.f.inHang, m.f.state = g, now, false, StatePayload
	return append(out, Output{Kind: OutputTune, Tune: g.tune()})
}

// AdviseIP sends an IP connection advice: a registration by random access
// with IP_Inform, and the address by UDT when the TSCC asks for it with a
// C_AHOY from IPI. ip holds an IPv4
// address in its first 4 octets, or an IPv6 address when ipv6 is set.
// remove asks the TSCC to remove the address and keeps the registration.
// The outputs follow from Burst and Tick: OutputAccepted when the TSCC
// accepted the address, OutputRefused with Ack
// enums.ReasonIPConnectionFailed when it refused the address and kept the
// registration, or OutputRefused for a refused registration. It returns
// ErrNotOnControl and ErrBusy as Call does. (ETSI TS 102 361-4 clause
// 6.4.11)
func (m *MS) AdviseIP(now time.Duration, ip [16]byte, ipv6, remove bool) error {
	switch {
	case m.off || m.f.state != StateControl:
		return ErrNotOnControl
	case m.req != reqNone:
		return ErrBusy
	}
	s := &m.svc
	s.ip, s.ipv6, s.ipRemove = ip, ipv6, remove
	field, _ := layer3.RegistrationServiceOptions{IPInform: true, PowerSave: s.psRQ, Register: !remove}.Field()
	m.kind = enums.ServiceRegistration
	m.startAccess(now, reqIP, constants.GatewayREGI, field, false)
	return nil
}

func (m *MS) svcCSBK(now time.Duration, c *pdu.CSBK, h *layer2.Hint, out []Output) ([]Output, bool) {
	s := &m.svc
	switch {
	case c.BroadcastPDU != nil:
		if v, ok := c.BroadcastPDU.AsMassReg(); ok {
			m.massReg(now, &v)
		}
	case c.AhoyPDU != nil:
		return m.svcAhoy(now, c.AhoyPDU, h, out)
	case c.AckOutboundPDU != nil && c.AckOutboundPDU.TargetAddress == m.addr:
		s.awakeEnd, s.asleep = now+s.awake, false
		return m.svcAck(now, c.AckOutboundPDU, h, out)
	}
	return out, false
}

func (m *MS) svcAhoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) ([]Output, bool) {
	s := &m.svc
	if !m.member(a.GroupIndividual, a.TargetAddress) || a.SourceAddress == m.addr {
		return out, false
	}
	s.awakeEnd, s.asleep = now+s.awake, false
	kind := enums.ServiceKind(a.ServiceKind)
	switch {
	case !a.GroupIndividual && (a.SourceAddress == constants.GatewaySTUNI || a.SourceAddress == constants.GatewayKILLI) && kind == enums.ServiceSupplementary:
		return m.stunAhoy(now, a, h, out), true
	case !a.GroupIndividual && a.SourceAddress == constants.GatewayIPI:
		if m.req != reqIP {
			return out, true
		}
		m.ra, m.registered, m.regArea = raIdle, true, m.areaOf(m.f.ctl.site.SysCode)
		m.waitAnswer, m.reqUntil = true, now+m.answerTO()
		n := 4
		if s.ipv6 {
			n = 16
		}
		return m.svcUpload(now, m.replySlot(h), constants.GatewayIPI, 6, s.ip[:n], out), true
	case !a.GroupIndividual && a.SourceAddress == constants.GatewaySUPLI:
		if m.req != reqCall && m.req != reqStatus || s.suplLen == 0 {
			return out, true
		}
		m.ra, m.waitAnswer, m.reqUntil = raIdle, true, now+m.answerTO()
		n := min(s.suplLen, (int(a.AppendedBlocks)+1)*12-2)
		out = m.svcUpload(now, m.replySlot(h), constants.GatewaySUPLI, s.suplFmt, s.supl[:n], out)
		s.suplLen = 0
		return out, true
	case kind == enums.ServiceStatusTransport:
		if s.stunned {
			return out, true
		}
		return m.statusAhoy(now, a, h, out), true
	case s.stunned && kind != enums.ServiceRegistration:
		return out, true
	}
	return out, false
}

func (m *MS) replySlot(h *layer2.Hint) enums.Slot {
	if h.Slot == enums.SlotUnknown {
		return m.slot
	}
	return h.Slot
}

func (m *MS) statusAhoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	s := &m.svc
	st := Status(a.ServiceOptsMirror&0x1F)<<2 | Status(a.AppendedBlocks)
	g := Grant{Service: enums.ServiceStatusTransport, Group: a.GroupIndividual, Target: a.TargetAddress, Source: a.SourceAddress}
	repeat := s.hasStat && now-s.lastStatAt < statusHold && s.lastStatV == st && s.lastStat.Target == g.Target && s.lastStat.Source == g.Source
	s.lastStat, s.lastStatV, s.lastStatAt, s.hasStat = g, st, now, true
	if !repeat {
		out = append(out, Output{Kind: OutputStatus, Status: st, Grant: g})
	}
	if a.GroupIndividual {
		return out
	}
	m.txAck = pdu.AckInboundPDU{ReasonCode: byte(enums.ReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr}
	if st == StatusPoll {
		m.txAck.ReasonCode, m.txAck.ResponseInfo = byte(enums.ReasonStatusPollAcceptedMS), uint8(s.status)
	}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckInbound, AckInboundPDU: &m.txAck}
	return m.send(now+frame, m.replySlot(h), out)
}

func (m *MS) stunAhoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	s := &m.svc
	gw := a.SourceAddress
	act := StunStun
	switch {
	case gw == constants.GatewayKILLI:
		act = StunKill
	case a.ServiceKindFlag:
		act = StunRevive
	}
	if act == StunKill && (s.noKill || s.auth == nil) || act != StunKill && s.noStun {
		return m.finalAck(now, gw, enums.ReasonMSNotSupported, h, out)
	}
	if s.auth == nil {
		out = m.finalAck(now, gw, enums.ReasonMSAccepted, h, out)
		return m.applyStun(act, out)
	}
	if !s.stunWait || s.stunAct != act || s.stunGw != gw {
		s.stunChal = m.rng.Uint32() % (maxAddress + 1)
	}
	s.stunWait, s.stunAct, s.stunGw, s.stunUntil = true, act, gw, now+m.regTO
	s.txAckvit = pdu.AckvitationPDU{ServiceKindFlag: act == StunRevive, ServiceKind: uint8(enums.ServiceSupplementary), TargetAddress: s.stunChal, SourceAddress: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckvitation, AckvitationPDU: &s.txAckvit}
	return m.send(now+frame, m.replySlot(h), out)
}

func (m *MS) finalAck(now time.Duration, gw uint32, r enums.AckReason, h *layer2.Hint, out []Output) []Output {
	m.txAck = pdu.AckInboundPDU{ReasonCode: byte(r), TargetAddress: gw, AdditionalInfo: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckInbound, AckInboundPDU: &m.txAck}
	return m.send(now+frame, m.replySlot(h), out)
}

func (m *MS) applyStun(act StunAction, out []Output) []Output {
	s := &m.svc
	switch act {
	case StunStun:
		s.stunned, m.f.noFollow = true, true
	case StunRevive:
		s.stunned, m.f.noFollow = false, false
	case StunKill:
		s.killed, m.off = true, true
	case StunNone:
	}
	return append(out, Output{Kind: OutputStun, Stun: act})
}

func (m *MS) svcAck(now time.Duration, a *pdu.AckOutboundPDU, h *layer2.Hint, out []Output) ([]Output, bool) {
	s := &m.svc
	r := enums.AckReason(a.ReasonCode)
	if r == enums.ReasonAuthResponseTS && s.stunWait {
		s.stunWait = false
		if !m.authOK(a.AdditionalInfo) {
			return m.finalAck(now, s.stunGw, enums.ReasonRecipientRefused, h, out), true
		}
		out = m.finalAck(now, s.stunGw, enums.ReasonMSAccepted, h, out)
		return m.applyStun(s.stunAct, out), true
	}
	if r == enums.ReasonRegAccepted && (m.req == reqRegister || m.req == reqIP) {
		s.psOffset = 0
		if s.psRQ > 0 {
			s.psOffset = a.ResponseInfo
		}
	}
	if m.ra == raWaitSlot && m.retries == 0 && m.req != reqNone {
		return out, true
	}
	switch m.req {
	case reqStatus, reqInterrupt:
		m.ra = raIdle
		switch {
		case r.Type() == enums.AckNACK:
			out = m.fail(ReasonNack, r, out)
		case r == enums.ReasonStatusPollAcceptedMS || r == enums.ReasonStatusPollAcceptedTS:
			out = append(out, Output{Kind: OutputStatus, Status: Status(a.ResponseInfo), Grant: Grant{Service: enums.ServiceStatusTransport, Target: m.addr, Source: m.target}})
			m.done()
		case r.Type() == enums.AckACK:
			out = append(out, Output{Kind: OutputAccepted, Ack: r})
			m.done()
		default:
			m.waitAnswer, m.reqUntil = true, now+m.answerTO()
		}
		return m.resume(now, out), true
	case reqIP:
		return m.ipAck(now, r, out), true
	case reqNone, reqRegister, reqDereg, reqCall:
	}
	return out, false
}

func (m *MS) authOK(resp uint32) bool {
	want, ok := m.svc.auth.Response(m.addr, m.svc.stunChal)
	return ok && want&0xFFFFFF == resp
}

func (m *MS) ipAck(now time.Duration, r enums.AckReason, out []Output) []Output {
	m.ra = raIdle
	was := m.Registered()
	switch {
	case r == enums.ReasonRegAccepted:
		m.registered, m.regArea = true, m.areaOf(m.f.ctl.site.SysCode)
		m.done()
		if !was {
			out = append(out, Output{Kind: OutputRegistered, Ack: r})
		}
		return append(out, Output{Kind: OutputAccepted, Ack: r})
	case r == enums.ReasonIPConnectionFailed:
		m.registered, m.regArea = true, m.areaOf(m.f.ctl.site.SysCode)
		if !was {
			out = append(out, Output{Kind: OutputRegistered, Ack: r})
		}
		return m.fail(ReasonNack, r, out)
	case r == enums.ReasonRegDenied:
		m.deny(now)
		out = m.fail(ReasonNack, r, out)
		return m.leaveTSCC(now, ReasonDenied, out)
	case r == enums.ReasonRegRefused:
		out = m.fail(ReasonNack, r, out)
		return m.leaveTSCC(now, ReasonNack, out)
	case r.Type() == enums.AckNACK:
		return m.fail(ReasonNack, r, out)
	}
	m.waitAnswer, m.reqUntil = true, now+m.answerTO()
	return out
}

func (m *MS) svcUpload(now time.Duration, slot enums.Slot, dest uint32, format uint8, data []byte, out []Output) []Output {
	n := max(1, (len(data)+2+11)/12)
	u := &m.svc.up
	*u = Appended{Format: enums.UDTFormat(format), PadNibble: uint8((n*12 - 2 - len(data)) * 2), Blocks: n}
	copy(u.Data[:], data)
	return m.sendUDT(now+frame, slot, u, dest, false, false, true, pdu.UDTOpcodeInboundHeader, out)
}

func (m *MS) massReg(now time.Duration, v *pdu.MassRegParms) {
	s := &m.svc
	a := Access{Mask: v.AlohaMask, Address: v.Address}
	if m.noReg || !a.Applies(m.addr, false) {
		return
	}
	win, ok := pdu.RegWindowTime(v.RegWindow)
	switch {
	case !ok:
		s.massOn = false
	case !s.massOn || now >= s.massEnd:
		s.massOn, s.massStarted, s.massAt, s.massEnd = true, false, now+m.rng.Duration(win), now+win
	}
}

func (m *MS) svcBurst(now time.Duration, before State, h *layer2.Hint, out []Output, n int) []Output {
	s := &m.svc
	c := &m.f.ctl
	if c.counterNew {
		c.counterNew = false
		s.counter, s.counterAt, s.hasCounter = c.site.SlotCounter, now, true
	}
	for i := n; i < len(out); i++ {
		switch out[i].Kind {
		case OutputLost:
			s.massOn, s.stunWait = false, false
		case OutputUDT:
			u := &c.udt
			hd := &u.Header
			if u.CRCOK && hd.SupplementaryFlag && !hd.Group && hd.LLIDSource == constants.GatewaySUPLI && hd.LLIDDestination == m.addr && !s.stunned {
				s.awakeEnd = now + s.awake
				out = m.finalAck(now, constants.GatewaySUPLI, enums.ReasonMSAccepted, h, out)
			}
		default:
		}
	}
	if m.f.state == StatePayload && before == StatePayload && h.Slot != enums.SlotUnknown && h.Slot != m.f.call.Slot {
		b := c.b
		if b.HasReverseChannel() && !b.FEC.EMB.Uncorrectable && !b.FEC.RC.Uncorrectable &&
			(!m.f.call.HasColorCode || b.EmbeddedSignalling.ColorCode == m.f.call.ColorCode) {
			out = append(out, Output{Kind: OutputReverseChannel, RC: b.ReverseChannel.RCCommand, Grant: m.f.call})
		}
	}
	return m.resume(now, out)
}

func (m *MS) svcTimers(now time.Duration, out []Output) []Output {
	s := &m.svc
	if s.intOn {
		m.f.clearedAt = now
	}
	if s.stunWait && now >= s.stunUntil {
		s.stunWait = false
	}
	if (m.req == reqStatus || m.req == reqInterrupt || m.req == reqIP) && m.waitAnswer && now >= m.reqUntil {
		out = m.fail(ReasonTimeout, 0, out)
	}
	if s.asleep && now < s.wakeAt {
		m.f.lastTSCC = now
	}
	return out
}

func (m *MS) svcLate(now time.Duration, out []Output) []Output {
	s := &m.svc
	if s.massOn && now >= s.massEnd {
		s.massOn = false
	}
	if s.massOn && !s.massStarted && now >= s.massAt && m.f.state == StateControl && m.req == reqNone {
		s.massStarted = true
		m.kind = enums.ServiceRegistration
		m.startAccess(now, reqRegister, constants.GatewayREGI, m.regOptions(), false)
	}
	out = m.powerSave(now, out)
	return m.resume(now, out)
}

func (m *MS) powerSave(now time.Duration, out []Output) []Output {
	s := &m.svc
	if s.psOffset == 0 || !s.hasCounter || !m.Registered() {
		s.asleep = false
		return out
	}
	busy := m.f.state != StateControl || m.req != reqNone || m.pending || s.stunWait || s.intOn || s.massDue(now+s.awake)
	if s.asleep {
		if now < s.wakeAt && !busy {
			m.f.lastTSCC = now
			return out
		}
		s.asleep, m.f.lastTSCC = false, now
		s.awakeEnd = max(s.awakeEnd, s.wakeAt+psFrame+frame)
	}
	if busy {
		s.awakeEnd = max(s.awakeEnd, now+s.awake)
		return out
	}
	if now < s.awakeEnd {
		return out
	}
	at := s.nextWake(now)
	if at <= now {
		s.awakeEnd = at + psFrame + frame
		return out
	}
	s.asleep, s.wakeAt = true, at
	return append(out, Output{Kind: OutputSleep, Due: at})
}

func (s *msSvc) massDue(at time.Duration) bool {
	return s.massOn && !s.massStarted && at >= s.massAt
}

func (s *msSvc) nextWake(now time.Duration) time.Duration {
	mask := int64(1)<<s.psRQ - 1
	c := int64(s.counter)
	q := c >> 2
	if now > s.counterAt {
		q = (c + int64((now-s.counterAt)/counterBlock)) >> 2
	}
	for range 2*mask + 2 {
		if q&mask == int64(s.psOffset)&mask {
			if start := s.counterAt + time.Duration(4*q-c-1)*counterBlock; start+psFrame > now {
				return start
			}
		}
		q++
	}
	return now
}

func (m *MS) sleepDeadline() (time.Duration, bool) {
	s := &m.svc
	at := s.wakeAt
	if s.massOn && !s.massStarted && s.massAt < at {
		at = s.massAt
	}
	return at, true
}

func (m *MS) svcDeadline(at time.Duration, ok bool) (time.Duration, bool) {
	s := &m.svc
	if s.stunWait {
		at, ok = earliest(at, ok, s.stunUntil)
	}
	if s.massOn {
		at, ok = earliest(at, ok, s.massEnd)
		if !s.massStarted && m.f.state == StateControl && m.req == reqNone {
			at, ok = earliest(at, ok, s.massAt)
		}
	}
	if s.psOffset != 0 && s.hasCounter && m.Registered() {
		at, ok = earliest(at, ok, s.awakeEnd)
	}
	return at, ok
}
