package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxPayloadChannels is the most payload channels of a TSCC.
const MaxPayloadChannels = 16

// MaxTSCCOutputs is the most outputs that one call of a TSCC method
// appends: one burst for the TSCC, one burst and one call end for each
// payload channel, and the events, such as a refusal for each
// authentication challenge and call set-up that times out.
const MaxTSCCOutputs = 1 + 2*MaxPayloadChannels + maxEvents

const (
	queueLen   = 32
	maxSetups  = 4
	maxEvents  = 2*maxSetups + svcEvents
	slotLength = 30 * time.Millisecond
)

// TSCCConfig configures NewTSCC. SysCode and at least one payload channel
// are required.
type TSCCConfig struct {
	// SysCode is the C_SYScode of the site.
	SysCode pdu.SysCode
	// ColorCode is the color code of the TSCC and its payload channels.
	ColorCode enums.ColorCode
	// Channel is the channel number of the TSCC, for the outputs of its
	// carrier.
	Channel uint16
	// Payload holds the first PayloadLen channel numbers of the payload
	// channels. Each has two slots for calls.
	Payload [MaxPayloadChannels]uint16
	// PayloadLen is the number of entries of Payload in use, 1 or more.
	PayloadLen int
	// Reg asks MSs to register, and refuses the services of an MS that is
	// not registered.
	Reg bool
	// MaxRegistrations is the size of the registration table. 0 means 64.
	MaxRegistrations int
	// NRandWait is the NRand_Wait of C_ALOHA. 0 means the code of
	// constants.NDefaultNW.
	NRandWait enums.NRandWait
	// Backoff is the backoff number of C_ALOHA. 0 means 3.
	Backoff enums.Backoff
	// GrantRepeats is the number of times the TSCC sends a grant. 0 means
	// 2.
	GrantRepeats int
	// ClearRepeats is the number of times the TSCC sends P_CLEAR. 0 means
	// 3.
	ClearRepeats int
	// LateEntry is the time between the late entry grants of a call. 0
	// means 1 s, and a negative value sends none.
	LateEntry time.Duration
	// Hangtime is TV_Hangtime, the time after the end of a transmission in
	// which the call keeps its payload slot. 0 means
	// constants.TVHangtimeDefault.
	Hangtime time.Duration
	// Inactive is TV_Inactive, the time without an inbound burst after
	// which the TSCC clears a call that never started. 0 means
	// constants.TVInactiveDefault.
	Inactive time.Duration
	// AnswerTimeout is how long the TSCC waits for the answer of a called
	// MS to its C_AHOY, and for the answer to an authentication challenge.
	// 0 means constants.TPTimerDefault.
	AnswerTimeout time.Duration
	// Authenticate challenges each MS that registers with a C_AHOY, and
	// registers it only when its answer matches the response that the
	// consumer passes to ExpectAuth after TSCCAuthChallenge. A wrong answer
	// gets C_NACKD Reg_Denied, and no answer within AnswerTimeout gets
	// C_NACKD Reg_Refused.
	Authenticate bool
	// Seed seeds the random choice of the authentication challenges.
	Seed uint64
	// Auth, when set, answers the authentication challenge of an MS before
	// a stun, revive or kill. (ETSI TS 102 361-4 clauses 6.4.9.2 and
	// 6.4.10)
	Auth Authenticator
	// TransmitInterrupt accepts the transmit interrupt requests of MSs,
	// status calls with StatusInterrupt and StatusTerminate. (ETSI TS 102
	// 361-4 clause 6.4.14.3)
	TransmitInterrupt bool
	// MaxCall is the longest time of a call. At its end the TSCC stops the
	// transmission with the reverse channel and clears the call. 0 means
	// no limit. (ETSI TS 102 361-4 clause 6.4.14.3.2)
	MaxCall time.Duration
	// PowerSave gives the MSs that ask for it a power save frame, and
	// holds the C_AHOY to an MS that sleeps until its frame. (ETSI TS 102
	// 361-4 clause 6.4.7)
	PowerSave bool
	// Awake is T_Awake, as in MSConfig. 0 means constants.TAwakeDefault.
	Awake time.Duration
	// IP offers the IP bearer service: the TSCC takes the IP connection
	// advice of MSs. Without it the TSCC refuses the advice with
	// IP_Connection_failed. (ETSI TS 102 361-4 clause 6.4.11)
	IP bool
	// Supplementary takes the supplementary_user data of call requests
	// and passes it to the called party before the call. Without it the
	// TSCC sets up the call without the data. (ETSI TS 102 361-4 clause
	// 6.4.13)
	Supplementary bool
}

