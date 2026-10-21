package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const frame = 60 * time.Millisecond

const maxAddress = 0xFFFCDF

const berWindow = 32

// MaxDenied is the number of entries of the denied registration list of
// an MS.
const MaxDenied = 8

// DefaultMaxBERPermille is the bit error rate of the TSCC, in parts per
// thousand of the FEC-checked bits of the last 32 TSCC bursts, above which
// an MS leaves the TSCC, when MSConfig.MaxBERPermille is 0. The spec leaves
// the threshold to the manufacturer. (ETSI TS 102 361-4 clause 6.3.2.3)
const DefaultMaxBERPermille = 50

// MSConfig configures NewMS. Address is required.
type MSConfig struct {
	// Address is the 24-bit individual address of the MS, 1 to 0xFFFCDF.
	Address uint32
	// Talkgroups holds the first TalkgroupsLen talkgroups whose calls the
	// MS follows and whose radio checks it answers. Its own calls and the
	// individual calls to it are always followed.
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
	// off. (ETSI TS 102 361-4 clause 6.2.1.1.7)
	RandomTimeout time.Duration
	// CallTimeout is TP_Timer, the longest wait for the grant of a service
	// that needs a payload channel. 0 means constants.TPTimerDefault, and a
	// negative value turns the timer off.
	CallTimeout time.Duration
	// RegTimeout is TNP_Timer, the longest wait for the answer to a
	// registration after an intermediate acknowledgement. 0 means
	// constants.TNPTimerDefault, and a negative value turns the timer off.
	RegTimeout time.Duration
	// Pending is T_Pending, the time a called MS waits for the grant after
	// it accepted a C_AHOY. 0 means constants.TPendingDefault.
	Pending time.Duration
	// Dereg is T_Dereg, the longest wait for the answer to a
	// de-registration. 0 means constants.TDeregDefault.
	Dereg time.Duration
	// DeniedLife is T_DENREG, how long a Reg_Denied keeps the MS off a
	// SYS_AREA. 0 means constants.TDenRegDefault, and a negative value
	// keeps the entry until Reset.
	DeniedLife time.Duration
	// DMRLA is the number of most significant SITE bits in the SYS_AREA of
	// the registration and denied records. 0 means constants.DMRLADefault.
	DMRLA int
	// MinLevelHunt, when set, is the signal level that Quality reports
	// below which the MS does not confirm a TSCC, as L_Lower_SHort.
	MinLevelHunt *int
	// MinLevelIdle, when set, is the signal level below which the MS
	// leaves a confirmed TSCC.
	MinLevelIdle *int
	// MaxBERPermille is the bit error rate of the TSCC above which the MS
	// leaves it. 0 means DefaultMaxBERPermille, and a negative value turns
	// the check off.
	MaxBERPermille int
	// AcceptRAS is as in FollowerConfig.
	AcceptRAS bool
	// NoSignal is T_Nosig, as in FollowerConfig.
	NoSignal time.Duration
	// Inactive is TV_Inactive, as in FollowerConfig.
	Inactive time.Duration
	// Hangtime is the payload hang time, as in FollowerConfig.
	Hangtime time.Duration
	// Scratch is the decode Burst, as in FollowerConfig.
	Scratch *layer2.Burst
	// NoDuplex refuses full-duplex calls to the MS with
	// MS_Duplex_Not_Supported.
	NoDuplex bool
	// Ambient accepts Ambient Listening Service calls to the MS. Without
	// it the MS refuses them with MSNot_Supported.
	Ambient bool
	// Auth, when set, makes the MS authenticate the TSCC before a stun or
	// revive, and lets it accept a kill, which is always authenticated.
	// Without it the MS refuses a kill as not supported. (ETSI TS 102
	// 361-4 clauses 6.4.9.2 and 6.4.10)
	Auth Authenticator
	// NoStun makes the MS refuse a stun or revive as not supported.
	NoStun bool
	// NoKill makes the MS refuse a kill as not supported.
	NoKill bool
	// Stunned starts the MS stunned, for an MS that a TSCC stunned before.
	Stunned bool
	// Killed starts the MS killed, for an MS that a TSCC killed before.
	Killed bool
	// PowerSave is the PowerSave_RQ of the registrations of the MS, 0 to
	// 7: the MS asks to be awake for one power save frame in 2^PowerSave.
	// 0 asks for no power save. (ETSI TS 102 361-4 clause 6.4.7)
	PowerSave uint8
	// Awake is T_Awake, the time the MS stays awake after a PDU for it in
	// power save. 0 means constants.TAwakeDefault.
	Awake time.Duration
}

