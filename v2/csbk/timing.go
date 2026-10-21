package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Channel timing constants of TDMA direct mode.
const (
	// AllSiteColorCode is the "All Site" color code of the CT_CSBKs (ETSI
	// TS 102 361-2 clause 6.2.2.2).
	AllSiteColorCode enums.ColorCode = 15
	// MaxTimingID is the largest 20-bit MS identifier of a wide area timing
	// identifier (ETSI TS 102 361-2 clause 7.2.10).
	MaxTimingID = 1<<20 - 1
	// MaxDynamicID is the largest dynamic identifier, the leader preference
	// High (ETSI TS 102 361-2 clause 7.2.9).
	MaxDynamicID = 3
)

const (
	ctoUnalignedRequest = 0b00
	ctoUnalignedTerm    = 0b01
	ctoAlignedStatus    = 0b10
	ctoAlignedPush      = 0b11

	saMax         = 1<<11 - 1
	saTolerance   = 1
	saConflict    = 10
	ctGiveUp      = 2 * time.Minute
	rhotKnownLow  = 2160 * time.Millisecond
	rhotWindowDec = 120 * time.Millisecond
	beaconBursts  = int(constants.BeaconDuration / BurstInterval)
	ctBursts      = int(constants.CTDuration / BurstInterval)
)

// TimingConfig configures NewTiming. (ETSI TS 102 361-2 clause 6.2)
type TimingConfig struct {
	// ID is MS_ID, the 20-bit identifier of the radio in its wide area
	// timing identifier, from 1 to MaxTimingID. The Timing picks a new one
	// when another radio uses it (ETSI TS 102 361-2 clause 6.2.3.7).
	ID uint32
	// DynamicID is MS_DI, the leader preference of the radio from 0 to
	// MaxDynamicID. 0 means the radio is not eligible to be the leader.
	// (ETSI TS 102 361-2 clause 7.2.9)
	DynamicID uint8
	// Slot is the provisioned time slot, enums.Slot1 or enums.Slot2. It
	// selects the TDMA direct mode SYNC of the CT_CSBKs.
	// (ETSI TS 102 361-2 clause 6.2)
	Slot enums.Slot
	// Seed seeds the generator of CT_RHOT and of new identifiers, so that
	// the same inputs give the same outputs. Radios on one channel need
	// different seeds. (ETSI TS 102 361-2 clause 6.2)
	Seed uint64
	// Scratch, when set, is the Burst that Burst decodes into, as for
	// RequesterConfig.Scratch. (ETSI TS 102 361-2 clause 6.2)
	Scratch *layer2.Burst
}

// TimingState is the high level state of a Timing (ETSI TS 102 361-2
// clause 6.2.1).
type TimingState uint8

// The TimingState values.
const (
	// TimingLeaderAndTimingUnknown knows neither the channel slot timing
	// nor the leader: after Join, and when no leader was ever known.
	// (ETSI TS 102 361-2 clause 6.2)
	TimingLeaderAndTimingUnknown TimingState = iota
	// TimingLeaderUnknown knew the timing, but SyncAge ran out, and knows
	// no leader. (ETSI TS 102 361-2 clause 6.2)
	TimingLeaderUnknown
	// TimingLeaderAndTimingKnown follows the leader and its slot timing.
	// (ETSI TS 102 361-2 clause 6.2)
	TimingLeaderAndTimingKnown
	// TimingLeader is the wide area timing leader and sets the slot timing.
	// (ETSI TS 102 361-2 clause 6.2)
	TimingLeader
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s TimingState) String() string {
	switch s {
	case TimingLeaderAndTimingUnknown:
		return "LeaderAndTimingUnknown"
	case TimingLeaderUnknown:
		return "LeaderUnknown"
	case TimingLeaderAndTimingKnown:
		return "LeaderAndTimingKnown"
	case TimingLeader:
		return "Leader"
	}
	return "TimingState(" + text.Int(int64(s)) + ")"
}

type ctKind uint8

const (
	ctNone ctKind = iota
	ctRequest
	ctResponse
	ctAppoint
	ctPropagate
	ctBeacon
)

type timer struct {
	at time.Duration
	on bool
}

func (t *timer) set(at time.Duration) {
	t.at, t.on = at, true
}