// TSCCOutputKind is the kind of a TSCCOutput.
type TSCCOutputKind uint8

// The TSCCOutputKind values.
const (
	// TSCCBurst is an outbound burst with its CACH for Channel and Slot.
	TSCCBurst TSCCOutputKind = iota + 1
	// TSCCRegistered reports that the MS Address registered.
	TSCCRegistered
	// TSCCDeregistered reports that the MS Address de-registered.
	TSCCDeregistered
	// TSCCCallStart reports the grant of Call.
	TSCCCallStart
	// TSCCCallEnd reports that the TSCC cleared Call, with Reason.
	TSCCCallEnd
	// TSCCRefused reports a C_NACKD with Ack to the MS Address.
	TSCCRefused
	// TSCCAuthChallenge reports that the TSCC sent the authentication
	// Challenge to the MS Address. Pass the expected response, from
	// auth.Response, to ExpectAuth before the answer of the MS.
	TSCCAuthChallenge
	// TSCCDelivered reports that a UDT short data, polling or DGNA
	// transfer from Address to Call.Target completed, with the final
	// acknowledgement in Ack.
	TSCCDelivered
	// TSCCUDT reports the UDT from the MS Address that answers
	// PollShortData. UDT returns it.
	TSCCUDT
	// TSCCUSBD reports a USBD Poll Response, to the poll of the MS
	// Address. USBD returns it.
	TSCCUSBD
	// TSCCDiversion reports that the MS Address set or cleared a call
	// diversion. Diverted returns it.
	TSCCDiversion
	// TSCCStatus reports a status call with Status between the parties in
	// Call, and the answer of the called MS in Ack. Reason is
	// ReasonTimeout when the called MS did not answer.
	TSCCStatus
	// TSCCStun reports the end of Stun or Kill for the MS Address: Stun
	// tells the action and Ack holds the final answer of the MS. Reason is
	// ReasonTimeout when no answer came, and ReasonAuthentication when the
	// authentication failed.
	TSCCStun
	// TSCCInterrupt reports the end of a transmit interrupt of Call for the
	// MS Address, or for the TSCC when Address is 0. Reason is
	// ReasonTimeout when the transmission did not stop.
	TSCCInterrupt
	// TSCCIPAdvice reports the IP connection advice of the MS Address, in
	// IP, IPv6 and Remove.
	TSCCIPAdvice
)

// String returns the name of the TSCCOutputKind.
func (k TSCCOutputKind) String() string {
	switch k {
	case TSCCBurst:
		return "Burst"
	case TSCCRegistered:
		return "Registered"
	case TSCCDeregistered:
		return "Deregistered"
	case TSCCCallStart:
		return "CallStart"
	case TSCCCallEnd:
		return "CallEnd"
	case TSCCRefused:
		return "Refused"
	case TSCCAuthChallenge:
		return "AuthChallenge"
	case TSCCDelivered:
		return "Delivered"
	case TSCCUDT:
		return "UDT"
	case TSCCUSBD:
		return "USBD"
	case TSCCDiversion:
		return "Diversion"
	case TSCCStatus:
		return "Status"
	case TSCCStun:
		return "Stun"
	case TSCCInterrupt:
		return "Interrupt"
	case TSCCIPAdvice:
		return "IPAdvice"
	}
	return unknownName
}

