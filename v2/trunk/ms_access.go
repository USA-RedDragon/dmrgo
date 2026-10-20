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
	m.txRand = pdu.RandomAccessPDU{ServiceOptions: m.options, ServiceKind: uint8(m.kind), TargetAddress: m.target, SourceAddress: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKRandomAccess, RandomAccessPDU: &m.txRand}
	m.retries++
	m.ra, m.frames = raWaitResp, m.f.ctl.site.Access.NRandWait.Frames()+1
	return m.send(now+frame, m.slot, out)
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
	c, ok := m.f.ctl.b.Data.(*pdu.CSBK)
	if !ok || c.FID != enums.FIDStandardized {
		return out
	}
	switch {
	case c.AlohaPDU != nil:
		if m.ra == raWaitAloha {
			m.backoff()
		}
	case c.AckOutboundPDU != nil && c.AckOutboundPDU.TargetAddress == m.addr:
		return m.answer(now, enums.AckReason(c.AckOutboundPDU.ReasonCode), out)
	case c.AhoyPDU != nil:
		return m.ahoy(now, c.AhoyPDU, h, out)
	case c.MovePDU != nil && c.MovePDU.PhysicalChannel != ChannelAbsolute:
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

func (m *MS) answer(now time.Duration, r enums.AckReason, out []Output) []Output {
	if m.req == reqNone {
		return out
	}
	m.ra = raIdle
	switch m.req {
	case reqDereg:
		if r == enums.ReasonRegAccepted {
			m.done()
			m.off = true
			return append(out, Output{Kind: OutputPowerOff, Ack: r})
		}
		return out
	case reqRegister:
		switch {
		case r == enums.ReasonRegAccepted:
			m.registered, m.regArea = true, m.areaOf(m.f.ctl.site.SysCode)
			m.done()
			return append(out, Output{Kind: OutputRegistered, Ack: r})
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
	case reqCall:
		if r.Type() == enums.AckNACK {
			return m.fail(ReasonNack, r, out)
		}
	case reqNone:
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
	return false
}

func (m *MS) ahoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	if !m.member(a.GroupIndividual, a.TargetAddress) || m.req != reqNone {
		return out
	}
	kind := enums.ServiceKind(a.ServiceKind)
	switch kind {
	case enums.ServiceIndividualVoice:
		if a.GroupIndividual {
			return out
		}
		m.pending, m.pendingEnd = true, now+m.pendingTO
	case enums.ServiceRegistration:
	default:
		return out
	}
	out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: kind, Group: a.GroupIndividual, Target: a.TargetAddress, Source: a.SourceAddress}})
	m.txAck = pdu.AckInboundPDU{ReasonCode: byte(enums.ReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckInbound, AckInboundPDU: &m.txAck}
	slot := h.Slot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	return m.send(now+frame, slot, out)
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
	switch {
	case c.ProtectPDU != nil:
		p := c.ProtectPDU
		kind := enums.ProtectKind(p.ProtectKind)
		if kind == enums.ProtectIllegallyParked {
			if m.addr != p.SourceAddress && m.addr != p.TargetAddress {
				m.ownCall = false
				return m.f.leave(now, ReasonParked, out)
			}
			return out
		}
		if p.GroupIndividual && g.Group && p.TargetAddress == g.Target || !p.GroupIndividual && p.TargetAddress == m.addr ||
			kind == enums.ProtectEnablePTTOneMS && p.GroupIndividual && g.Group && p.TargetAddress == g.Target {
			return append(out, Output{Kind: OutputProtect, Protect: kind, Grant: *g})
		}
	case c.AhoyPDU != nil && enums.ServiceKind(c.AhoyPDU.ServiceKind) == enums.ServiceRegistration && m.member(c.AhoyPDU.GroupIndividual, c.AhoyPDU.TargetAddress):
		a := c.AhoyPDU
		out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: enums.ServiceRegistration, Group: a.GroupIndividual, Target: a.TargetAddress, Source: a.SourceAddress}})
		m.txAckP = pdu.AckInboundPayloadPDU{ReasonCode: byte(enums.ReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr}
		m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckInboundPayload, AckInboundPayloadPDU: &m.txAckP}
		return m.send(now+frame, g.Slot, out)
	}
	return out
}

func (m *MS) send(due time.Duration, slot enums.Slot, out []Output) []Output {
	p, err := m.tx.Encode()
	if err != nil {
		return out
	}
	cc := m.f.ctl.site.ColorCode
	if m.f.state == StatePayload && m.f.call.HasColorCode {
		cc = m.f.call.ColorCode
	}
	raw, err := layer2.BuildDataBurst(p, enums.DataTypeCSBK, cc, layer2.SyncSourceMS)
	if err != nil {
		return out
	}
	return append(out, Output{Kind: OutputBurst, Burst: raw, Slot: slot, Due: due})
}

func (m *MS) done() {
	m.req, m.ra, m.waitAnswer = reqNone, raIdle, false
}

func (m *MS) fail(reason Reason, ack enums.AckReason, out []Output) []Output {
	m.done()
	return append(out, Output{Kind: OutputRefused, Reason: reason, Ack: ack})
}
