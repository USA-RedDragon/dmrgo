package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const frame = 60 * time.Millisecond

const maxAddress = 0xFFFCDF

// MSConfig configures NewMS. Address is required.
type MSConfig struct {
	// Address is the 24-bit individual address of the MS, 1 to 0xFFFCDF.
	Address uint32
	// Talkgroups holds the first TalkgroupsLen talkgroups whose calls the
	// MS follows. Its own calls and the individual calls to it are always
	// followed.
	Talkgroups [MaxTalkgroups]uint32
	// TalkgroupsLen is the number of entries of Talkgroups in use.
	TalkgroupsLen int
	// SysCode, when set, is the system to use, as in FollowerConfig.
	SysCode *pdu.SysCode
	// Hunt holds the first HuntLen channel numbers of the hunt list.
	Hunt [MaxChannels]uint16
	// HuntLen is the number of entries of Hunt in use.
	HuntLen int
	// Plan, when set, is the channel plan to start from.
	Plan *ChannelPlan
	// Slot is the TDMA channel of the TSCC on which the MS makes random
	// access. SlotUnknown means enums.Slot1.
	Slot enums.Slot
	// Seed seeds the random choice of the backoff frames. Two MSs with
	// the same seed make the same choices; 0 is a valid seed.
	Seed uint64
	// NoRegistration keeps the MS from registering when the TSCC asks for
	// it with Reg.
	NoRegistration bool
	// RandomTimeout is TRand_TC, the longest time of a random access. 0
	// means constants.TRandTCDefault, and a negative value turns the timer
	// off (ETSI TS 102 361-4 clause 6.2.1.1.7).
	RandomTimeout time.Duration
	// CallTimeout is TP_Timer, the longest time from a call request to its
	// grant. 0 means constants.TPTimerDefault, and a negative value turns
	// the timer off.
	CallTimeout time.Duration
	// Pending is T_Pending, the time a called MS waits for the grant after
	// it accepted a C_AHOY. 0 means constants.TPendingDefault.
	Pending time.Duration
	// NoSignal is T_Nosig, as in FollowerConfig.
	NoSignal time.Duration
	// Inactive is TV_Inactive, as in FollowerConfig.
	Inactive time.Duration
	// Hangtime is the payload hang time, as in FollowerConfig.
	Hangtime time.Duration
	// Scratch is the decode Burst, as in FollowerConfig.
	Scratch *layer2.Burst
}

type raState uint8

const (
	raIdle raState = iota
	raWaitSlot
	raWaitResp
)

type request uint8

const (
	reqNone request = iota
	reqRegister
	reqCall
)

// MS is a Tier III mobile station: it hunts for and follows a TSCC as a
// Follower does, registers when the TSCC asks for it, makes talkgroup and
// individual voice calls by random access, answers the C_AHOY of an
// individual call (OACSU), and ends its own calls with P_MAINT (ETSI TS
// 102 361-4 clauses 6.2, 6.4.4 and 6.6.1 to 6.6.2). The consumer sends the
// voice of a granted call with a voice.Transmitter with the MS SYNC.
// Make it with NewMS. An MS is not safe for concurrent use and must not be
// copied; go vet reports a copy.
type MS struct {
	f          Follower
	addr       uint32
	slot       enums.Slot
	rng        pcg
	noReg      bool
	randTO     time.Duration
	callTO     time.Duration
	pendingTO  time.Duration
	registered bool
	regSys     pdu.SysCode

	ra         raState
	frames     int
	retries    int
	raStart    time.Duration
	req        request
	reqUntil   time.Duration
	waitGrant  bool
	kind       enums.ServiceKind
	target     uint32
	options    uint8
	emergency  bool
	ownCall    bool
	pendingEnd time.Duration
	pending    bool

	tx      pdu.CSBK
	txRand  pdu.RandomAccessPDU
	txAck   pdu.AckInboundPDU
	txMaint pdu.MaintenancePDU
	noCopy  noCopy
}