func (t *timer) due(now time.Duration) bool {
	if !t.on || now < t.at {
		return false
	}
	t.on = false
	return true
}

// Timing runs the TDMA direct mode wide area timing of one radio: it
// learns the leader and the channel slot timing from the CT_CSBKs of other
// radios, elects and appoints a leader, propagates the leader beacon,
// answers requests and corrects radios with old timing, and beacons when
// it is the leader. It sends each CT_CSBK after the random holdoff CT_RHOT,
// polite to all, in the provisioned slot with the All Site color code.
// OutputTiming tells the caller to take the channel slot timing from the
// CT_CSBK that it passed. Make it with NewTiming and call Join when the
// radio arrives on the channel. A Timing is not safe for concurrent use and
// must not be copied; go vet reports a copy. (ETSI TS 102 361-2 clause 6.2)
type Timing struct {
	src    layer2.SyncSource
	di     uint8
	id     uint32
	rng    pcg.Source
	d      decoder
	busy   bool
	joined bool

	state   TimingState
	lwatid  uint32
	gen     uint8
	syncRef time.Duration

	noLeader timer
	syncAge  timer
	warning  timer
	beacon   timer

	pend      ctKind
	pendAt    time.Duration
	pendStart time.Duration
	rhotLow   time.Duration
	rhotHigh  time.Duration

	sending   int
	sendDue   time.Duration
	sendBurst layer2.RawBurst

	emitted bool
	build   pdu.CSBK
	ct      pdu.ChannelTimingPDU
	noCopy  noCopy
}

// NewTiming returns a Timing for cfg. It returns a *ConfigError for a nil
// cfg, an ID of 0 or above MaxTimingID, a DynamicID above MaxDynamicID or a
// Slot other than Slot1 or Slot2. It allocates the Timing, and its Burst
// unless cfg.Scratch is set. The Timing does nothing before Join.
// (ETSI TS 102 361-2 clause 6.2)
func NewTiming(cfg *TimingConfig) (*Timing, error) {
	switch {
	case cfg == nil, cfg.ID == 0, cfg.ID > MaxTimingID:
		return nil, errID
	case cfg.DynamicID > MaxDynamicID:
		return nil, errDynamicID
	}
	m := &Timing{di: cfg.DynamicID, id: cfg.ID, rng: seedPCG(cfg.Seed), d: newDecoder(cfg.Scratch)}
	switch cfg.Slot {
	case enums.Slot1:
		m.src = layer2.SyncSourceDirectTS1
	case enums.Slot2:
		m.src = layer2.SyncSourceDirectTS2
	default:
		return nil, errSlot
	}
	return m, nil
}

// Join starts the Timing for a radio that arrives on the channel: at power
// up and on a change of frequency, but not on a change to the other slot of
// the same frequency. The radio knows no leader and no timing, and waits
// NoLeader for the CT_CSBKs of other radios before it asks for them.
// (ETSI TS 102 361-2 clause 6.2.3.1)
func (m *Timing) Join(now time.Duration) {
	m.Reset()
	m.joined = true
	m.noLeader.set(now + constants.NoLeader)
}

// Reset stops the Timing until the next Join and forgets the channel
// state. It keeps the config, the identifier and the state of the random
// generator. (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Reset() {
	m.joined, m.busy, m.state, m.lwatid, m.gen, m.syncRef = false, false, TimingLeaderAndTimingUnknown, 0, 0, 0
	m.noLeader, m.syncAge, m.warning, m.beacon = timer{}, timer{}, timer{}, timer{}
	m.pend, m.sending, m.rhotLow, m.rhotHigh = ctNone, 0, 0, constants.CTRHOTMax
}

// State returns the state of the Timing. (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) State() TimingState {
	return m.state
}

// Leader returns the dynamic identifier and the identifier of the wide
// area timing leader. ok is false when the radio knows no leader.
// (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Leader() (dynamicID uint8, id uint32, ok bool) {
	return uint8(m.lwatid >> 20), m.lwatid & MaxTimingID, m.lwatid != 0
}

// Channel reports whether the frequency has RF activity on either slot. A
// CT_CSBK waits for an idle channel. Until the first report the channel
// counts as idle. (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Channel(_ time.Duration, busy bool) {
	m.busy = busy
}

