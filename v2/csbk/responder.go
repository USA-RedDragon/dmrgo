package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// OACSUAnswer is how a Responder answers UU_V_Req, the presence check of
// an OACSU individual call.
type OACSUAnswer uint8

// The OACSUAnswer values.
const (
	// OACSUProceed answers UU_Ans_Rsp with Proceed.
	OACSUProceed OACSUAnswer = iota
	// OACSUDeny answers UU_Ans_Rsp with Deny.
	OACSUDeny
	// OACSUNotSupported answers the Feature Not Supported NACK_Rsp, unless
	// ResponderConfig.NoFNS is set.
	OACSUNotSupported
)

// String returns the name of the answer. It allocates only for a value
// without a name.
func (a OACSUAnswer) String() string {
	switch a {
	case OACSUProceed:
		return "Proceed"
	case OACSUDeny:
		return "Deny"
	case OACSUNotSupported:
		return "NotSupported"
	}
	return "OACSUAnswer(" + text.Int(int64(a)) + ")"
}

// ResponderConfig configures NewResponder.
type ResponderConfig struct {
	// Address is the 24-bit address of the radio. The Responder answers
	// the CSBKs that address it.
	Address uint32
	// ColorCode is the color code of the slot types of the bursts sent.
	ColorCode enums.ColorCode
	// Source selects the SYNC pattern: layer2.SyncSourceBS, the zero value,
	// for a repeater outbound, layer2.SyncSourceMS for a radio.
	Source layer2.SyncSource
	// Access is the channel access policy of the answers. The zero value
	// is polite to own color code.
	Access enums.Access
	// OACSU is the answer to UU_V_Req. The zero value is OACSUProceed.
	OACSU OACSUAnswer
	// NoRadioCheck leaves Motorola radio checks without an answer and
	// without an OutputRequest. An AnyTone radio answers them only when its
	// settings allow it.
	NoRadioCheck bool
	// NoCallAlert leaves Motorola call alerts without an acknowledgement
	// and without an OutputRequest.
	NoCallAlert bool
	// NoFNS leaves standard CSBKs that the Responder does not support
	// without the Feature Not Supported NACK_Rsp, which is mandatory for an
	// MS. Their OutputRequest stays. (ETSI TS 102 361-2 clause 5.1.2)
	NoFNS bool
	// IdleSearch is T_IdleSrch, the wait for an idle channel before a
	// polite answer. 0 means DefaultIdleSearch, and a negative value does
	// not wait.
	IdleSearch time.Duration
	// HoldoffMax is the upper end of the random holdoff T_Holdoff of a
	// NACK_Rsp. 0 means DefaultHoldoffMax, and a negative value means no
	// holdoff.
	HoldoffMax time.Duration
	// Seed seeds the generator of the random holdoff.
	Seed uint64
	// Scratch, when set, is the Burst that Burst decodes into, as for
	// RequesterConfig.Scratch.
	Scratch *layer2.Burst
}

// Responder answers the CSBK requests that address one radio on one
// logical channel, as the package doc describes. It answers one request at
// a time: a request that comes before the answer to the one before has
// gone out replaces it. Make it with NewResponder. A Responder is not safe
// for concurrent use and must not be copied; go vet reports a copy.
type Responder struct {
	address     uint32
	oacsu       OACSUAnswer
	noRadio     bool
	noAlert     bool
	noFNS       bool
	s           sender
	d           decoder
	service     Service
	reqTarget   uint32
	reqSource   uint32
	build       pdu.CSBK
	uuAns       pdu.UnitToUnitVoiceServiceAnswerResponsePDU
	nack        pdu.NegativeAcknowledgementPDU
	noCopy      noCopy
	answerReady bool
}

// NewResponder returns a Responder for cfg. It returns a *dmrgo.ConfigError
// for a nil cfg, an Address of 0 or above 24 bits, a color code above 15, an
// unknown Source, Access or OACSU. It allocates the Responder, and its Burst
// unless cfg.Scratch is set.
func NewResponder(cfg *ResponderConfig) (*Responder, error) {
	switch {
	case cfg == nil, cfg.Address == 0, cfg.Address>>24 != 0:
		return nil, errAddress
	case cfg.ColorCode > 15:
		return nil, errColorCode
	case cfg.Access > enums.AccessImpolite:
		return nil, errAccess
	case cfg.OACSU > OACSUNotSupported:
		return nil, errOACSU
	}
	if _, _, ok := cfg.Source.SyncPatterns(); !ok {
		return nil, errSource
	}
	r := &Responder{address: cfg.Address, oacsu: cfg.OACSU, noRadio: cfg.NoRadioCheck, noAlert: cfg.NoCallAlert, noFNS: cfg.NoFNS}
	r.s.init(cfg.ColorCode, cfg.Source, cfg.IdleSearch, cfg.HoldoffMax, cfg.Seed)
	r.s.access = cfg.Access
	r.d = newDecoder(cfg.Scratch)
	return r, nil
}

// Reset drops the pending answer and forgets the channel state. It keeps
// the config.
func (r *Responder) Reset() {
	if r == nil {
		return
	}
	r.s.stop()
	r.s.busy = false
}

// Pending reports whether an answer waits to go out.
func (r *Responder) Pending() bool {
	if r == nil {
		return false
	}
	return r.s.state != sendIdle
}

