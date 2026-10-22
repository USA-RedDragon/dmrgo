package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func (m *MS) startAccess(now time.Duration, req request, target uint32, options uint8, emergency bool) {
	m.req, m.target, m.options, m.emergency = req, target, options, emergency
	m.ra, m.frames, m.retries, m.raStart, m.waitAnswer = raWaitSlot, 1, 0, now, false
	m.u.reserved, m.u.proxy, m.u.sendTo = 0, false, 0
	m.svcStart(options)
}

func (m *MS) backoff() {
	m.ra, m.frames = raWaitSlot, between(&m.rng, m.f.ctl.site.Access.Backoff.Frames())
}

func (m *MS) frame(now time.Duration, cach *layer2.CACH, out []Output) []Output {
	switch m.ra {
	case raIdle, raWaitAloha:
		return out
	case raWaitResp:
		m.frames--
		if m.frames > 0 {
			return out
		}
		if m.retries >= m.maxRetries() || (m.randTO > 0 && now >= m.raStart+m.randTO) {
			if m.req == reqDereg {
				m.done()
				return out
			}
			out = m.fail(ReasonNoAnswer, 0, out)
			return m.leaveTSCC(now, ReasonNoAnswer, out)
		}
		m.backoff()
		return out
	case raWaitSlot:
	}
	m.frames--
	if m.frames > 0 {
		return out
	}
	if cach == nil || cach.TACT.FEC.Uncorrectable || cach.TACT.AccessType {
		m.backoff()
		return out
	}
	if !m.invited() {
		m.ra = raWaitAloha
		return out
	}
	m.tx = pdu.CSBK{LastBlock: true}
	err := m.tx.SetRandomAccess(pdu.RandomAccessPDU{ServiceOptions: m.options, ProxyFlag: m.u.proxy, Reserved: m.u.reserved | m.svc.raExtra, ServiceKind: uint8(m.kind),
		TargetAddress: m.target, SourceAddress: m.addr})
	m.retries++
	m.ra, m.frames = raWaitResp, m.f.ctl.site.Access.NRandWait.Frames()+1
	return m.send(err, now+frame, m.slot, out)
}

func (m *MS) invited() bool {
	s := &m.f.ctl.site
	if !s.HasAccess || !s.Access.Applies(m.addr, m.emergency) {
		return false
	}
	return m.emergency || s.Access.Function.Invites(m.kind)
}

func (m *MS) maxRetries() int {
	if m.emergency {
		return constants.NRandNE
	}
	return constants.NRandNR
}

func (m *MS) addressed(now time.Duration, h *layer2.Hint, out []Output) []Output {
	if !m.f.ctl.okData() {
		return out
	}
	if u, ok := m.f.ctl.b.Data.(*pdu.UnifiedSingleBlockData); ok {
		return m.usbdPoll(now, u, h, out)
	}
	c, ok := m.f.ctl.b.Data.(*pdu.CSBK)
	if !ok || c.FID != enums.FIDStandardized {
		return out
	}
	if o, done := m.svcCSBK(now, c, h, out); done {
		return o
	}
	if _, ok := c.AsAloha(); ok {
		if m.ra == raWaitAloha {
			m.backoff()
		}
		return out
	}
	if a, ok := c.AsAckOutbound(); ok {
		if a.TargetAddress == m.addr {
			return m.answer(now, enums.AckReason(a.ReasonCode), a.AdditionalInfo, out)
		}
		return out
	}
	if a, ok := c.AsAhoy(); ok {
		return m.ahoyAny(now, &a, h, out)
	}
	if mv, ok := c.AsMove(); ok && mv.PhysicalChannel != ChannelAbsolute {
		return m.moved(now, out)
	}
	return out
}