// Terminator appends to dst the CT_CSBK_Term that follows a voice, data
// or CSBK transmission of the radio, due at now. Its channel timing opcode
// is unaligned request without a leader, and with a leader unaligned
// terminator while Channel reports activity, else aligned status. A
// response data header and a CSBK acknowledgement have no CT_CSBK_Term.
// It allocates only when dst has no room for MaxOutputs outputs. (ETSI TS
// 102 361-2 clauses 6.2.2.3.3 and 6.2.3.12)
func (m *Timing) Terminator(now time.Duration, dst []Output) []Output {
	m.ct = pdu.ChannelTimingPDU{Generation: m.gen, SyncAge: m.sa(now), SourceIdentifier: m.id}
	m.setLeader(m.lwatid)
	switch {
	case m.state == TimingLeader:
		m.ct.Generation, m.ct.SourceDynamicIdentifier = 0, m.di
	case m.state == TimingLeaderUnknown:
		m.ct.SourceDynamicIdentifier = m.di
	}
	switch {
	case m.lwatid == 0:
		m.setCTO(ctoUnalignedRequest)
	case m.busy:
		m.setCTO(ctoUnalignedTerm)
	default:
		m.setCTO(ctoAlignedStatus)
	}
	b, ok := m.encode()
	if !ok {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst)
	o.Service, o.Burst, o.Due = ServiceTiming, b, now
	return dst
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Timing as Decoded does, and appends the outputs to dst. It
// allocates only when dst has no room for MaxOutputs outputs.
// (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	return m.Decoded(now, m.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil, from either slot,
// and evaluates a CT_CSBK in it: it appends OutputTiming to dst when the
// radio takes the slot timing of b, and starts, keeps or cancels the
// CT_CSBK that the radio sends. A CSBK whose CRC fails is dropped. It
// allocates only when dst has no room for MaxOutputs outputs. (ETSI TS 102
// 361-2 clauses 6.2.3.2 to 6.2.3.11)
func (m *Timing) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	dst = m.Tick(now, dst)
	if !m.joined {
		return dst
	}
	kind, c, octets := classify(b)
	if kind != rxCSBK || c.FID != enums.FIDStandardized || c.ChannelTimingPDU == nil {
		return dst
	}
	m.emitted = false
	rx := c.ChannelTimingPDU
	if m.pend != ctNone {
		var eval bool
		if dst, eval = m.pending(now, rx, octets, dst); !eval {
			return dst
		}
	}
	if m.state == TimingLeader {
		return m.leader(now, rx, octets, dst)
	}
	return m.evaluate(now, rx, octets, dst)
}

func (m *Timing) pending(now time.Duration, rx *pdu.ChannelTimingPDU, octets pdu.CSBKBytes, dst []Output) ([]Output, bool) {
	lw, sw, cto := lwatidOf(rx), swatidOf(rx), ctoOf(rx)
	keep := false
	switch m.pend {
	case ctRequest:
		switch m.state {
		case TimingLeaderAndTimingUnknown, TimingLeaderUnknown:
			if keep = lw != 0 && rx.LeaderDynamicIdentifier < m.di; keep {
				dst = m.take(now, octets, dst)
			}
		case TimingLeaderAndTimingKnown, TimingLeader:
			keep = cto < ctoAlignedStatus || lw < m.lwatid || lw == m.lwatid && m.older(now, rx.SyncAge)
		}
	case ctResponse:
		keep = cto < ctoAlignedStatus || lw < m.lwatid
	case ctAppoint:
		keep = lw <= m.lwatid && sw <= m.lwatid
	case ctPropagate:
		if lw == m.lwatid && cto == ctoAlignedPush && m.sameSA(now, rx.SyncAge) {
			m.cancel()
			return dst, false
		}
		keep = lw <= m.lwatid && sw <= m.lwatid
	case ctNone, ctBeacon:
		return dst, true
	}
	if !keep {
		m.cancel()
	}
	return dst, !keep
}