// NewMS returns an MS for cfg. It returns a *ConfigError for an Address
// out of range and for the errors of NewFollower. It allocates the MS, and
// its Burst unless cfg.Scratch is set.
func NewMS(cfg *MSConfig) (*MS, error) {
	if cfg == nil || cfg.Address == 0 || cfg.Address > maxAddress {
		return nil, errAddress
	}
	m := &MS{addr: cfg.Address, slot: cfg.Slot, noReg: cfg.NoRegistration, randTO: cfg.RandomTimeout, callTO: cfg.CallTimeout, pendingTO: cfg.Pending}
	err := m.f.init(&FollowerConfig{
		SysCode: cfg.SysCode, Hunt: cfg.Hunt, HuntLen: cfg.HuntLen, Plan: cfg.Plan, Talkgroups: cfg.Talkgroups,
		TalkgroupsLen: cfg.TalkgroupsLen, Individual: true, NoSignal: cfg.NoSignal, Inactive: cfg.Inactive,
		Hangtime: cfg.Hangtime, Scratch: cfg.Scratch,
	})
	if err != nil {
		return nil, err
	}
	m.f.self = cfg.Address
	if m.slot == enums.SlotUnknown {
		m.slot = enums.Slot1
	}
	if m.randTO == 0 {
		m.randTO = constants.TRandTCDefault
	}
	if m.callTO == 0 {
		m.callTO = constants.TPTimerDefault
	}
	if m.pendingTO <= 0 {
		m.pendingTO = constants.TPendingDefault
	}
	m.rng.seed(cfg.Seed)
	return m, nil
}

// State returns the channel state of the MS.
func (m *MS) State() State {
	return m.f.state
}

// Site returns what the MS knows about the TSCC.
func (m *MS) Site() Site {
	return m.f.ctl.site
}

// Registered reports whether the TSCC that the MS is on accepted its
// registration.
func (m *MS) Registered() bool {
	return m.registered && m.f.ctl.site.HasSysCode && m.regSys == m.f.ctl.site.SysCode
}

// Busy reports whether a registration or call request runs.
func (m *MS) Busy() bool {
	return m.req != reqNone
}

// Reset forgets the site, the registration and any request, as after
// NewMS. It keeps the config.
func (m *MS) Reset() {
	m.f.Reset()
	m.registered, m.ra, m.req, m.pending, m.ownCall, m.waitGrant = false, raIdle, reqNone, false, false, false
}

// Call asks the TSCC for a voice call to the talkgroup or MS target, by
// random access. The outputs follow from Burst and Tick: OutputGrant and
// OutputTune for the grant, or OutputRefused. It returns ErrNotOnControl
// when the MS is not on a confirmed TSCC, ErrBusy while another request
// runs, and a *ConfigError for a target or options out of range.
func (m *MS) Call(now time.Duration, group bool, target uint32, opts layer3.VoiceServiceOptions) error {
	if m.f.state != StateControl {
		return ErrNotOnControl
	}
	if m.req != reqNone {
		return ErrBusy
	}
	field, err := opts.Field()
	if err != nil || target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	m.kind = enums.ServiceIndividualVoice
	if group {
		m.kind = enums.ServiceTalkgroupVoice
	}
	m.startAccess(now, reqCall, target, field, opts.IsEmergency)
	return nil
}

// Cancel stops the running request. It appends OutputRefused with
// ReasonCancelled when one ran, and allocates only when dst has no room
// for MaxOutputs outputs.
func (m *MS) Cancel(now time.Duration, dst []Output) []Output {
	dst = m.timers(now, dst)
	if m.req == reqNone {
		return dst
	}
	return m.fail(ReasonCancelled, 0, dst)
}

// EndCall ends the own call on the payload channel: it appends the N_Maint
// P_MAINT DISCON bursts, one per TDMA frame from now, and returns to the
// TSCC (ETSI TS 102 361-4 clause 6.6.2.3). It does nothing outside the own
// call, and allocates only when dst has no room for MaxOutputs outputs.
func (m *MS) EndCall(now time.Duration, dst []Output) []Output {
	dst = m.timers(now, dst)
	if m.f.state != StatePayload || !m.ownCall {
		return dst
	}
	g := &m.f.call
	m.txMaint = pdu.MaintenancePDU{TargetAddress: g.Target, SourceAddress: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKMaintenance, MaintenancePDU: &m.txMaint}
	for i := range constants.NMaint {
		dst = m.send(now+time.Duration(i)*frame, g.Slot, dst)
	}
	m.ownCall = false
	return m.f.leave(now, ReasonMaintenance, dst)
}