// TSCCOutput is one result of a TSCC. Fields that do not apply to Kind are
// zero.
type TSCCOutput struct {
	// Kind is the kind of the output.
	Kind TSCCOutputKind
	// Channel is the channel number of the carrier of TSCCBurst.
	Channel uint16
	// Slot is the TDMA channel of TSCCBurst.
	Slot enums.Slot
	// CACH is the CACH before the burst of TSCCBurst, in transmit order.
	CACH [3]byte
	// Burst is the burst of TSCCBurst.
	Burst layer2.RawBurst
	// Address is the MS of TSCCRegistered, TSCCDeregistered,
	// TSCCRefused, TSCCDelivered, TSCCUDT, TSCCDiversion, TSCCStun,
	// TSCCInterrupt and TSCCIPAdvice.
	Address uint32
	// Ack is the reason of TSCCRefused and TSCCDelivered, and the answer of
	// the MS for TSCCStatus and TSCCStun.
	Ack enums.AckReason
	// Call is the call of TSCCCallStart, TSCCCallEnd and TSCCInterrupt,
	// and the parties of TSCCStatus.
	Call Grant
	// Reason tells why for TSCCCallEnd, TSCCStatus, TSCCStun and
	// TSCCInterrupt.
	Reason Reason
	// Challenge is the 24-bit challenge of TSCCAuthChallenge.
	Challenge uint32
	// Status is the status of TSCCStatus.
	Status Status
	// Stun is the action of TSCCStun.
	Stun StunAction
	// IP holds the IPv4 address of TSCCIPAdvice in its first 4 octets, or
	// the IPv6 address when IPv6 is set.
	IP [16]byte
	// IPv6 reports an IPv6 address in IP.
	IPv6 bool
	// Remove reports that the MS removes the address, rather than adds it.
	Remove bool
}

type queued struct {
	p     [12]byte
	reply bool
	addr  uint32
}

type callSlot struct {
	active    bool
	grant     Grant
	last      time.Duration
	hangEnd   time.Duration
	inHang    bool
	started   bool
	clears    int
	lateNext  time.Duration
	relay     layer2.RawBurst
	hasRelay  bool
	clearPDU  [12]byte
	endReason Reason
}

type authWait struct {
	used      bool
	addr      uint32
	challenge uint32
	expect    uint32
	hasExpect bool
	ps        uint8
	until     time.Duration
}

type setup struct {
	used   bool
	caller uint32
	called uint32
	until  time.Duration
	opts   uint8
}

// TSCC is a Tier III dedicated TSCC with its payload channels, for a
// single site (ETSI TS 102 361-4 clauses 6.2, 6.4.4, 6.4.8 and 6.6). It sends
// C_ALOHA and the C_SYS_Parms Short LC, reserves the inbound slot of a
// PDU that solicits an answer with the AT bit, registers MSs, sets up
// talkgroup voice calls and individual voice calls with OACSU, sends
// grants and late entry grants, relays the inbound bursts of each call
// on its payload slot, clears calls with P_CLEAR, and authenticates the
// MSs that register when TSCCConfig.Authenticate is set. It also carries
// UDT short data, UDT and USBD polling, call diversion and DGNA, sets up
// full-duplex calls and ALS calls, and runs power save, mass registration,
// stun, revive and kill, status calls, transmit interrupts, IP connection
// advice and supplementary_user data (ETSI TS 102 361-4 clauses 6.4.5 to
// 6.4.15, 6.6.2.6 and 6.6.4 to 6.6.11). Half-duplex data calls and FOACSU
// are not supported. Make it with NewTSCC.
// A TSCC is not safe for concurrent use and must not be copied; go vet
// reports a copy.
type TSCC struct {
	cfg     TSCCConfig
	code    uint16
	netSite uint16
	regs    []uint32
	nRegs   int
	queue   [queueLen]queued
	qHead   int
	qLen    int
	calls   [2 * MaxPayloadChannels]callSlot
	setups  [maxSetups]setup
	auths   [maxSetups]authWait
	rng     pcg.Source
	slot    enums.Slot
	ticks   uint32
	next    time.Duration
	counter uint16
	caches  [1 + MaxPayloadChannels]bs.CACH
	b       *layer2.Burst
	cs      pdu.CSBK
	aloha   pdu.AlohaPDU
	ack     pdu.AckOutboundPDU
	ahoy    pdu.AhoyPDU
	tv      pdu.TalkgroupVoiceGrantPDU
	pv      pdu.PrivateVoiceGrantPDU
	clear   pdu.ClearPDU
	slc     pdu.ShortLC
	csys    pdu.ShortLCCSysParms
	psys    pdu.ShortLCPSysParms
	u       tsccUDT
	svc     tsccSvc
	noCopy  noCopy
}

