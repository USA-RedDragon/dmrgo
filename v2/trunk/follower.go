package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxTalkgroups is the size of FollowerConfig.Talkgroups.
const MaxTalkgroups = 32

const clearHold = time.Second

// FollowerConfig configures NewFollower. A nil config means the zero
// FollowerConfig.
type FollowerConfig struct {
	// SysCode, when set, is the system to follow: a TSCC with another
	// MODEL, NET or SITE is skipped. Nil follows the first TSCC found.
	SysCode *pdu.SysCode
	// Hunt holds the first HuntLen channel numbers of the hunt list. With
	// a hunt list the Follower asks for each channel in turn with
	// OutputTune until it finds a TSCC, and an Ann_WD_TSCC broadcast adds
	// and withdraws channels. With HuntLen 0 the consumer keeps the
	// receiver on one TSCC, which is then channel CHNULL.
	Hunt [MaxChannels]uint16
	// HuntLen is the number of entries of Hunt in use.
	HuntLen int
	// Plan, when set, is the channel plan to start from. The Follower
	// copies it and adds the channels that CG_AP and Chan_Freq announce.
	Plan *ChannelPlan
	// Talkgroups holds the first TalkgroupsLen talkgroups to follow.
	// TalkgroupsLen 0 follows every talkgroup.
	Talkgroups [MaxTalkgroups]uint32
	// TalkgroupsLen is the number of entries of Talkgroups in use.
	TalkgroupsLen int
	// Individual follows individual voice calls too.
	Individual bool
	// Data follows packet data grants too.
	Data bool
	// NoFollow reports grants with OutputGrant but never leaves the TSCC,
	// for a receiver that cannot tune, such as the replay of a recording.
	NoFollow bool
	// NoSignal is T_Nosig, the time without a TSCC burst after which the
	// Follower hunts again, and the time it listens on each hunt list
	// channel. 0 means constants.TNoSignalDefault, and a negative value
	// turns the timer off (ETSI TS 102 361-4 Table A.1).
	NoSignal time.Duration
	// Inactive is TV_Inactive, the time without a burst on the payload
	// channel after which the Follower returns to the TSCC. 0 means
	// constants.TVInactiveDefault, and a negative value turns the timer
	// off.
	Inactive time.Duration
	// Hangtime is the time after a Terminator with LC in which the
	// Follower waits on the payload channel for more voice. 0 means
	// constants.TVHangtimeDefault, and a negative value turns the timer
	// off.
	Hangtime time.Duration
	// SysErrors is NSYSerr, the number of C_SYScodes from another system
	// after which the Follower leaves the TSCC. 0 means
	// constants.NSysErrDefault.
	SysErrors int
	// AcceptRAS takes a CSBK whose CRC fails as valid when its BPTC needed
	// no correction and its BPTC reserved bit R(2) flags RAS. The CRC of a
	// Motorola Capacity Max system with RAS is a MAC with the key of the
	// system, which the Follower cannot check.
	AcceptRAS bool
	// Scratch, when set, is the Burst that the Follower decodes into, as
	// for voice.ReceiverConfig.Scratch. Nil means the Follower makes its
	// own.
	Scratch *layer2.Burst
}

// Follower follows a Tier III trunked site without transmitting, as a
// scanner does: it hunts for a TSCC, reads its site parameters,
// broadcasts and grants, follows the grants of the calls it wants to their
// payload channels, and returns to the TSCC on P_CLEAR, after the hang time
// or when the payload channel goes quiet (ETSI TS 102 361-4 clauses 6.3
// and 6.6). Make it with NewFollower. A Follower is not safe for
// concurrent use and must not be copied; go vet reports a copy.
type Follower struct {
	ctl       control
	cach      layer2.CACH
	talk      [MaxTalkgroups]uint32
	talkLen   int
	dyn       [MaxDynamicGroups]uint32
	indiv     bool
	data      bool
	noFollow  bool
	noSignal  time.Duration
	inactive  time.Duration
	hangtime  time.Duration
	sysErrMax int

	self uint32

	state     State
	started   bool
	huntIdx   int
	dwellEnd  time.Duration
	lastTSCC  time.Duration
	sysErrs   int
	call      Grant
	lastBurst time.Duration
	hangEnd   time.Duration
	inHang    bool
	cleared   Grant
	clearedAt time.Duration
	hasClear  bool
	noCopy    noCopy
}