// Burst decodes the burst raw with what the Hint h says about it, and the
// CACH before it when cach is not nil, as Follower.Burst does, runs the
// random access and the requests, and appends the outputs to dst. On the
// TSCC the MS counts TDMA frames by the bursts of its Slot, so h.Slot
// should be set; random access needs the CACH, for its AT bit. It
// allocates only when dst has no room for MaxOutputs outputs.
func (m *MS) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *layer2.CACH, dst []Output) []Output {
	dst = m.timers(now, dst)
	before := m.f.state
	n := len(dst)
	dst = m.f.Burst(now, raw, h, cach, dst)
	dst = m.follow(now, before, dst, n)
	if m.f.state != StateControl || before != StateControl {
		return dst
	}
	dst = m.addressed(now, &h, dst)
	if h.Slot == enums.SlotUnknown || h.Slot == m.slot {
		dst = m.frame(now, cach, dst)
	}
	return dst
}

// Lost tells the MS that the receiver lost the signal, as Follower.Lost
// does. It allocates only when dst has no room for MaxOutputs outputs.
func (m *MS) Lost(now time.Duration, dst []Output) []Output {
	dst = m.timers(now, dst)
	n := len(dst)
	before := m.f.state
	dst = m.f.Lost(now, dst)
	return m.follow(now, before, dst, n)
}

// Tick runs the timers that are due at now and appends their outputs to
// dst. A Tick before Deadline does nothing. It allocates only when dst has
// no room for MaxOutputs outputs.
func (m *MS) Tick(now time.Duration, dst []Output) []Output {
	return m.timers(now, dst)
}

// Deadline returns the time at which the MS needs a Tick. ok is false
// when no timer runs.
func (m *MS) Deadline() (at time.Duration, ok bool) {
	at, ok = m.f.Deadline()
	if m.ra == raWaitSlot && m.randTO > 0 && (!ok || m.raStart+m.randTO < at) {
		at, ok = m.raStart+m.randTO, true
	}
	if m.waitGrant && m.callTO > 0 && (!ok || m.reqUntil < at) {
		at, ok = m.reqUntil, true
	}
	if m.pending && (!ok || m.pendingEnd < at) {
		at, ok = m.pendingEnd, true
	}
	return at, ok
}

func (m *MS) timers(now time.Duration, out []Output) []Output {
	n := len(out)
	before := m.f.state
	out = m.f.timers(now, out)
	out = m.follow(now, before, out, n)
	if m.ra == raWaitSlot && m.randTO > 0 && now >= m.raStart+m.randTO {
		out = m.fail(ReasonNoAnswer, 0, out)
	}
	if m.waitGrant && m.callTO > 0 && now >= m.reqUntil {
		out = m.fail(ReasonTimeout, 0, out)
	}
	if m.pending && now >= m.pendingEnd {
		m.pending = false
	}
	return out
}

func (m *MS) follow(now time.Duration, before State, out []Output, n int) []Output {
	for i := n; i < len(out); i++ {
		switch o := &out[i]; o.Kind {
		case OutputSite:
			if m.req == reqNone && !m.noReg && m.f.ctl.site.Reg && !m.Registered() {
				m.startAccess(now, reqRegister, constants.GatewayREGI, regOptions(), false)
			}
		case OutputLost:
			if m.req != reqNone {
				return m.fail(ReasonNoSignal, 0, out)
			}
		case OutputTune:
			if before == StateControl && m.f.state == StatePayload {
				g := &m.f.call
				m.ownCall = g.Source == m.addr
				if m.req == reqCall && m.ownCall {
					m.done()
				}
				m.pending = false
			}
		case OutputGrant, OutputCallEnd, OutputBurst, OutputRegistered, OutputRefused, OutputAhoy, OutputUDT, OutputProtect, OutputPowerOff:
		}
	}
	return out
}

func regOptions() uint8 {
	v, _ := layer3.RegistrationServiceOptions{Register: true}.Field()
	return v
}

func (m *MS) startAccess(now time.Duration, req request, target uint32, options uint8, emergency bool) {
	if req == reqRegister {
		m.kind = enums.ServiceRegistration
	}
	m.req, m.target, m.options, m.emergency = req, target, options, emergency
	m.ra, m.frames, m.retries, m.raStart, m.waitGrant = raWaitSlot, 1, 0, now, false
}