func (m *MS) moved(now time.Duration, out []Output) []Output {
	mv := m.f.ctl.move
	m.f.ctl.move = pdu.MovePDU{}
	a := Access{Mask: mv.Mask, Address: mv.MSAddress}
	if !a.Applies(m.addr, false) {
		return out
	}
	out = m.leaveTSCC(now, ReasonMove, out)
	n := len(out)
	if n > 0 && out[n-1].Kind == OutputTune {
		out = out[:n-1]
	}
	return m.f.moveTo(now, mv.PhysicalChannel, out)
}

func (m *MS) answer(now time.Duration, r enums.AckReason, from uint32, out []Output) []Output {
	if m.req == reqNone || m.staleAck(from) {
		return out
	}
	m.ra = raIdle
	switch m.req {
	case reqDereg:
		if r == enums.AckReasonRegAccepted {
			m.done()
			m.off = true
			return append(out, Output{Kind: OutputPowerOff, Ack: r})
		}
		return out
	case reqRegister:
		switch {
		case r == enums.AckReasonRegAccepted:
			m.registered, m.regArea = true, m.areaOf(m.f.ctl.site.SysCode)
			m.done()
			return append(out, Output{Kind: OutputRegistered, Ack: r})
		case r == enums.AckReasonRegDenied:
			m.deny(now)
			out = m.fail(ReasonNack, r, out)
			return m.leaveTSCC(now, ReasonDenied, out)
		case r == enums.AckReasonRegRefused:
			out = m.fail(ReasonNack, r, out)
			return m.leaveTSCC(now, ReasonNack, out)
		case r.Type() == enums.AckNACK:
			return m.fail(ReasonNack, r, out)
		}
	case reqCall:
		if r.Type() == enums.AckNACK {
			return m.fail(ReasonNack, r, out)
		}
	case reqShortData, reqPoll, reqDivert, reqDGNA:
		return m.udtAnswer(now, r, out)
	case reqNone, reqStatus, reqInterrupt, reqIP:
	}
	m.waitAnswer, m.reqUntil = true, now+m.answerTO()
	return out
}

func (m *MS) member(group bool, target uint32) bool {
	if !group {
		return target == m.addr
	}
	for i := range m.f.talkLen {
		if m.f.talk[i] == target {
			return true
		}
	}
	return m.f.dynamic(target)
}

const maxChallenge = 0xFFFCDF

type authChallenge struct {
	on      bool
	payload bool
	slot    enums.Slot
	due     time.Duration
}

func (m *MS) isChallenge(a *pdu.AhoyPDU) bool {
	return enums.ServiceKind(a.ServiceKind) == enums.ServiceKindRegistration && !a.ServiceKindFlag && !a.Group &&
		a.TargetAddress == m.addr && a.SourceAddress <= maxChallenge
}

func (m *MS) challenged(now time.Duration, c uint32, slot enums.Slot, payload bool, out []Output) []Output {
	m.auth = authChallenge{on: true, payload: payload, slot: slot, due: now + frame}
	if m.req != reqNone && m.req != reqDereg {
		m.ra, m.waitAnswer, m.reqUntil = raIdle, true, now+m.answerTO()
	}
	return append(out, Output{Kind: OutputAuthChallenge, Challenge: c})
}

// AuthResponse sends response, the answer to the challenge of the last
// OutputAuthChallenge that trunkauth.Response computes, to the TSCC: a C_ACKU,
// or a P_ACKU on a payload channel, with reason Authentication Response,
// in the slot after the challenge. It appends that OutputBurst to dst. It
// does nothing when no challenge waits or that slot has passed, so call it
// before the next Burst or Tick. It allocates only when dst has no room
// for MaxOutputs outputs. (ETSI TS 102 361-4 V1.12.1 clauses 6.4.8.3
// and 6.6.2.3.2.2, Table 6.18)
func (m *MS) AuthResponse(now time.Duration, response uint32, dst []Output) []Output {
	if m == nil {
		return dst
	}

	a := m.auth
	m.auth = authChallenge{}
	if m.off || !a.on || now > a.due {
		return dst
	}
	m.tx = pdu.CSBK{LastBlock: true}
	var err error
	if a.payload {
		err = m.tx.SetAckInboundPayload(pdu.AckInboundPayloadPDU{ReasonCode: byte(enums.AckReasonAuthResponseMS), TargetAddress: response & 0xFFFFFF, AdditionalInfo: m.addr})
	} else {
		err = m.tx.SetAckInbound(pdu.AckInboundPDU{ReasonCode: byte(enums.AckReasonAuthResponseMS), TargetAddress: response & 0xFFFFFF, AdditionalInfo: m.addr})
	}
	return m.send(err, a.due, a.slot, dst)
}