func (m *Timing) evaluate(now time.Duration, rx *pdu.ChannelTimingPDU, octets pdu.CSBKBytes, dst []Output) []Output {
	lw, sw, cto := lwatidOf(rx), swatidOf(rx), ctoOf(rx)
	if lw == m.watid() && rx.NewLeader {
		dst = m.take(now, octets, dst)
		m.gen, m.lwatid = 0, m.watid()
		m.setSA(now, rx.SyncAge)
		m.enter(TimingLeader)
		m.beacon.set(now + constants.BeaconInterval)
		m.schedule(now, ctBeacon)
		m.pendAt = now
		return dst
	}
	if lw == m.watid() || rx.SourceIdentifier == m.id {
		m.newID()
	}
	if m.lwatid == 0 {
		dst = m.take(now, octets, dst)
		switch {
		case lw == 0 && rx.SourceDynamicIdentifier > m.di:
			m.appoint(now, sw)
		case lw == 0:
			if m.pend == ctNone {
				m.schedule(now, ctRequest)
			}
		default:
			m.accept(now, rx)
			if cto == ctoAlignedPush {
				m.schedule(now, ctPropagate)
			}
		}
		return dst
	}
	if cto == ctoUnalignedTerm {
		if lw < m.lwatid {
			m.schedule(now, ctResponse)
		}
		return dst
	}
	switch {
	case lw < m.lwatid || lw == m.lwatid && m.older(now, rx.SyncAge):
		if rx.SourceDynamicIdentifier <= uint8(m.lwatid>>20) {
			m.schedule(now, ctResponse)
		} else {
			m.appoint(now, sw)
		}
	case lw > m.lwatid && cto == ctoAlignedPush:
		dst = m.take(now, octets, dst)
		m.accept(now, rx)
		m.schedule(now, ctPropagate)
	case lw > m.lwatid && cto == ctoAlignedStatus:
		dst = m.take(now, octets, dst)
		m.accept(now, rx)
	case lw != m.lwatid:
	case m.sameSA(now, rx.SyncAge) && rx.Generation < m.gen:
		dst = m.take(now, octets, dst)
		m.gen = min(rx.Generation+1, 31)
		if cto == ctoAlignedPush {
			m.schedule(now, ctPropagate)
		}
	case !m.sameSA(now, rx.SyncAge) && cto >= ctoAlignedStatus:
		dst = m.take(now, octets, dst)
		m.accept(now, rx)
		if cto == ctoAlignedPush {
			m.schedule(now, ctPropagate)
		}
	}
	return dst
}

func (m *Timing) leader(now time.Duration, rx *pdu.ChannelTimingPDU, octets pdu.CSBKBytes, dst []Output) []Output {
	lw, sw := lwatidOf(rx), swatidOf(rx)
	self := m.watid()
	switch {
	case lw == self:
		sa, msa := int(rx.SyncAge), int(m.sa(now))
		switch {
		case abs(sa-msa) <= saConflict:
		case abs(sa%int(constants.BeaconInterval/constants.SyncAgeIncrement)-msa) <= saConflict:
			m.schedule(now, ctResponse)
		default:
			m.newID()
			if m.watid() < lw && ctoOf(rx) >= ctoAlignedStatus {
				dst = m.take(now, octets, dst)
				m.accept(now, rx)
			} else {
				m.lwatid = m.watid()
			}
		}
	case lw != 0 && lw < self, lw == 0 && rx.SourceDynamicIdentifier <= m.di:
		m.schedule(now, ctResponse)
	case lw == 0, rx.SourceDynamicIdentifier > rx.LeaderDynamicIdentifier:
		m.appoint(now, sw)
	case ctoOf(rx) >= ctoAlignedStatus:
		dst = m.take(now, octets, dst)
		m.accept(now, rx)
	}
	return dst
}

func (m *Timing) take(now time.Duration, octets pdu.CSBKBytes, dst []Output) []Output {
	if m.emitted {
		return dst
	}
	m.emitted = true
	var o *Output
	dst, o = appendOutput(dst, OutputTiming)
	o.Service, o.CSBK, o.Due = ServiceTiming, octets, now
	return dst
}

func (m *Timing) accept(now time.Duration, rx *pdu.ChannelTimingPDU) {
	if ctoOf(rx) < ctoAlignedStatus {
		return
	}
	m.gen, m.lwatid = min(rx.Generation+1, 31), lwatidOf(rx)
	m.setSA(now, rx.SyncAge)
	m.enter(TimingLeaderAndTimingKnown)
}