// NewFollower returns a Follower for cfg. A nil cfg means the zero
// FollowerConfig. It returns a *dmrgo.ConfigError for a HuntLen or
// TalkgroupsLen out of range, a hunt list channel that names no channel, or
// a SysCode whose fields do not fit. It allocates the Follower, and its
// Burst unless cfg.Scratch is set.
func NewFollower(cfg *FollowerConfig) (*Follower, error) {
	f := &Follower{}
	if err := f.init(cfg); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Follower) init(cfg *FollowerConfig) error {
	if cfg == nil {
		return f.init(&FollowerConfig{})
	}
	if cfg.HuntLen < 0 || cfg.HuntLen > MaxChannels {
		return errHunt
	}
	if cfg.TalkgroupsLen < 0 || cfg.TalkgroupsLen > MaxTalkgroups {
		return errTalkgroups
	}
	f.talk, f.talkLen, f.indiv, f.data, f.noFollow = cfg.Talkgroups, cfg.TalkgroupsLen, cfg.Individual, cfg.Data, cfg.NoFollow
	f.noSignal, f.inactive, f.hangtime, f.sysErrMax = cfg.NoSignal, cfg.Inactive, cfg.Hangtime, cfg.SysErrors
	f.ctl.b = cfg.Scratch
	f.ctl.acceptRAS = cfg.AcceptRAS
	if cfg.SysCode != nil {
		if _, err := cfg.SysCode.Pack(); err != nil {
			return errSysCode
		}
		f.ctl.want, f.ctl.hasWant = *cfg.SysCode, true
	}
	if cfg.Plan != nil {
		f.ctl.plan = *cfg.Plan
	}
	for i := range cfg.HuntLen {
		if f.ctl.hunt.Add(HuntEntry{Channel: cfg.Hunt[i]}) != nil {
			return errHunt
		}
	}
	f.ctl.useHunt = cfg.HuntLen > 0
	if f.noSignal == 0 {
		f.noSignal = constants.TNoSignalDefault
	}
	if f.inactive == 0 {
		f.inactive = constants.TVInactiveDefault
	}
	if f.hangtime == 0 {
		f.hangtime = constants.TVHangtimeDefault
	}
	if f.sysErrMax <= 0 {
		f.sysErrMax = constants.NSysErrDefault
	}
	if f.ctl.b == nil {
		f.ctl.b = layer2.NewBurst()
	}
	return nil
}

// State returns the state of the Follower.
func (f *Follower) State() State {
	if f == nil {
		return 0
	}

	return f.state
}

// Site returns what the Follower knows about the TSCC.
func (f *Follower) Site() Site {
	if f == nil {
		return Site{}
	}

	return f.ctl.site
}

// Plan returns a copy of the channel plan, with the channels that CG_AP
// and Chan_Freq announced. The copy is large; Lookup reads one channel
// without it.
func (f *Follower) Plan() ChannelPlan {
	if f == nil {
		return ChannelPlan{}
	}

	return f.ctl.plan
}

// Lookup returns the channel with number n from the channel plan, as
// ChannelPlan.Lookup does. ok is false when no entry and no rule gives it.
func (f *Follower) Lookup(n uint16) (c Channel, ok bool) {
	if f == nil {
		return Channel{}, false
	}

	return f.ctl.plan.Lookup(n)
}

// Hunt returns the hunt list, with the changes of Ann_WD_TSCC broadcasts.
func (f *Follower) Hunt() HuntList {
	if f == nil {
		return HuntList{}
	}

	return f.ctl.hunt
}

// Call returns the grant of the call that the Follower follows. ok is
// false outside StatePayload.
func (f *Follower) Call() (g Grant, ok bool) {
	if f == nil {
		return Grant{}, false
	}

	return f.call, f.state == StatePayload
}

// Reset forgets the site and the call and starts hunting again, as after
// NewFollower. It keeps the config, the channel plan and the hunt list.
func (f *Follower) Reset() {
	if f == nil {
		return
	}

	f.ctl.forget()
	f.state, f.started, f.huntIdx, f.sysErrs = StateHunting, false, 0, 0
	f.inHang, f.hasClear = false, false
}

// Burst decodes the burst raw with what the Hint h says about it, and the
// CACH field before it when cach is not nil, and appends the outputs to
// dst. cach is in transmit order, as bs.Output.CACH and
// TSCCOutput.CACH give it. The time now is on the monotonic clock of the
// caller. On a payload channel only the bursts of the granted slot count,
// so h.Slot should be set when the transport knows it. A nil raw is a
// transport frame without a burst: with Hint.SyncLost it does what Lost
// does, else it runs the timers. It allocates only when dst has no room
// for MaxOutputs outputs.
func (f *Follower) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *[3]byte, dst []Output) []Output {
	if f == nil {
		return dst
	}

	var c *layer2.CACH
	if cach != nil {
		f.cach = layer2.DecodeCACHField(*cach)
		c = &f.cach
	}
	return f.burst(now, raw, h, c, dst)
}