func (m *MS) ahoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	slot := h.Slot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	if m.isChallenge(a) {
		return m.challenged(now, a.SourceAddress, slot, false, out)
	}
	if !m.member(a.Group, a.TargetAddress) || m.req != reqNone {
		return out
	}
	kind := enums.ServiceKind(a.ServiceKind)
	switch kind {
	case enums.ServiceKindIndividualVoice:
		if a.Group {
			return out
		}
		m.pending, m.pendingEnd = true, now+m.pendingTO
	case enums.ServiceKindRegistration:
	default:
		return out
	}
	out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: kind, Group: a.Group, Target: a.TargetAddress, Source: a.SourceAddress}})
	return m.sendAck(now+frame, slot, pdu.AckInboundPDU{ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr}, out)
}

func (m *MS) payload(now time.Duration, h *layer2.Hint, out []Output) []Output {
	g := &m.f.call
	if h.Slot != enums.SlotUnknown && h.Slot != g.Slot || !m.f.ctl.dataOK() {
		return out
	}
	c, ok := m.f.ctl.b.Data.(*pdu.CSBK)
	if !ok || c.FID != enums.FIDStandardized {
		return out
	}
	if p, ok := c.AsProtect(); ok {
		kind := enums.ProtectKind(p.ProtectKind)
		if kind == enums.ProtectIllegallyParked {
			if m.addr != p.SourceAddress && m.addr != p.TargetAddress {
				m.ownCall = false
				return m.f.leave(now, ReasonParked, out)
			}
			return out
		}
		if p.Group && g.Group && p.TargetAddress == g.Target || !p.Group && p.TargetAddress == m.addr ||
			kind == enums.ProtectEnablePTTOneMS {
			return append(out, Output{Kind: OutputProtect, Protect: kind, Grant: *g})
		}
		return out
	}
	a, ok := c.AsAhoy()
	switch {
	case ok && m.isChallenge(&a):
		return m.challenged(now, a.SourceAddress, g.Slot, true, out)
	case ok && enums.ServiceKind(a.ServiceKind) == enums.ServiceKindRegistration && m.member(a.Group, a.TargetAddress):
		out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: enums.ServiceKindRegistration, Group: a.Group, Target: a.TargetAddress, Source: a.SourceAddress}})
		m.tx = pdu.CSBK{LastBlock: true}
		err := m.tx.SetAckInboundPayload(pdu.AckInboundPayloadPDU{ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr})
		return m.send(err, now+frame, g.Slot, out)
	}
	return out
}

func (m *MS) sendAck(due time.Duration, slot enums.Slot, a pdu.AckInboundPDU, out []Output) []Output {
	m.tx = pdu.CSBK{LastBlock: true}
	return m.send(m.tx.SetAckInbound(a), due, slot, out)
}

func (m *MS) send(err error, due time.Duration, slot enums.Slot, out []Output) []Output {
	if err != nil {
		return out
	}
	p, err := m.tx.Encode()
	if err != nil {
		return out
	}
	return m.sendRaw(due, slot, p, enums.DataTypeCSBK, out)
}

func (m *MS) done() {
	m.req, m.ra, m.waitAnswer = reqNone, raIdle, false
}

func (m *MS) fail(reason Reason, ack enums.AckReason, out []Output) []Output {
	m.done()
	return append(out, Output{Kind: OutputRefused, Reason: reason, Ack: ack})
}