func (m *Timing) appoint(now time.Duration, sw uint32) {
	if m.state < TimingLeaderAndTimingKnown {
		m.syncRef = now
	}
	m.gen, m.lwatid = 1, sw
	m.enter(TimingLeaderAndTimingKnown)
	m.schedule(now, ctAppoint)
}

func (m *Timing) enter(s TimingState) {
	if s != TimingLeader && m.state == TimingLeader {
		m.beacon.on = false
		if m.pend == ctBeacon {
			m.pend = ctNone
		}
	}
	m.state = s
	m.noLeader.on = false
	if s == TimingLeaderAndTimingKnown {
		m.syncAge.set(m.syncRef + constants.SyncAge)
		m.warning.set(m.syncRef + constants.SyncAgeWarning)
	} else {
		m.syncAge.on, m.warning.on = false, false
	}
}

func (m *Timing) schedule(now time.Duration, k ctKind) {
	m.pend, m.pendStart = k, now
	m.pendAt = now + m.rhot()
}

func (m *Timing) cancel() {
	m.pend = ctNone
	if m.rhotLow == 0 {
		return
	}
	m.rhotLow, m.rhotHigh = m.rhotLow-rhotWindowDec, m.rhotHigh-rhotWindowDec
	if m.rhotLow <= 0 {
		m.rhotLow, m.rhotHigh = rhotKnownLow, constants.CTRHOTMax
	}
}

func (m *Timing) rhot() time.Duration {
	lo, hi := time.Duration(0), constants.CTRHOTMax
	if m.state >= TimingLeaderAndTimingKnown {
		lo, hi = m.rhotLow, m.rhotHigh
	}
	steps := uint32((hi - lo) / constants.CTRHOTIncrement)
	return lo + time.Duration(m.rng.Uint32()%(steps+1))*constants.CTRHOTIncrement
}

func (m *Timing) newID() {
	for {
		id := m.rng.Uint32() & MaxTimingID
		if id != 0 && id != m.id {
			m.id = id
			return
		}
	}
}

func (m *Timing) watid() uint32 {
	return uint32(m.di)<<20 | m.id
}

func (m *Timing) sa(now time.Duration) uint16 {
	if m.state < TimingLeaderAndTimingKnown || now < m.syncRef {
		return 0
	}
	return uint16(min((now-m.syncRef)/constants.SyncAgeIncrement, saMax))
}

func (m *Timing) setSA(now time.Duration, sa uint16) {
	m.syncRef = now - time.Duration(sa)*constants.SyncAgeIncrement
}

func (m *Timing) sameSA(now time.Duration, sa uint16) bool {
	return abs(int(sa)-int(m.sa(now))) <= saTolerance
}

func (m *Timing) older(now time.Duration, sa uint16) bool {
	return int(sa) > int(m.sa(now))+saTolerance
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func lwatidOf(c *pdu.ChannelTimingPDU) uint32 {
	return uint32(c.LeaderDynamicIdentifier)<<20 | c.LeaderIdentifier
}

func swatidOf(c *pdu.ChannelTimingPDU) uint32 {
	return uint32(c.SourceDynamicIdentifier)<<20 | c.SourceIdentifier
}

func ctoOf(c *pdu.ChannelTimingPDU) uint8 {
	var cto uint8
	if c.ChannelTimingOp1 {
		cto = 0b10
	}
	if c.ChannelTimingOp0 {
		cto |= 0b01
	}
	return cto
}

func (m *Timing) setCTO(cto uint8) {
	m.ct.ChannelTimingOp1, m.ct.ChannelTimingOp0 = cto&0b10 != 0, cto&0b01 != 0
}

func (m *Timing) setLeader(lw uint32) {
	m.ct.LeaderDynamicIdentifier, m.ct.LeaderIdentifier = uint8(lw>>20&0x3), lw&MaxTimingID
}

func (m *Timing) encode() (layer2.RawBurst, bool) {
	m.build = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKChannelTiming, ChannelTimingPDU: &m.ct}
	octets, err := m.build.Encode()
	if err != nil {
		return layer2.RawBurst{}, false
	}
	b, err := layer2.BuildDataBurst(octets, enums.DataTypeCSBK, AllSiteColorCode, m.src)
	return b, err == nil
}