func (f *Follower) burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *layer2.CACH, dst []Output) []Output {
	if raw == nil {
		if h.SyncLost {
			return f.Lost(now, dst)
		}
		return f.timers(now, dst)
	}
	dst = f.timers(now, dst)
	ok := f.ctl.decode(raw, &h)
	if f.state == StatePayload {
		return f.payload(now, ok, &h, dst)
	}
	if cach != nil && !cach.TACT.FEC.Uncorrectable && f.ctl.cach(cach) && f.state == StateControl {
		dst = append(dst, Output{Kind: OutputSite})
	}
	if !ok {
		return dst
	}
	return f.control(now, &h, dst)
}

// Lost tells the Follower that the receiver lost the signal. On a payload
// channel the call ends as on TV_Inactive. It appends the outputs to dst,
// and allocates only when dst has no room for MaxOutputs outputs.
func (f *Follower) Lost(now time.Duration, dst []Output) []Output {
	if f == nil {
		return dst
	}

	dst = f.timers(now, dst)
	if f.state == StatePayload {
		return f.leave(now, ReasonInactive, dst)
	}
	return dst
}

// Tick runs the timers that are due at now and appends their outputs to
// dst. After NewFollower and Reset the first Tick or Burst asks for the
// first hunt list channel. A Tick before Deadline does nothing. It
// allocates only when dst has no room for MaxOutputs outputs.
func (f *Follower) Tick(now time.Duration, dst []Output) []Output {
	if f == nil {
		return dst
	}

	return f.timers(now, dst)
}

// Deadline returns the time at which the Follower needs a Tick. ok is
// false when no timer runs.
func (f *Follower) Deadline() (at time.Duration, ok bool) {
	if f == nil {
		return 0, false
	}

	if !f.started && f.ctl.useHunt && f.ctl.hunt.Len() > 0 {
		return 0, true
	}
	switch f.state {
	case StateHunting:
		if f.started && f.ctl.hunt.Len() > 0 && f.noSignal > 0 {
			return f.dwellEnd, true
		}
	case StateControl:
		if f.noSignal > 0 {
			return f.lastTSCC + f.noSignal, true
		}
	case StatePayload:
		if f.inactive > 0 {
			at, ok = f.lastBurst+f.inactive, true
		}
		if f.inHang && (!ok || f.hangEnd < at) {
			at, ok = f.hangEnd, true
		}
	}
	return at, ok
}

func (f *Follower) timers(now time.Duration, out []Output) []Output {
	for range 4 {
		at, ok := f.Deadline()
		if !ok || now < at {
			return out
		}
		switch f.state {
		case StateHunting:
			if f.started {
				f.huntIdx++
			}
			out = f.huntNext(now, out)
		case StateControl:
			out = f.lose(now, ReasonNoSignal, out)
		case StatePayload:
			reason := ReasonInactive
			if f.inHang && at == f.hangEnd {
				reason = ReasonHangtime
			}
			out = f.leave(now, reason, out)
		}
	}
	return out
}

func (f *Follower) huntNext(now time.Duration, out []Output) []Output {
	f.started = true
	n := f.ctl.hunt.Len()
	if n == 0 {
		return out
	}
	f.huntIdx %= n
	t := f.ctl.huntTune(f.huntIdx)
	f.ctl.forget()
	f.ctl.site.Channel = t.Channel
	f.state, f.dwellEnd, f.sysErrs = StateHunting, now+f.noSignal, 0
	return append(out, Output{Kind: OutputTune, Tune: t})
}

func (f *Follower) lose(now time.Duration, reason Reason, out []Output) []Output {
	out = append(out, Output{Kind: OutputLost, Reason: reason})
	f.state = StateHunting
	if f.ctl.useHunt && f.ctl.hunt.Len() > 0 {
		f.huntIdx++
		return f.huntNext(now, out)
	}
	ch := f.ctl.site.Channel
	f.ctl.forget()
	f.ctl.site.Channel = ch
	f.started = true
	return out
}

func (f *Follower) control(now time.Duration, h *layer2.Hint, out []Output) []Output {
	ev := f.ctl.tscc(now, h.Slot)
	switch ev {
	case eventNone:
		return out
	case eventOther:
		f.sysErrs++
		if f.state == StateControl && f.sysErrs >= f.sysErrMax {
			return f.lose(now, ReasonSysCode, out)
		}
		return out
	case eventTSCC, eventMove:
		if f.state == StateControl {
			f.lastTSCC = now
		}
		return out
	case eventUDT:
		if f.state == StateControl {
			f.lastTSCC = now
			out = append(out, Output{Kind: OutputUDT})
		}
		return out
	case eventSystem, eventSiteNews, eventGrant:
	}
	f.sysErrs = 0
	if f.state == StateHunting {
		if !f.ctl.site.HasSysCode {
			return out
		}
		f.state, f.started = StateControl, true
		out = append(out, Output{Kind: OutputSite})
	} else if ev == eventSiteNews {
		out = append(out, Output{Kind: OutputSite})
	}
	f.lastTSCC = now
	if ev != eventGrant {
		return out
	}
	g := &f.ctl.grant
	out = append(out, Output{Kind: OutputGrant, Grant: *g})
	if !f.wants(now, g) {
		return out
	}
	f.call, f.lastBurst, f.inHang = *g, now, false
	f.state = StatePayload
	return append(out, Output{Kind: OutputTune, Tune: g.tune()})
}