// NewTSCC returns a TSCC for cfg. It returns a *dmrgo.ConfigError for a
// SysCode whose fields do not fit, a color code over 15, a PayloadLen out of
// range or a payload channel that names no channel. It allocates the TSCC
// and its registration table.
func NewTSCC(cfg *TSCCConfig) (*TSCC, error) {
	if cfg == nil {
		return nil, errChannels
	}
	t := &TSCC{cfg: *cfg, slot: enums.Slot2}
	c := &t.cfg
	var err error
	if t.code, err = c.SysCode.Pack(); err != nil {
		return nil, errSysCode
	}
	t.netSite, _ = c.SysCode.NetSite()
	if c.ColorCode > 15 {
		return nil, errColorCode
	}
	if c.PayloadLen < 1 || c.PayloadLen > MaxPayloadChannels {
		return nil, errChannels
	}
	for i := range c.PayloadLen {
		if ch := c.Payload[i]; ch == ChannelNull || ch >= ChannelAbsolute {
			return nil, errChannels
		}
	}
	if c.MaxRegistrations < 0 {
		return nil, errRegistrations
	}
	if c.MaxRegistrations == 0 {
		c.MaxRegistrations = 64
	}
	if c.NRandWait == 0 {
		c.NRandWait = constants.NDefaultNW
	}
	if c.Backoff == 0 {
		c.Backoff = 3
	}
	if c.GrantRepeats <= 0 {
		c.GrantRepeats = 2
	}
	if c.ClearRepeats <= 0 {
		c.ClearRepeats = 3
	}
	if c.LateEntry == 0 {
		c.LateEntry = time.Second
	}
	if c.Hangtime <= 0 {
		c.Hangtime = constants.TVHangtimeDefault
	}
	if c.Inactive <= 0 {
		c.Inactive = constants.TVInactiveDefault
	}
	if c.AnswerTimeout <= 0 {
		c.AnswerTimeout = constants.TPTimerDefault
	}
	t.regs = make([]uint32, c.MaxRegistrations)
	t.rng = pcg.New(c.Seed)
	t.b = layer2.NewBurst()
	t.initSvc()
	return t, nil
}

// Registered reports whether the MS addr is in the registration table.
func (t *TSCC) Registered(addr uint32) bool {
	return t.regIndex(addr) >= 0
}

// Deadline returns the time of the next Tick: the TSCC sends one burst
// per 30 ms slot. It is the time of the last Tick plus 30 ms, or 0 before
// the first Tick.
func (t *TSCC) Deadline() (at time.Duration, ok bool) {
	return t.next, true
}

// Inbound takes the burst raw that the TSCC received on the inbound
// channel of carrier ch in slot s, and appends the outputs to dst. On the
// TSCC it answers C_RAND, C_ACKU and C_NACKU; on a payload slot it keeps
// the burst for the relay of the call and ends the call on P_MAINT. It
// allocates only when dst has no room for MaxTSCCOutputs outputs.
func (t *TSCC) Inbound(now time.Duration, ch uint16, s enums.Slot, raw *layer2.RawBurst, h layer2.Hint, dst []TSCCOutput) []TSCCOutput {
	out := dst
	opts := layer2.DecodeOptions{Kind: h.Kind}
	if t.b.DecodeFromBytesWith(*raw, &opts) != nil {
		return out
	}
	if ch != t.cfg.Channel {
		return t.payloadIn(now, ch, s, raw, out)
	}
	b := t.b
	if b.Kind != layer2.KindData || b.FEC.SlotType.Uncorrectable || b.FEC.PDU.Uncorrectable || b.SlotType.ColorCode != t.cfg.ColorCode {
		return out
	}
	if o, done := t.udtInbound(now, s, out); done {
		return o
	}
	if o, done := t.svcInbound(now, s, out); done {
		return o
	}
	c, ok := b.Data.(*pdu.CSBK)
	if !ok || c.FID != enums.FIDStandardized {
		return out
	}
	switch {
	case c.RandomAccessPDU != nil:
		return t.random(now, c.RandomAccessPDU, out)
	case c.AckInboundPDU != nil:
		return t.answer(now, c.AckInboundPDU, out)
	}
	return out
}