type raState uint8

const (
	raIdle raState = iota
	raWaitSlot
	raWaitResp
	raWaitAloha
)

type request uint8

const (
	reqNone request = iota
	reqRegister
	reqDereg
	reqCall
)

type sysArea struct {
	model pdu.NetworkModel
	net   uint16
	area  uint16
}

type deniedEntry struct {
	area    sysArea
	until   time.Duration
	forever bool
	used    bool
}

// MS is a Tier III mobile station: it hunts for and follows a TSCC as a
// Follower does, registers and de-registers, makes voice and packet data
// calls by random access, answers C_AHOY and P_AHOY, follows C_MOVE and
// P_PROTECT, and ends its own calls with P_MAINT (ETSI TS 102 361-4
// clauses 6.2 to 6.6). The consumer sends the voice of a granted call with
// a voice.Transmitter with the MS SYNC. Make it with NewMS. An MS is not
// safe for concurrent use and must not be copied; go vet reports a copy.
type MS struct {
	f         Follower
	addr      uint32
	slot      enums.Slot
	rng       pcg.Source
	noReg     bool
	randTO    time.Duration
	callTO    time.Duration
	regTO     time.Duration
	pendingTO time.Duration
	deregTO   time.Duration
	deniedTO  time.Duration
	dmrla     int
	minHunt   int
	minIdle   int
	hasHunt   bool
	hasIdle   bool
	maxBER    int

	off        bool
	registered bool
	regArea    sysArea
	denied     [MaxDenied]deniedEntry
	level      int
	hasLevel   bool
	berErr     [berWindow]int
	berBits    [berWindow]int
	berIdx     int
	berN       int

	ra         raState
	frames     int
	retries    int
	raStart    time.Duration
	req        request
	reqUntil   time.Duration
	waitAnswer bool
	deregEnd   time.Duration
	kind       enums.ServiceKind
	target     uint32
	options    uint8
	emergency  bool
	ownCall    bool
	pendingEnd time.Duration
	pending    bool

	auth    authChallenge
	tx      pdu.CSBK
	txRand  pdu.RandomAccessPDU
	txAck   pdu.AckInboundPDU
	txAckP  pdu.AckInboundPayloadPDU
	txMaint pdu.MaintenancePDU
	u       msUDT
	svc     msSvc
	noCopy  noCopy
}