func (m *MS) frame(now time.Duration, cach *layer2.CACH, out []Output) []Output {
	switch m.ra {
	case raIdle:
		return out
	case raWaitResp:
		m.frames--
		if m.frames > 0 {
			return out
		}
		if m.retries >= m.maxRetries() || (m.randTO > 0 && now >= m.raStart+m.randTO) {
			return m.fail(ReasonNoAnswer, 0, out)
		}
		m.ra, m.frames = raWaitSlot, m.rng.between(m.f.ctl.site.Access.Backoff.Frames())
		return out
	case raWaitSlot:
	}
	m.frames--
	if m.frames > 0 {
		return out
	}
	if !m.mayAccess(cach) {
		m.frames = m.rng.between(m.f.ctl.site.Access.Backoff.Frames())
		return out
	}
	m.txRand = pdu.RandomAccessPDU{ServiceOptions: m.options, ServiceKind: uint8(m.kind), TargetAddress: m.target, SourceAddress: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKRandomAccess, RandomAccessPDU: &m.txRand}
	m.retries++
	m.ra, m.frames = raWaitResp, m.f.ctl.site.Access.NRandWait.Frames()+1
	return m.send(now+frame, m.slot, out)
}

func (m *MS) mayAccess(cach *layer2.CACH) bool {
	s := &m.f.ctl.site
	if cach == nil || cach.TACT.FEC.Uncorrectable || cach.TACT.AccessType || !s.HasAccess {
		return false
	}
	if !s.Access.Applies(m.addr, m.emergency) {
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
	b := m.f.ctl.b
	if !m.f.ctl.okData() {
		return out
	}
	c, ok := b.Data.(*pdu.CSBK)
	if !ok || c.FID != enums.FIDStandardized {
		return out
	}
	switch {
	case c.AckOutboundPDU != nil && c.AckOutboundPDU.TargetAddress == m.addr:
		return m.answer(now, enums.AckReason(c.AckOutboundPDU.ReasonCode), out)
	case c.AhoyPDU != nil && c.AhoyPDU.TargetAddress == m.addr && !c.AhoyPDU.GroupIndividual:
		return m.ahoy(now, c.AhoyPDU, h, out)
	}
	return out
}

func (m *MS) answer(now time.Duration, r enums.AckReason, out []Output) []Output {
	if m.req == reqNone {
		return out
	}
	m.ra = raIdle
	switch r.Type() {
	case enums.AckNACK:
		return m.fail(ReasonNack, r, out)
	case enums.AckACK:
		if m.req == reqRegister {
			m.registered, m.regSys = true, m.f.ctl.site.SysCode
			m.done()
			return append(out, Output{Kind: OutputRegistered, Ack: r})
		}
		m.waitGrant, m.reqUntil = true, now+m.callTO
	case enums.AckQACK, enums.AckWACK:
		m.waitGrant, m.reqUntil = true, now+m.callTO
	}
	return out
}

func (m *MS) ahoy(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	if enums.ServiceKind(a.ServiceKind) != enums.ServiceIndividualVoice || m.req != reqNone {
		return out
	}
	out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: enums.ServiceIndividualVoice, Target: a.TargetAddress, Source: a.SourceAddress}})
	m.txAck = pdu.AckInboundPDU{ReasonCode: byte(enums.ReasonMSAccepted), TargetAddress: a.SourceAddress, AdditionalInfo: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckInbound, AckInboundPDU: &m.txAck}
	slot := h.Slot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	m.pending, m.pendingEnd = true, now+m.pendingTO
	return m.send(now+frame, slot, out)
}

func (m *MS) send(due time.Duration, slot enums.Slot, out []Output) []Output {
	p, err := m.tx.Encode()
	if err != nil {
		return out
	}
	raw, err := layer2.BuildDataBurst(p, enums.DataTypeCSBK, m.f.ctl.site.ColorCode, layer2.SyncSourceMS)
	if err != nil {
		return out
	}
	return append(out, Output{Kind: OutputBurst, Burst: raw, Slot: slot, Due: due})
}

func (m *MS) done() {
	m.req, m.ra, m.waitGrant = reqNone, raIdle, false
}

func (m *MS) fail(reason Reason, ack enums.AckReason, out []Output) []Output {
	m.done()
	return append(out, Output{Kind: OutputRefused, Reason: reason, Ack: ack})
}