func (t *TSCC) payloadIn(now time.Duration, ch uint16, s enums.Slot, raw *layer2.RawBurst, out []TSCCOutput) []TSCCOutput {
	i := t.callIndex(ch, s)
	if i < 0 || !t.calls[i].active || t.calls[i].clears > 0 {
		return out
	}
	cs := &t.calls[i]
	b := t.b
	if b.Kind == layer2.KindData && (b.FEC.SlotType.Uncorrectable || b.SlotType.ColorCode != t.cfg.ColorCode) {
		return out
	}
	cs.last, cs.started = now, true
	if b.Kind == layer2.KindData {
		switch b.SlotType.DataType {
		case enums.DataTypeCSBK:
			if c, ok := b.Data.(*pdu.CSBK); ok && !b.FEC.PDU.Uncorrectable && c.MaintenancePDU != nil &&
				(c.MaintenancePDU.SourceAddress == cs.grant.Source || duplex(&cs.grant) && c.MaintenancePDU.SourceAddress == cs.grant.Target) {
				return t.startClear(i, ReasonMaintenance, out)
			}
			return out
		case enums.DataTypeTerminatorWithLC:
			cs.inHang, cs.hangEnd = true, now+t.cfg.Hangtime
		default:
			cs.inHang = false
		}
	} else {
		cs.inHang = false
	}
	rs := t.relaySlot(i)
	t.svcRelay(rs)
	t.calls[rs].relay, t.calls[rs].hasRelay = *raw, true
	return out
}

func (t *TSCC) random(now time.Duration, r *pdu.RandomAccessPDU, out []TSCCOutput) []TSCCOutput {
	if o, done := t.udtRandom(now, r, out); done {
		return o
	}
	src := r.SourceAddress
	switch enums.ServiceKind(r.ServiceKind) {
	case enums.ServiceRegistration:
		return t.registration(now, r, out)
	case enums.ServiceTalkgroupVoice, enums.ServiceIndividualVoice:
	default:
		return t.refuse(src, enums.ReasonNotSupported, out)
	}
	if t.cfg.Reg && t.regIndex(src) < 0 {
		return t.refuse(src, enums.ReasonMSNotRegistered, out)
	}
	group := enums.ServiceKind(r.ServiceKind) == enums.ServiceTalkgroupVoice
	if i := t.findCall(group, r.TargetAddress, src); i >= 0 {
		t.sendGrant(i)
		return out
	}
	if group {
		return t.grant(now, true, r.TargetAddress, src, out)
	}
	if t.cfg.Reg && t.regIndex(r.TargetAddress) < 0 {
		return t.refuse(src, enums.ReasonNoregMSAwayRefused, out)
	}
	for i := range t.setups {
		if t.setups[i].used && t.setups[i].caller == src {
			return out
		}
	}
	for i := range t.setups {
		st := &t.setups[i]
		if st.used {
			continue
		}
		*st = setup{used: true, caller: src, called: r.TargetAddress, until: now + t.cfg.AnswerTimeout, opts: r.ServiceOptions}
		t.sendAck(src, enums.ReasonWait)
		t.ahoy = pdu.AhoyPDU{ServiceOptsMirror: r.ServiceOptions, ALS: r.Reserved&alsBit != 0, ServiceKind: uint8(enums.ServiceIndividualVoice),
			TargetAddress: r.TargetAddress, SourceAddress: src}
		t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &t.ahoy}
		t.enqueueTo(r.TargetAddress, true)
		return out
	}
	return t.refuse(src, enums.ReasonSysBusyRefused, out)
}