func (m *Timing) fill(now time.Duration) int {
	m.ct = pdu.ChannelTimingPDU{Generation: m.gen, SyncAge: m.sa(now), SourceIdentifier: m.id}
	m.setLeader(m.lwatid)
	n := ctBursts
	switch m.pend {
	case ctRequest:
		if m.state < TimingLeaderAndTimingKnown {
			m.ct.Generation, m.ct.SourceDynamicIdentifier = 0, m.di
			m.setLeader(0)
			m.setCTO(ctoUnalignedRequest)
		} else {
			m.setCTO(ctoAlignedStatus)
		}
	case ctResponse:
		if m.state == TimingLeader {
			m.ct.Generation, m.ct.SourceDynamicIdentifier = 0, m.di
		}
		m.setCTO(ctoAlignedStatus)
	case ctAppoint:
		m.ct.NewLeader = true
		m.setCTO(ctoAlignedStatus)
	case ctPropagate:
		m.setCTO(ctoAlignedPush)
		n = beaconBursts
	case ctBeacon:
		m.syncRef = now
		m.ct.Generation, m.ct.SyncAge, m.ct.SourceDynamicIdentifier = 0, 0, m.di
		m.setLeader(m.watid())
		m.setCTO(ctoAlignedPush)
		n = beaconBursts
	case ctNone:
		return 0
	}
	return n
}

// Tick runs the timers that are due at now and appends their outputs to
// dst: at most one CT_CSBK burst. A CT_CSBK_Beacon and a CT_CSBK_Prop take
// BeaconDuration, ten bursts, and the others CTDuration, three bursts.
// A CT_CSBK whose CT_RHOT ends on a busy channel waits another CT_RHOT, for
// up to two minutes. A Tick before Deadline does nothing. It allocates only
// when dst has no room for MaxOutputs outputs. (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Tick(now time.Duration, dst []Output) []Output {
	if !m.joined {
		return dst
	}
	m.timers(now)
	if m.pend != ctNone && m.sending == 0 && now >= m.pendAt {
		switch {
		case !m.busy:
			m.sending, m.sendDue = m.fill(m.pendAt), m.pendAt
			m.sendBurst, _ = m.encode()
			if m.sending > 0 && m.state >= TimingLeaderAndTimingKnown {
				m.rhotLow, m.rhotHigh = rhotKnownLow, constants.CTRHOTMax
			}
			if m.pend == ctRequest && m.state < TimingLeaderAndTimingKnown {
				m.noLeader.set(m.pendAt + constants.NoLeader)
			}
			m.pend = ctNone
		case now-m.pendStart >= ctGiveUp:
			m.pend = ctNone
		default:
			m.pendAt = now + m.rhot()
		}
	}
	if m.sending == 0 || now < m.sendDue {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst)
	o.Service, o.Burst, o.Due = ServiceTiming, m.sendBurst, m.sendDue
	m.sending--
	m.sendDue += BurstInterval
	return dst
}

func (m *Timing) timers(now time.Duration) {
	if m.noLeader.due(now) && m.pend == ctNone {
		m.schedule(now, ctRequest)
	}
	if m.syncAge.due(now) {
		m.gen, m.lwatid, m.pend = 0, 0, ctNone
		m.enter(TimingLeaderUnknown)
		m.noLeader.set(now + constants.NoLeader)
	}
	if m.warning.due(now) && m.pend == ctNone {
		m.schedule(now, ctRequest)
	}
	if m.beacon.due(now) {
		m.beacon.set(now + constants.BeaconInterval)
		m.schedule(now, ctBeacon)
	}
}

// Deadline returns the time at which the Timing needs a Tick. ok is false
// when no timer runs. (ETSI TS 102 361-2 clause 6.2)
func (m *Timing) Deadline() (at time.Duration, ok bool) {
	if !m.joined {
		return 0, false
	}
	var d deadline
	for _, t := range [...]*timer{&m.noLeader, &m.syncAge, &m.warning, &m.beacon} {
		if t.on {
			d.add(t.at)
		}
	}
	switch {
	case m.sending > 0:
		d.add(m.sendDue)
	case m.pend != ctNone:
		d.add(m.pendAt)
	}
	return d.at, d.ok
}

type deadline struct {
	at time.Duration
	ok bool
}

func (d *deadline) add(t time.Duration) {
	if !d.ok || t < d.at {
		d.at, d.ok = t, true
	}
}