func (f *Follower) wants(now time.Duration, g *Grant) bool {
	if f.noFollow || g.Channel == ChannelNull || g.Channel == ChannelAbsolute {
		return false
	}
	if f.hasClear && now-f.clearedAt < clearHold && g.LateEntry && sameCall(&f.cleared, g) {
		return false
	}
	if f.self != 0 && (g.Service == enums.ServiceDuplexVoice || g.Service == enums.ServiceDuplexPacketData) {
		return g.Target == f.self
	}
	if f.self != 0 && (g.Source == f.self || !g.Group && g.Target == f.self) {
		return !g.Data() || f.data
	}
	switch {
	case g.Data():
		if !f.data {
			return false
		}
	case !g.Group:
		return f.indiv && (f.self == 0 || g.Target == f.self || g.Source == f.self)
	}
	if !g.Group || f.talkLen == 0 && f.self == 0 {
		return true
	}
	for i := range f.talkLen {
		if f.talk[i] == g.Target {
			return true
		}
	}
	return f.dynamic(g.Target)
}

func (f *Follower) dynamic(tg uint32) bool {
	for _, d := range f.dyn {
		if d != 0 && d == tg {
			return true
		}
	}
	return false
}

func (f *Follower) payload(now time.Duration, ok bool, h *layer2.Hint, out []Output) []Output {
	if h.Slot != enums.SlotUnknown && h.Slot != f.call.Slot {
		return out
	}
	if !ok {
		return out
	}
	b := f.ctl.b
	if f.call.HasColorCode && b.Kind == layer2.KindData && b.SlotType.ColorCode != f.call.ColorCode {
		return out
	}
	f.lastBurst = now
	if b.Kind == layer2.KindVoice {
		f.inHang = false
		return out
	}
	if b.Kind != layer2.KindData || b.FEC.SlotType.Uncorrectable {
		return out
	}
	switch b.SlotType.DataType {
	case enums.DataTypeTerminatorWithLC:
		if f.hangtime > 0 && !f.call.Data() {
			f.inHang, f.hangEnd = true, now+f.hangtime
		}
	case enums.DataTypeVoiceLCHeader:
		f.inHang = false
	case enums.DataTypeCSBK:
		c, isCSBK := b.Data.(*pdu.CSBK)
		if isCSBK && f.ctl.dataOK() && c.FID == enums.FIDStandardized && c.ClearPDU != nil && f.clears(c.ClearPDU) {
			f.cleared, f.clearedAt, f.hasClear = f.call, now, true
			out = f.leave(now, ReasonClear, out)
			if ch := c.ClearPDU.PhysicalChannel; ch != ChannelNull && ch != ChannelAbsolute && ch != f.ctl.site.Channel {
				out = out[:len(out)-1]
				out = f.moveTo(now, ch, out)
			}
			return out
		}
	default:
	}
	return out
}

func (f *Follower) clears(p *pdu.ClearPDU) bool {
	if p.TargetAddress == constants.GatewayALLMSI || p.TargetAddress == constants.AllMSID {
		return true
	}
	if p.GroupIndividual {
		return f.call.Group && p.TargetAddress == f.call.Target
	}
	return !f.call.Group && (p.TargetAddress == f.call.Target || p.TargetAddress == f.call.Source)
}

func (f *Follower) moveTo(now time.Duration, ch uint16, out []Output) []Output {
	t := Tune{Channel: ch}
	if c, ok := f.ctl.plan.Lookup(ch); ok {
		t.RXHz, t.TXHz = c.RXHz, c.TXHz
	}
	f.ctl.forget()
	f.ctl.site.Channel = ch
	f.state, f.started, f.dwellEnd, f.sysErrs = StateHunting, true, now+f.noSignal, 0
	return append(out, Output{Kind: OutputTune, Tune: t})
}

// UDT returns the UDT delivery of the last OutputUDT. It points into the
// Follower and holds until the next call of a method of the Follower.
func (f *Follower) UDT() *UDT {
	if f == nil {
		return nil
	}

	return &f.ctl.udt
}

func (f *Follower) leave(now time.Duration, reason Reason, out []Output) []Output {
	out = append(out, Output{Kind: OutputCallEnd, Grant: f.call, Reason: reason})
	f.state, f.inHang, f.lastTSCC = StateControl, false, now
	f.ctl.slc.Reset()
	return append(out, Output{Kind: OutputTune, Tune: f.ctl.tsccTune()})
}