// NewMS returns an MS for cfg. It returns a *dmrgo.ConfigError for an
// Address out of range and for the errors of NewFollower. It allocates the
// MS, and its Burst unless cfg.Scratch is set.
func NewMS(cfg *MSConfig) (*MS, error) {
	if cfg == nil || cfg.Address == 0 || cfg.Address > maxAddress {
		return nil, errAddress
	}
	if cfg.DMRLA < 0 || cfg.DMRLA > 10 {
		return nil, errDMRLA
	}
	m := &MS{
		addr: cfg.Address, slot: cfg.Slot, noReg: cfg.NoRegistration, randTO: cfg.RandomTimeout, callTO: cfg.CallTimeout,
		regTO: cfg.RegTimeout, pendingTO: cfg.Pending, deregTO: cfg.Dereg, deniedTO: cfg.DeniedLife, dmrla: cfg.DMRLA, maxBER: cfg.MaxBERPermille,
	}
	err := m.f.init(&FollowerConfig{
		SysCode: cfg.SysCode, Hunt: cfg.Hunt, HuntLen: cfg.HuntLen, Plan: cfg.Plan, Talkgroups: cfg.Talkgroups,
		TalkgroupsLen: cfg.TalkgroupsLen, Individual: true, Data: true, NoSignal: cfg.NoSignal, Inactive: cfg.Inactive,
		Hangtime: cfg.Hangtime, AcceptRAS: cfg.AcceptRAS, Scratch: cfg.Scratch,
	})
	if err != nil {
		return nil, err
	}
	m.f.self = cfg.Address
	m.u.noDuplex, m.u.ambient = cfg.NoDuplex, cfg.Ambient
	if err := m.initSvc(cfg); err != nil {
		return nil, err
	}
	if cfg.MinLevelHunt != nil {
		m.minHunt, m.hasHunt = *cfg.MinLevelHunt, true
	}
	if cfg.MinLevelIdle != nil {
		m.minIdle, m.hasIdle = *cfg.MinLevelIdle, true
	}
	if m.slot == enums.SlotUnknown {
		m.slot = enums.Slot1
	}
	m.randTO = orDefault(m.randTO, constants.TRandTCDefault)
	m.callTO = orDefault(m.callTO, constants.TPTimerDefault)
	m.regTO = orDefault(m.regTO, constants.TNPTimerDefault)
	m.deniedTO = orDefault(m.deniedTO, constants.TDenRegDefault)
	if m.pendingTO <= 0 {
		m.pendingTO = constants.TPendingDefault
	}
	if m.deregTO <= 0 {
		m.deregTO = constants.TDeregDefault
	}
	if m.dmrla == 0 {
		m.dmrla = constants.DMRLADefault
	}
	if m.maxBER == 0 {
		m.maxBER = DefaultMaxBERPermille
	}
	m.rng = pcg.New(cfg.Seed)
	return m, nil
}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// State returns the channel state of the MS.
func (m *MS) State() State {
	return m.f.state
}

// Site returns what the MS knows about the TSCC.
func (m *MS) Site() Site {
	return m.f.ctl.site
}

// UDT returns the UDT delivery of the last OutputUDT, as Follower.UDT
// does.
func (m *MS) UDT() *UDT {
	return &m.f.ctl.udt
}

// Registered reports whether the MS holds a registration record for the
// SYS_AREA of the TSCC that it is on.
func (m *MS) Registered() bool {
	return m.registered && m.f.ctl.site.HasSysCode && m.regArea == m.areaOf(m.f.ctl.site.SysCode)
}

// Busy reports whether a registration, de-registration or call request
// runs.
func (m *MS) Busy() bool {
	return m.req != reqNone
}

// Off reports whether the MS completed PowerOff or a TSCC killed it. An MS
// that is off ignores every input until Reset, and a killed MS for good.
func (m *MS) Off() bool {
	return m.off
}

// Reset forgets the site, the registration, the denied list, the DGNA
// talkgroups and any request, as after NewMS. It keeps the config, the
// poll responses, and a stun or kill.
func (m *MS) Reset() {
	m.f.Reset()
	m.off, m.registered, m.ra, m.req, m.pending, m.ownCall, m.waitAnswer = false, false, raIdle, reqNone, false, false, false
	m.auth = authChallenge{}
	m.denied = [MaxDenied]deniedEntry{}
	m.berN, m.berIdx, m.hasLevel = 0, 0, false
	m.u.reset()
	m.f.dyn = [MaxDynamicGroups]uint32{}
	m.resetSvc()
}

// Call asks the TSCC for a voice call to the talkgroup or MS target, by
// random access. The outputs follow from Burst and Tick: OutputGrant and
// OutputTune for the grant, or OutputRefused. An emergency call uses the
// emergency random access of ETSI TS 102 361-4 clause 6.2.1.1.9. It returns
// ErrNotOnControl when the MS is not on a confirmed TSCC, ErrBusy while
// another request runs, ErrNotRegistered when the TSCC asks for a
// registration that the MS does not hold, and a *dmrgo.ConfigError for a
// target or options out of range.
func (m *MS) Call(now time.Duration, group bool, target uint32, opts layer3.VoiceServiceOptions) error {
	kind := enums.ServiceIndividualVoice
	if group {
		kind = enums.ServiceTalkgroupVoice
	}
	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	return m.request(now, kind, target, field, opts.IsEmergency)
}

