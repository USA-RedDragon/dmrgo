package bs

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func (r *Repeater) authorize(now time.Duration, s *slot, b *layer2.Burst, raw *layer2.RawBurst, hang bool, from Source, dst []Output) ([]Output, bool) {
	switch s.state {
	case SlotChanAuth:
		if !r.authMatch(s, b, raw, from) {
			return dst, true
		}
		r.authEnd(now, s)
		s.authed = true
		return dst, false
	case SlotChanAuthSelect:
		if burst, source, voiceAuth, ok := r.chanAuthPDU(b, raw); ok && source < s.authSource {
			s.auth, s.authFrom, s.authSource, s.authVoice = burst, from, source, voiceAuth
			dst = r.forward(now, s, from, dst)
		}
		return dst, true
	case SlotIdle, SlotCallHang:
		s.authed = false
	case SlotData, SlotDataHang, SlotPreamble:
		return dst, false
	case SlotVoice, SlotRejected:
		if s.authed && from == s.authFrom {
			if source, ok := r.burstSource(b, raw); ok && source != s.authSource {
				return dst, true
			}
		}
		return dst, false
	}
	if !hang || !from.RF && r.chanAuth != ChannelAuthMultiSite {
		return dst, false
	}
	burst, source, voiceAuth, ok := r.chanAuthPDU(b, raw)
	if !ok || s.state == SlotCallHang && (!voiceAuth || !sameTarget(&r.lc, &s.call)) {
		return dst, false
	}
	var lc *pdu.LCBytes
	if voiceAuth {
		lc = &r.lc
	}
	admitted, dst := r.admit(now, s, from, lc, dst)
	if !admitted {
		return dst, true
	}
	s.authPrev = s.state
	s.auth, s.authFrom, s.authSource, s.authVoice = burst, from, source, voiceAuth
	if r.chanAuth == ChannelAuthMultiSite {
		s.state, s.authUntil = SlotChanAuthSelect, now+r.authSel
		return r.forward(now, s, from, dst), true
	}
	return r.authSelected(now, s, dst), true
}

func (r *Repeater) forward(now time.Duration, s *slot, from Source, dst []Output) []Output {
	if !from.RF {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst)
	o.Slot, o.To, o.Burst, o.Due, o.From = s.id, ToNetwork, s.auth, now, from
	return dst
}

func (r *Repeater) authSelected(at time.Duration, s *slot, dst []Output) []Output {
	rsp := r.authRsp
	switch {
	case rsp != 0:
	case s.authVoice:
		rsp = constants.TBSChanAuthRspVoice
	case r.chanAuth == ChannelAuthMultiSite:
		rsp = constants.TBSChanAuthRspCSBKMulti
	default:
		rsp = constants.TBSChanAuthRspCSBKSingle
	}
	s.state, s.authUntil = SlotChanAuth, at+max(rsp, 0)
	return dst
}

func (r *Repeater) authEnd(now time.Duration, s *slot) {
	s.state = s.authPrev
	if s.state == SlotCallHang && now >= s.hangUntil {
		s.state = SlotIdle
	}
}

func (r *Repeater) authMatch(s *slot, b *layer2.Burst, raw *layer2.RawBurst, from Source) bool {
	if source, ok := r.burstSource(b, raw); ok {
		return source == s.authSource
	}
	return s.authVoice && b.Kind == layer2.KindVoice && from == s.authFrom
}

func (r *Repeater) burstSource(b *layer2.Burst, raw *layer2.RawBurst) (uint32, bool) {
	if b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable {
		return 0, false
	}
	p, res := r.bptc(raw)
	if res.Uncorrectable {
		return 0, false
	}
	var source uint32
	ok := false
	switch dt := b.SlotType.DataType; dt {
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		if octets, lres, err := pdu.DecodeFullLCCodeword(p, dt); err == nil && !lres.Uncorrectable {
			r.lc = octets
			_, source, ok = r.lc.Addresses()
		}
	case enums.DataTypeCSBK:
		if c := pdu.CSBKBytes(p); c.CRCOK() {
			_, source, ok = c.Addresses()
		}
	case enums.DataTypeDataHeader:
		if h := pdu.DataHeaderBytes(p); h.CRCOK() {
			_, source, ok = h.Addresses()
		}
	case enums.DataTypePIHeader, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeRate12,
		enums.DataTypeRate34, enums.DataTypeIdle, enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
	}
	return source, ok
}

func (r *Repeater) chanAuthPDU(b *layer2.Burst, raw *layer2.RawBurst) (out layer2.RawBurst, source uint32, voiceAuth, ok bool) {
	if b.Kind != layer2.KindData {
		return out, 0, false, false
	}
	switch b.SlotType.DataType {
	case enums.DataTypeVoiceLCHeader:
		if source, ok = r.burstSource(b, raw); !ok {
			return out, 0, false, false
		}
		cw, err := r.lc.Codeword(enums.DataTypeVoiceLCHeader)
		if err != nil {
			return out, 0, false, false
		}
		out, err = layer2.BuildDataBurst(cw, enums.DataTypeVoiceLCHeader, r.cc, layer2.SyncSourceBS)
		return out, source, true, err == nil
	case enums.DataTypeCSBK:
		p, res := r.bptc(raw)
		c := pdu.CSBKBytes(p)
		if res.Uncorrectable || b.SlotType.FEC.Uncorrectable || !c.CRCOK() || c.FID() != enums.FIDStandardized || c.Opcode() != pdu.CSBKPreamble {
			return out, 0, false, false
		}
		if _, source, ok = c.Addresses(); !ok {
			return out, 0, false, false
		}
		out, err := layer2.BuildDataBurst(p, enums.DataTypeCSBK, r.cc, layer2.SyncSourceBS)
		return out, source, false, err == nil
	case enums.DataTypePIHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeIdle, enums.DataTypeRate1,
		enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
	}
	return out, 0, false, false
}