func (t *TSCC) challenge(now time.Duration, src uint32, opts, ps uint8, out []TSCCOutput) []TSCCOutput {
	w := -1
	for i := range t.auths {
		if t.auths[i].used && t.auths[i].addr == src {
			w = i
			break
		}
		if w < 0 && !t.auths[i].used {
			w = i
		}
	}
	if w < 0 {
		return t.refuse(src, enums.ReasonRegRefused, out)
	}
	c := t.rng.Uint32() % (maxChallenge + 1)
	t.auths[w] = authWait{used: true, addr: src, challenge: c, ps: ps, until: now + t.cfg.AnswerTimeout}
	t.ahoy = pdu.AhoyPDU{ServiceOptsMirror: opts, ServiceKind: uint8(enums.ServiceRegistration), TargetAddress: src, SourceAddress: c}
	t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &t.ahoy}
	t.enqueue(true)
	return append(out, TSCCOutput{Kind: TSCCAuthChallenge, Address: src, Challenge: c})
}

// ExpectAuth sets response as the answer that the TSCC accepts from the MS
// addr to the challenge of the last TSCCAuthChallenge for addr. It does
// nothing when no challenge to addr waits. (ETSI TS 102 361-4 clause
// 6.4.8.2)
func (t *TSCC) ExpectAuth(addr, response uint32) {
	for i := range t.auths {
		if w := &t.auths[i]; w.used && w.addr == addr {
			w.expect, w.hasExpect = response&0xFFFFFF, true
		}
	}
}

func (t *TSCC) authAnswer(now time.Duration, a *pdu.AckInboundPDU, out []TSCCOutput) []TSCCOutput {
	for i := range t.auths {
		w := &t.auths[i]
		if !w.used || w.addr != a.AdditionalInfo {
			continue
		}
		ok, ps := w.hasExpect && w.expect == a.TargetAddress, w.ps
		*w = authWait{}
		if !ok {
			return t.refuse(a.AdditionalInfo, enums.ReasonRegDenied, out)
		}
		return t.admit(now, a.AdditionalInfo, ps, out)
	}
	return out
}

func (t *TSCC) answer(now time.Duration, a *pdu.AckInboundPDU, out []TSCCOutput) []TSCCOutput {
	if enums.AckReason(a.ReasonCode) == enums.ReasonAuthResponseMS {
		return t.authAnswer(now, a, out)
	}
	if o, done := t.udtAck(now, a, out); done {
		return o
	}
	called := a.AdditionalInfo
	for i := range t.setups {
		st := &t.setups[i]
		if !st.used || st.called != called {
			continue
		}
		st.used = false
		r := enums.AckReason(a.ReasonCode)
		if r.Type() == enums.AckNACK {
			return t.refuse(st.caller, r, out)
		}
		return t.grant(now, false, st.called, st.caller, out)
	}
	return out
}

func (t *TSCC) grant(now time.Duration, group bool, target, source uint32, out []TSCCOutput) []TSCCOutput {
	for i := range 2 * t.cfg.PayloadLen {
		cs := &t.calls[i]
		if cs.active {
			continue
		}
		g := Grant{Group: group, Target: target, Source: source, Channel: t.cfg.Payload[i/2], Slot: enums.Slot1 + enums.Slot(i%2),
			ColorCode: t.cfg.ColorCode, HasColorCode: true}
		g.Opcode, g.Service = pdu.CSBKPrivateVoiceGrant, enums.ServiceIndividualVoice
		if group {
			g.Opcode, g.Service = pdu.CSBKTalkgroupVoiceGrant, enums.ServiceTalkgroupVoice
		}
		*cs = callSlot{active: true, grant: g, last: now, lateNext: now + t.cfg.LateEntry}
		for range t.cfg.GrantRepeats {
			t.sendGrant(i)
		}
		return append(out, TSCCOutput{Kind: TSCCCallStart, Call: g})
	}
	return t.refuse(source, enums.ReasonSysBusyRefused, out)
}

func (t *TSCC) sendGrant(i int) {
	g := &t.calls[i].grant
	ts2 := g.Slot == enums.Slot2
	if g.Group {
		t.tv = pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: g.Channel, LogicalChannel: ts2, LateEntry: g.LateEntry, TargetAddress: g.Target, SourceAddress: g.Source}
		t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &t.tv}
	} else {
		t.pv = pdu.PrivateVoiceGrantPDU{PhysicalChannel: g.Channel, LogicalChannel: ts2, TargetAddress: g.Target, SourceAddress: g.Source}
		t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &t.pv}
	}
	t.enqueue(false)
}