// CallData asks the TSCC for a packet data call to the talkgroup or MS
// target, as Call does. The consumer sends the data on the granted
// channel with data.Sender.
func (m *MS) CallData(now time.Duration, group bool, target uint32, opts layer3.DataServiceOptions) error {
	kind := enums.ServiceIndividualPacketData
	if group {
		kind = enums.ServiceTalkgroupPacketData
	}
	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	return m.request(now, kind, target, field, opts.IsEmergency)
}

func (m *MS) request(now time.Duration, kind enums.ServiceKind, target uint32, field uint8, emergency bool) error {
	switch {
	case m.off || m.f.state != StateControl:
		return ErrNotOnControl
	case m.req != reqNone:
		return ErrBusy
	case m.svc.stunned:
		return ErrStunned
	case target == 0 || target > 0xFFFFFF:
		return errTarget
	case m.f.ctl.site.Reg && !m.noReg && !m.Registered():
		return ErrNotRegistered
	}
	m.kind = kind
	m.startAccess(now, reqCall, target, field, emergency)
	return nil
}

// PowerOff de-registers the MS before it is switched off: it sends the
// de-registration by random access and appends OutputPowerOff when the
// TSCC accepts it or after T_Dereg (ETSI TS 102 361-4 clause 6.4.6). An MS
// that holds no registration appends OutputPowerOff at once. It allocates
// only when dst has no room for MaxOutputs outputs.
func (m *MS) PowerOff(now time.Duration, dst []Output) []Output {
	dst = m.timers(now, dst)
	if m.off || m.req == reqDereg {
		return dst
	}
	if m.req != reqNone {
		dst = m.fail(ReasonCancelled, 0, dst)
	}
	if m.f.state != StateControl || !m.Registered() {
		m.off = true
		return append(dst, Output{Kind: OutputPowerOff})
	}
	m.registered = false
	opts, _ := layer3.RegistrationServiceOptions{}.Field()
	m.kind = enums.ServiceRegistration
	m.startAccess(now, reqDereg, constants.GatewayREGI, opts, false)
	m.deregEnd = now + m.deregTO
	return dst
}

// Quality gives the MS the signal level of the TSCC, in the units of
// MSConfig.MinLevelHunt and MinLevelIdle, for the confirmation of a TSCC
// and for leaving it (ETSI TS 102 361-4 clauses 6.3.2.3 and 6.3.3.1). It
// appends the outputs to dst, and allocates only when dst has no room for
// MaxOutputs outputs.
func (m *MS) Quality(now time.Duration, level int, dst []Output) []Output {
	dst = m.timers(now, dst)
	m.level, m.hasLevel = level, true
	if m.f.state == StateControl && m.hasIdle && level < m.minIdle && m.req == reqNone {
		return m.leaveTSCC(now, ReasonQuality, dst)
	}
	return dst
}

// Cancel stops the running request. It appends OutputRefused with
// ReasonCancelled when one ran, and allocates only when dst has no room
// for MaxOutputs outputs.
func (m *MS) Cancel(now time.Duration, dst []Output) []Output {
	dst = m.timers(now, dst)
	if m.req == reqNone || m.req == reqDereg {
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
	to := g.Target
	if to == m.addr {
		to = g.Source
	}
	m.txMaint = pdu.MaintenancePDU{TargetAddress: to, SourceAddress: m.addr}
	m.tx = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKMaintenance, MaintenancePDU: &m.txMaint}
	for i := range constants.NMaint {
		dst = m.send(now+time.Duration(i)*frame, g.Slot, dst)
	}
	m.ownCall = false
	return m.f.leave(now, ReasonMaintenance, dst)
}