// Channel reports whether the logical channel is busy, as
// Requester.Channel does.
func (r *Responder) Channel(now time.Duration, busy bool) {
	if r == nil {
		return
	}
	r.s.channel(now, busy)
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Responder as Decoded does, and appends the outputs to dst. A
// nil raw, a transport frame without a burst, is a nil b for Decoded. It
// allocates only when dst has no room for MaxOutputs outputs.
func (r *Responder) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if r == nil {
		return dst
	}
	return r.Decoded(now, r.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil. For a CSBK that
// addresses the radio and that the Responder answers, it appends an
// OutputRequest to dst and queues the answer, which Tick sends. A CSBK
// whose CRC fails is dropped. It allocates only when dst has no room for
// MaxOutputs outputs.
func (r *Responder) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if r == nil {
		return dst
	}
	dst = r.Tick(now, dst)
	kind, c, octets := classify(b)
	if kind != rxCSBK {
		return dst
	}
	target, source, ok := addresses(c)
	if !ok || target != r.address || source == r.address {
		return dst
	}
	if group, ok := octets.Group(); ok && group {
		return dst
	}
	svc, answer, timeCritical, ok := r.answerFor(c, target, source)
	if !ok {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputRequest)
	o.Service, o.Target, o.Source, o.CSBK = svc, target, source, octets
	if !r.answerReady {
		return dst
	}
	r.service, r.reqTarget, r.reqSource = svc, target, source
	r.s.timeCritical = timeCritical
	r.s.start(now, answer, source, target, false, 0)
	return dst
}

func (r *Responder) answerFor(c *pdu.CSBK, target, source uint32) (svc Service, answer pdu.CSBKBytes, timeCritical, ok bool) {
	r.answerReady = false
	if c.FID == enums.FIDMotorola {
		switch {
		case c.Opcode == pdu.CSBKMotorolaRadioCheck && !r.noRadio:
			p, ok := c.AsMotorolaRadioCheck()
			if !ok || p.Answer {
				return 0, answer, false, false
			}
			p.Answer = true
			r.build = pdu.CSBK{LastBlock: true}
			if r.build.SetMotorolaRadioCheck(p) != nil {
				return 0, answer, false, false
			}
			return ServiceRadioCheck, r.encode(), true, true
		case c.Opcode == pdu.CSBKMotorolaCallAlert && !r.noAlert:
			first, _ := c.Encode()
			r.build = pdu.CSBK{LastBlock: true}
			if r.build.SetMotorolaCallAlertAck(pdu.MotorolaCallAlertAckPDU{Request: first[0], TargetAddress: source, SourceAddress: target}) != nil {
				return 0, answer, false, false
			}
			return ServiceCallAlert, r.encode(), true, true
		}
		return 0, answer, false, false
	}
	if c.FID != enums.FIDStandardized {
		return 0, answer, false, false
	}
	switch c.Opcode {
	case pdu.CSBKUnitToUnitVoiceServiceRequest:
		so := layer3.ServiceOptions{}
		if p := c.UnitToUnitVoiceServiceRequestPDU; p != nil {
			so = p.ServiceOptions
		}
		switch r.oacsu {
		case OACSUProceed, OACSUDeny:
			resp := byte(answerProceed)
			if r.oacsu == OACSUDeny {
				resp = answerDeny
			}
			r.uuAns = pdu.UnitToUnitVoiceServiceAnswerResponsePDU{ServiceOptions: so, AnswerResponse: resp, TargetAddress: source, SourceAddress: target}
			r.build = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKUnitToUnitVoiceServiceAnswerResponse, UnitToUnitVoiceServiceAnswerResponsePDU: &r.uuAns}
			return ServiceOACSU, r.encode(), true, true
		case OACSUNotSupported:
		}
	case pdu.CSBKPreamble, pdu.CSBKUnitToUnitVoiceServiceAnswerResponse, pdu.CSBKNegativeAcknowledgement,
		pdu.CSBKBSOutboundActivation, pdu.CSBKChannelTiming:
		return 0, answer, false, false
	default:
	}
	if r.noFNS {
		return ServiceUnsupported, answer, false, true
	}
	r.nack = pdu.NegativeAcknowledgementPDU{
		AdditionalInfo: layer3.AdditionalInfoValid, SourceType: layer3.SourceTypeMS,
		ServiceType: c.Opcode, ReasonCode: byte(ReasonNotSupported), SourceAddress: target, TargetAddress: source,
	}
	r.build = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKNegativeAcknowledgement, NegativeAcknowledgementPDU: &r.nack}
	return ServiceUnsupported, r.encode(), false, true
}

func (r *Responder) encode() pdu.CSBKBytes {
	octets, err := r.build.Encode()
	r.answerReady = err == nil
	return pdu.CSBKBytes(octets)
}

// Lost tells the Responder that the transport lost the signal: the channel
// counts as idle. It runs the timers as Tick does, and allocates only when
// dst has no room for MaxOutputs outputs.
func (r *Responder) Lost(now time.Duration, dst []Output) []Output {
	if r == nil {
		return dst
	}
	r.s.channel(now, false)
	return r.Tick(now, dst)
}

// Tick runs the timers that are due at now and appends their outputs to
// dst: the burst of the answer, or an OutputDone with ResultDenied for a
// time critical answer that the channel denied. A Tick before Deadline does
// nothing. It allocates only when dst has no room for MaxOutputs outputs.
func (r *Responder) Tick(now time.Duration, dst []Output) []Output {
	if r == nil {
		return dst
	}
	if r.s.state == sendIdle {
		return dst
	}
	dst, ev := r.s.tick(now, dst)
	if ev == sendDenied {
		var o *Output
		dst, o = appendOutput(dst, OutputDone)
		o.Service, o.Result, o.Target, o.Source = r.service, ResultDenied, r.reqTarget, r.reqSource
	}
	return dst
}

// Deadline returns the time at which the Responder needs a Tick. ok is
// false when no answer waits.
func (r *Responder) Deadline() (at time.Duration, ok bool) {
	if r == nil {
		return 0, false
	}
	return r.s.deadline()
}