func (t *TSCC) refuse(addr uint32, r enums.AckReason, out []TSCCOutput) []TSCCOutput {
	t.sendAck(addr, r)
	return append(out, TSCCOutput{Kind: TSCCRefused, Address: addr, Ack: r})
}

func (t *TSCC) sendAck(addr uint32, r enums.AckReason) {
	t.ack = pdu.AckOutboundPDU{ReasonCode: byte(r), TargetAddress: addr}
	t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &t.ack}
	t.enqueue(false)
}

func (t *TSCC) enqueue(reply bool) {
	p, err := t.cs.Encode()
	if err != nil || t.qLen == queueLen {
		return
	}
	t.queue[(t.qHead+t.qLen)%queueLen] = queued{p: p, reply: reply}
	t.qLen++
}

func (t *TSCC) findCall(group bool, target, source uint32) int {
	for i := range 2 * t.cfg.PayloadLen {
		g := &t.calls[i].grant
		if t.calls[i].active && t.calls[i].clears == 0 && g.Group == group && g.Target == target && (group || g.Source == source) {
			return i
		}
	}
	return -1
}

func (t *TSCC) callIndex(ch uint16, s enums.Slot) int {
	if s != enums.Slot1 && s != enums.Slot2 {
		return -1
	}
	for i := range t.cfg.PayloadLen {
		if t.cfg.Payload[i] == ch {
			return 2*i + int(s-enums.Slot1)
		}
	}
	return -1
}

func (t *TSCC) regIndex(addr uint32) int {
	for i := range t.nRegs {
		if t.regs[i] == addr {
			return i
		}
	}
	return -1
}

func (t *TSCC) startClear(i int, reason Reason, out []TSCCOutput) []TSCCOutput {
	cs := &t.calls[i]
	g := &cs.grant
	t.clear = pdu.ClearPDU{GroupIndividual: g.Group, TargetAddress: g.Target, SourceAddress: g.Source}
	t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKClear, ClearPDU: &t.clear}
	p, err := t.cs.Encode()
	if err != nil {
		return out
	}
	cs.clearPDU, cs.clears, cs.endReason, cs.hasRelay = p, t.cfg.ClearRepeats, reason, false
	return out
}

// Tick sends the outbound bursts of one 30 ms slot: on the TSCC a queued
// PDU or a C_ALOHA, and on each payload channel with a call the relayed
// inbound burst, P_CLEAR or an idle burst, each with its CACH. It runs the
// call timers and appends the outputs to dst. The slots alternate from
// TS1 at the first Tick. It allocates only when dst has no room for
// MaxTSCCOutputs outputs.
func (t *TSCC) Tick(now time.Duration, dst []TSCCOutput) []TSCCOutput {
	out := dst
	t.slot = t.slot.Other()
	t.next = now + slotLength
	out = t.timers(now, out)
	out = t.udtTimers(now, out)
	out = t.svcTimers(now, out)
	out = t.control(out)
	for c := range t.cfg.PayloadLen {
		out = t.carrier(c, out)
	}
	t.ticks++
	if t.ticks%4 == 0 {
		t.counter = (t.counter + 1) % 512
	}
	return out
}

func (t *TSCC) timers(now time.Duration, out []TSCCOutput) []TSCCOutput {
	for i := range t.auths {
		if w := &t.auths[i]; w.used && now >= w.until {
			addr := w.addr
			*w = authWait{}
			out = t.refuse(addr, enums.ReasonRegRefused, out)
		}
	}
	for i := range t.setups {
		st := &t.setups[i]
		if st.used && now >= st.until {
			st.used = false
			out = t.refuse(st.caller, enums.ReasonMSAwayRefused, out)
		}
	}
	for i := range 2 * t.cfg.PayloadLen {
		cs := &t.calls[i]
		if !cs.active || cs.clears > 0 {
			continue
		}
		switch {
		case cs.inHang && now >= cs.hangEnd:
			out = t.startClear(i, ReasonHangtime, out)
		case !cs.inHang && now-cs.last >= t.cfg.Inactive:
			out = t.startClear(i, ReasonInactive, out)
		case t.cfg.LateEntry > 0 && now >= cs.lateNext:
			cs.lateNext = now + t.cfg.LateEntry
			cs.grant.LateEntry = cs.grant.Group
			t.sendGrant(i)
		}
	}
	return out
}