// Burst decodes the burst raw with what the Hint h says about it, and the
// CACH field before it, in transmit order, when cach is not nil, as
// Follower.Burst does, runs the
// random access and the requests, and appends the outputs to dst. On the
// TSCC the MS counts TDMA frames by the bursts of its Slot, so h.Slot
// should be set; random access needs the CACH, for its AT bit. A nil raw
// is a transport frame without a burst: with Hint.SyncLost it does what
// Lost does, else it runs the timers. It allocates only when dst has no
// room for MaxOutputs outputs.
func (m *MS) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *[3]byte, dst []Output) []Output {
	if m.off {
		return dst
	}
	if raw == nil {
		if h.SyncLost {
			return m.Lost(now, dst)
		}
		return m.timers(now, dst)
	}
	var c *layer2.CACH
	if cach != nil {
		m.f.cach = layer2.DecodeCACHField(*cach)
		c = &m.f.cach
	}
	dst = m.timers(now, dst)
	before := m.f.state
	n := len(dst)
	dst = m.f.burst(now, raw, h, c, dst)
	dst = m.follow(now, before, dst, n)
	dst = m.svcBurst(now, before, &h, dst, n)
	switch {
	case m.f.state == StatePayload && before == StatePayload:
		return m.payload(now, &h, dst)
	case m.f.state != StateControl || before != StateControl:
		return dst
	}
	if m.f.ctl.b.Kind == layer2.KindData && m.f.ctl.okData() && m.berBurst() {
		return m.leaveTSCC(now, ReasonQuality, dst)
	}
	dst = m.addressed(now, &h, dst)
	if m.f.state == StateControl && (h.Slot == enums.SlotUnknown || h.Slot == m.slot) {
		dst = m.frame(now, c, dst)
	}
	return dst
}

// Lost tells the MS that the receiver lost the signal, as Follower.Lost
// does. It allocates only when dst has no room for MaxOutputs outputs.
func (m *MS) Lost(now time.Duration, dst []Output) []Output {
	if m.off {
		return dst
	}
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
	if m.off {
		return dst
	}
	return m.timers(now, dst)
}

// Deadline returns the time at which the MS needs a Tick. ok is false
// when no timer runs.
func (m *MS) Deadline() (at time.Duration, ok bool) {
	if m.off {
		return 0, false
	}
	if m.svc.asleep {
		return m.sleepDeadline()
	}
	at, ok = m.f.Deadline()
	if m.ra == raWaitSlot && m.randTO > 0 {
		at, ok = earliest(at, ok, m.raStart+m.randTO)
	}
	if m.waitAnswer && m.answerTO() > 0 {
		at, ok = earliest(at, ok, m.reqUntil)
	}
	if m.req == reqDereg {
		at, ok = earliest(at, ok, m.deregEnd)
	}
	if m.pending {
		at, ok = earliest(at, ok, m.pendingEnd)
	}
	at, ok = m.u.deadline(at, ok)
	return m.svcDeadline(at, ok)
}

func earliest(at time.Duration, ok bool, t time.Duration) (time.Duration, bool) {
	if !ok || t < at {
		return t, true
	}
	return at, ok
}

func (m *MS) answerTO() time.Duration {
	if m.req == reqCall {
		return m.callTO
	}
	return m.regTO
}

func (m *MS) timers(now time.Duration, out []Output) []Output {
	out = m.svcTimers(now, out)
	n := len(out)
	before := m.f.state
	out = m.f.timers(now, out)
	out = m.follow(now, before, out, n)
	if m.req == reqDereg && now >= m.deregEnd {
		m.done()
		m.off = true
		return append(out, Output{Kind: OutputPowerOff, Reason: ReasonTimeout})
	}
	if m.ra == raWaitSlot && m.randTO > 0 && now >= m.raStart+m.randTO && m.req == reqDereg {
		m.ra = raIdle
	}
	if m.ra == raWaitSlot && m.randTO > 0 && now >= m.raStart+m.randTO {
		sent := m.retries > 0
		out = m.fail(ReasonNoAnswer, 0, out)
		if sent && m.f.state == StateControl {
			out = m.leaveTSCC(now, ReasonNoAnswer, out)
		}
	}
	if m.waitAnswer && m.answerTO() > 0 && now >= m.reqUntil {
		reg := m.req == reqRegister
		out = m.fail(ReasonTimeout, 0, out)
		if reg && m.f.state == StateControl {
			out = m.leaveTSCC(now, ReasonTimeout, out)
		}
	}
	if m.pending && now >= m.pendingEnd {
		m.pending = false
	}
	out = m.udtTimers(now, out)
	return m.svcLate(now, out)
}