func (t *TSCC) control(out []TSCCOutput) []TSCCOutput {
	now := t.next - slotLength
	p, dt, reply, ok := t.udtNext(now)
	if !ok && (!t.u.downHold || t.u.downN == 0 || t.u.downI == 0) {
		p, dt, reply, ok = t.dequeue(now)
	}
	if !ok {
		dt = enums.DataTypeCSBK
		t.aloha = pdu.AlohaPDU{SiteTSSync: true, Version: 1, NRandWait: uint8(t.cfg.NRandWait), Reg: t.cfg.Reg, Backoff: uint8(t.cfg.Backoff), SysIdentCode: t.code}
		t.cs = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &t.aloha}
		p, _ = t.cs.Encode()
	}
	t.csys = pdu.ShortLCCSysParms{MODEL: uint8(t.cfg.SysCode.Model), NetSiteRaw: t.netSite, Reg: t.cfg.Reg, CommonSlotCounter: t.counter}
	t.slc = pdu.ShortLC{SLCO: enums.SLCOCSysParms, CSysParms: &t.csys}
	return t.emit(0, t.cfg.Channel, p, dt, reply, out)
}

func (t *TSCC) carrier(c int, out []TSCCOutput) []TSCCOutput {
	if !t.calls[2*c].active && !t.calls[2*c+1].active {
		t.caches[1+c].Reset()
		return out
	}
	cs := &t.calls[2*c+int(t.slot-enums.Slot1)]
	t.psys = pdu.ShortLCPSysParms{MODEL: uint8(t.cfg.SysCode.Model), NetSiteRaw: t.netSite, CommonSlotCounter: t.counter}
	t.slc = pdu.ShortLC{SLCO: enums.SLCOPSysParms, PSysParms: &t.psys}
	ch := t.cfg.Payload[c]
	switch {
	case cs.active && cs.clears > 0:
		cs.clears--
		p := cs.clearPDU
		if cs.clears == 0 {
			g, reason := cs.grant, cs.endReason
			*cs = callSlot{}
			out = append(out, TSCCOutput{Kind: TSCCCallEnd, Call: g, Reason: reason})
		}
		return t.emit(1+c, ch, p, enums.DataTypeCSBK, false, out)
	case cs.active && cs.hasRelay:
		cs.hasRelay = false
		raw := cs.relay
		switch sp, d := raw.Sync(); {
		case d <= layer2.DefaultSyncThreshold && sp == enums.SyncMSSourcedVoice:
			_ = raw.SetSync(enums.SyncBSSourcedVoice)
		case d <= layer2.DefaultSyncThreshold && sp == enums.SyncMSSourcedData:
			_ = raw.SetSync(enums.SyncBSSourcedData)
		}
		return t.emitRaw(1+c, ch, &raw, false, out)
	}
	return t.emit(1+c, ch, pdu.PRFillBits(), enums.DataTypeIdle, false, out)
}

func (t *TSCC) emit(idx int, ch uint16, p [12]byte, dt enums.DataType, reply bool, out []TSCCOutput) []TSCCOutput {
	raw, err := layer2.BuildDataBurst(p, dt, t.cfg.ColorCode, layer2.SyncSourceBS)
	if err != nil {
		return out
	}
	return t.emitRaw(idx, ch, &raw, reply, out)
}

func (t *TSCC) emitRaw(idx int, ch uint16, raw *layer2.RawBurst, reply bool, out []TSCCOutput) []TSCCOutput {
	t.svc.buf = *raw
	t.svcEmit(idx)
	cach, err := t.caches[idx].Next(t.slot, reply, &t.slc)
	if err != nil {
		return out
	}
	return append(out, TSCCOutput{Kind: TSCCBurst, Channel: ch, Slot: t.slot, CACH: cach, Burst: t.svc.buf})
}