func (m *MS) follow(now time.Duration, before State, out []Output, n int) []Output {
	for i := n; i < len(out); i++ {
		switch o := &out[i]; o.Kind {
		case OutputSite:
			if before == StateHunting {
				m.berN, m.berIdx = 0, 0
				if m.isDenied(now) {
					return m.leaveTSCC(now, ReasonDenied, out)
				}
				if m.hasHunt && m.hasLevel && m.level < m.minHunt {
					return m.leaveTSCC(now, ReasonQuality, out)
				}
			}
			if m.req == reqNone && !m.noReg && m.f.ctl.site.Reg && !m.Registered() {
				m.kind = enums.ServiceRegistration
				m.startAccess(now, reqRegister, constants.GatewayREGI, m.regOptions(), false)
			}
		case OutputLost:
			if m.aborts() {
				out = m.fail(ReasonNoSignal, 0, out)
			}
		case OutputTune:
			if before == StateControl && m.f.state == StatePayload {
				g := &m.f.call
				m.ownCall = g.Source == m.addr
				if m.req == reqCall && (m.ownCall || g.Target == m.addr) {
					m.done()
				}
				m.pending = false
				out = m.joined(now, out)
			}
		case OutputUDT:
			out = m.udtIn(now, out)
		case OutputGrant, OutputCallEnd, OutputBurst, OutputRegistered, OutputRefused, OutputAhoy, OutputProtect, OutputPowerOff,
			OutputAuthChallenge, OutputDelivered, OutputPolled, OutputGroups, OutputAmbient:
		}
	}
	return out
}

func (m *MS) aborts() bool {
	return m.req != reqNone && m.req != reqDereg && (m.req != reqCall || !m.waitAnswer)
}

func (m *MS) leaveTSCC(now time.Duration, reason Reason, out []Output) []Output {
	if m.aborts() {
		out = m.fail(reason, 0, out)
	}
	m.ra = raIdle
	return m.f.lose(now, reason, out)
}

func (m *MS) regOptions() uint8 {
	v, _ := layer3.RegistrationServiceOptions{Register: true, PowerSave: m.svc.psRQ}.Field()
	return v
}

func (m *MS) areaOf(sc pdu.SysCode) sysArea {
	bits, err := sc.Model.SiteBits()
	if err != nil {
		return sysArea{}
	}
	a := sc.Site
	if m.dmrla < bits {
		a >>= bits - m.dmrla
	}
	return sysArea{model: sc.Model, net: sc.Net, area: a}
}

func (m *MS) isDenied(now time.Duration) bool {
	area := m.areaOf(m.f.ctl.site.SysCode)
	for i := range m.denied {
		d := &m.denied[i]
		if !d.used {
			continue
		}
		if !d.forever && now >= d.until {
			d.used = false
			continue
		}
		if d.area == area {
			return true
		}
	}
	return false
}

func (m *MS) deny(now time.Duration) {
	e := deniedEntry{area: m.areaOf(m.f.ctl.site.SysCode), until: now + m.deniedTO, forever: m.deniedTO < 0, used: true}
	oldest := 0
	for i := range m.denied {
		if !m.denied[i].used {
			m.denied[i] = e
			return
		}
		if !m.denied[i].forever && m.denied[i].until < m.denied[oldest].until {
			oldest = i
		}
	}
	m.denied[oldest] = e
}

func (m *MS) berBurst() bool {
	if m.maxBER < 0 {
		return false
	}
	r := m.f.ctl.b.FEC.Aggregate()
	errs := r.ErrorsCorrected
	if r.Uncorrectable {
		errs = r.BitsChecked
	}
	m.berErr[m.berIdx], m.berBits[m.berIdx] = errs, r.BitsChecked
	m.berIdx = (m.berIdx + 1) % berWindow
	if m.berN < berWindow {
		m.berN++
		return false
	}
	var e, b int
	for i := range berWindow {
		e += m.berErr[i]
		b += m.berBits[i]
	}
	return b > 0 && e*1000 > b*m.maxBER
}
