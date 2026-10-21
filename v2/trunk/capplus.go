package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// CapacityPlusNoSignalDefault is the default CapacityPlusConfig.NoSignal:
// more than twice the 2 s beacon period of an idle rest channel.
const CapacityPlusNoSignalDefault = 5 * time.Second

// CapacityPlusConfig configures NewCapacityPlusFollower. A nil config means
// the zero CapacityPlusConfig.
type CapacityPlusConfig struct {
	// Plan, when set, maps the repeaters of the site to frequencies:
	// channel number n is repeater n, which carries LSN 2n-1 on timeslot 1
	// and LSN 2n on timeslot 2. The Follower copies it and hunts over the
	// repeaters that it names, in number order. Without a plan the
	// Follower hunts nothing, and the consumer keeps the receiver on one
	// repeater.
	Plan *ChannelPlan
	// Talkgroups holds the first TalkgroupsLen talkgroups to follow.
	// TalkgroupsLen 0 follows every talkgroup.
	Talkgroups [MaxTalkgroups]uint32
	// TalkgroupsLen is the number of entries of Talkgroups in use.
	TalkgroupsLen int
	// Individual follows private calls too: private voice grants, and the
	// radio entries of a site status, which can also be data.
	Individual bool
	// NoFollow reports calls with OutputGrant but never leaves the rest
	// channel, for a receiver that cannot tune.
	NoFollow bool
	// NoSignal is the time without a valid Capacity Plus PDU on the rest
	// channel after which the Follower hunts again, and the time it
	// listens on each repeater while it hunts. 0 means
	// CapacityPlusNoSignalDefault, and a negative value turns the timer
	// off.
	NoSignal time.Duration
	// Inactive is the time without a burst of the call after which the
	// Follower returns to the rest channel. 0 means
	// constants.TVInactiveDefault, and a negative value turns the timer
	// off.
	Inactive time.Duration
	// Hangtime is the time after a Terminator with LC in which the
	// Follower waits for more voice. 0 means constants.TVHangtimeDefault,
	// and a negative value turns the timer off.
	Hangtime time.Duration
	// Scratch, when set, is the Burst that the Follower decodes into. Nil
	// means the Follower makes its own.
	Scratch *layer2.Burst
}

// CapacityPlusFollower follows a MOTOTRBO Capacity Plus or Linked Capacity
// Plus site without transmitting. It finds the rest channel, where every
// call starts, moves with it when a call takes it, and follows the calls
// it wants: from the group or private voice grant in the Voice LC Header
// on the rest channel, or by late entry from the channel activity of a
// site status. Output.Tune and Grant name the repeater in Channel and the
// timeslot of the LSN in Slot. A Grant from a site status has Opcode
// pdu.CSBKCapacityPlusSiteStatus and LateEntry set; one from a Voice LC
// Header has Opcode 0. No public spec describes the protocol; the decoding
// follows recordings of real systems. Make it with NewCapacityPlusFollower.
// A CapacityPlusFollower is not safe for concurrent use and must not be
// copied.
type CapacityPlusFollower struct {
	plan     ChannelPlan
	talk     [MaxTalkgroups]uint32
	talkLen  int
	indiv    bool
	noFollow bool
	noSignal time.Duration
	inactive time.Duration
	hangtime time.Duration

	b      *layer2.Burst
	opts   layer2.DecodeOptions
	slc    layer2.ShortLCAssembler
	slcOut pdu.ShortLC
	slcP   pdu.ShortLCPayloads
	status [2]pdu.CapacityPlusSiteStatusAssembler

	state     State
	started   bool
	repeater  uint16
	huntNext  uint16
	dwellEnd  time.Duration
	lastRest  time.Duration
	rest      uint8
	site      uint8
	hasSite   bool
	call      Grant
	lastBurst time.Duration
	hangEnd   time.Duration
	inHang    bool
	noCopy    noCopy
}

// NewCapacityPlusFollower returns a CapacityPlusFollower for cfg. A nil
// cfg means the zero CapacityPlusConfig. It returns a *ConfigError for a
// TalkgroupsLen out of range. It allocates the Follower, and its Burst
// unless cfg.Scratch is set.
func NewCapacityPlusFollower(cfg *CapacityPlusConfig) (*CapacityPlusFollower, error) {
	var zero CapacityPlusConfig
	if cfg == nil {
		cfg = &zero
	}
	if cfg.TalkgroupsLen < 0 || cfg.TalkgroupsLen > MaxTalkgroups {
		return nil, &ConfigError{Field: "TalkgroupsLen"}
	}
	f := &CapacityPlusFollower{
		talk: cfg.Talkgroups, talkLen: cfg.TalkgroupsLen,
		indiv: cfg.Individual, noFollow: cfg.NoFollow,
		noSignal: orDefault(cfg.NoSignal, CapacityPlusNoSignalDefault),
		inactive: orDefault(cfg.Inactive, constants.TVInactiveDefault),
		hangtime: orDefault(cfg.Hangtime, constants.TVHangtimeDefault),
		b:        cfg.Scratch,
	}
	if cfg.Plan != nil {
		f.plan = *cfg.Plan
	}
	if f.b == nil {
		f.b = &layer2.Burst{}
	}
	return f, nil
}

// State returns the state of the Follower: StateHunting, StateControl on
// the rest channel, or StatePayload in a call.
func (f *CapacityPlusFollower) State() State {
	return f.state
}

// RestLSN returns the last rest LSN that the site announced. ok is false
// before the Follower found the rest channel.
func (f *CapacityPlusFollower) RestLSN() (lsn uint8, ok bool) {
	return f.rest, f.rest != 0
}

// SiteNumber returns the site number of a Linked Capacity Plus site, from
// the rest channel Short LC or the neighbor report. ok is false before
// either arrived.
func (f *CapacityPlusFollower) SiteNumber() (site uint8, ok bool) {
	return f.site, f.hasSite
}

// Call returns the grant of the call that the Follower follows. ok is
// false outside a call.
func (f *CapacityPlusFollower) Call() (g Grant, ok bool) {
	return f.call, f.state == StatePayload
}

// Reset forgets the site and the call and starts hunting again, as after
// NewCapacityPlusFollower.
func (f *CapacityPlusFollower) Reset() {
	f.slc.Reset()
	f.status[0].Reset()
	f.status[1].Reset()
	f.state, f.started, f.repeater, f.huntNext = StateHunting, false, 0, 0
	f.rest, f.site, f.hasSite, f.inHang = 0, 0, false, false
	f.call = Grant{}
}

// Burst decodes the burst raw with what the Hint h says about it, and the
// CACH before it when cach is not nil, and appends the outputs to dst.
// h.Slot, or the TDMA channel of the CACH, tells the timeslot of the
// burst. It allocates only when dst has no room for MaxOutputs outputs.
func (f *CapacityPlusFollower) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *layer2.CACH, dst []Output) []Output {
	dst = f.timers(now, dst)
	slot := h.Slot
	if cach != nil && !cach.TACT.FEC.Uncorrectable {
		if slot == enums.SlotUnknown {
			slot = enums.Slot1
			if cach.TACT.TDMAChannel {
				slot = enums.Slot2
			}
		}
		dst = f.shortLC(now, cach, dst)
	}
	f.opts.Kind = h.Kind
	f.opts.VoiceBurst = enums.VoiceBurstUnknown
	if h.Kind == layer2.KindVoice {
		f.opts.VoiceBurst = h.VoiceBurst
	}
	if f.b.DecodeFromBytesWith(*raw, &f.opts) != nil {
		return dst
	}
	if f.state == StatePayload && slot == f.call.Slot {
		f.payload(now)
	}
	if f.b.Kind != layer2.KindData || f.b.FEC.SlotType.Uncorrectable || f.b.FEC.PDU.Uncorrectable {
		return dst
	}
	switch d := f.b.Data.(type) {
	case *pdu.CSBK:
		return f.csbk(now, d, dst)
	case *pdu.FullLinkControl:
		return f.fullLC(now, d, slot, dst)
	}
	return dst
}

// Lost tells the Follower that the receiver lost the signal. In a call it
// returns to the rest channel as on Inactive. It appends the outputs to
// dst, and allocates only when dst has no room for MaxOutputs outputs.
func (f *CapacityPlusFollower) Lost(now time.Duration, dst []Output) []Output {
	dst = f.timers(now, dst)
	if f.state == StatePayload {
		return f.leave(now, ReasonInactive, dst)
	}
	return dst
}

// Tick runs the timers that are due at now and appends their outputs to
// dst. After NewCapacityPlusFollower and Reset, with a plan, the first
// Tick or Burst asks for the first repeater. It allocates only when dst
// has no room for MaxOutputs outputs.
func (f *CapacityPlusFollower) Tick(now time.Duration, dst []Output) []Output {
	return f.timers(now, dst)
}

// Deadline returns the time at which the Follower needs a Tick. ok is
// false when no timer runs.
func (f *CapacityPlusFollower) Deadline() (at time.Duration, ok bool) {
	hunts := f.plan.Len() > 0 || f.plan.nRules > 0
	if !f.started && hunts {
		return 0, true
	}
	switch f.state {
	case StateHunting:
		if hunts && f.noSignal > 0 {
			return f.dwellEnd, true
		}
	case StateControl:
		if f.noSignal > 0 {
			return f.lastRest + f.noSignal, true
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

func (f *CapacityPlusFollower) timers(now time.Duration, out []Output) []Output {
	for range 4 {
		at, ok := f.Deadline()
		if !ok || now < at {
			return out
		}
		switch f.state {
		case StateHunting:
			out = f.hunt(now, out)
		case StateControl:
			out = append(out, Output{Kind: OutputLost, Reason: ReasonNoSignal})
			f.state, f.rest = StateHunting, 0
			out = f.hunt(now, out)
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

func (f *CapacityPlusFollower) hunt(now time.Duration, out []Output) []Output {
	f.started = true
	f.dwellEnd = now + f.noSignal
	for range pdu.CapacityPlusMaxLSN / 2 {
		f.huntNext = f.huntNext%(pdu.CapacityPlusMaxLSN/2) + 1
		if c, ok := f.plan.Lookup(f.huntNext); ok {
			f.repeater = f.huntNext
			return append(out, Output{Kind: OutputTune, Tune: Tune{Channel: c.Number, RXHz: c.RXHz, TXHz: c.TXHz}})
		}
	}
	return out
}

func (f *CapacityPlusFollower) tune(lsn uint8) Tune {
	r, slot, _ := pdu.CapacityPlusChannel(lsn)
	t := Tune{Channel: uint16(r), Slot: slot}
	if c, ok := f.plan.Lookup(t.Channel); ok {
		t.RXHz, t.TXHz = c.RXHz, c.TXHz
	}
	return t
}

func (f *CapacityPlusFollower) restSeen(now time.Duration, lsn uint8, out []Output) []Output {
	if lsn == 0 || lsn > pdu.CapacityPlusMaxLSN {
		return out
	}
	moved := lsn != f.rest
	f.rest = lsn
	if f.state == StatePayload {
		return out
	}
	r, _, _ := pdu.CapacityPlusChannel(lsn)
	if f.state == StateHunting {
		f.state = StateControl
		f.lastRest = now
		out = append(out, Output{Kind: OutputSite})
	}
	if uint16(r) == f.repeater || f.repeater == 0 {
		f.repeater = uint16(r)
		f.lastRest = now
		return out
	}
	if moved && !f.noFollow {
		f.repeater, f.lastRest = uint16(r), now
		out = append(out, Output{Kind: OutputTune, Tune: f.tune(lsn)})
	}
	return out
}

func (f *CapacityPlusFollower) shortLC(now time.Duration, cach *layer2.CACH, out []Output) []Output {
	done, err := f.slc.AddInto(&f.slcOut, &f.slcP, *cach)
	if err != nil || !done || f.slcOut.FEC.Uncorrectable {
		return out
	}
	p, ok := f.slcOut.AsCapacityPlusRestChannel()
	if !ok {
		return out
	}
	f.site, f.hasSite = p.Site, true
	return f.restSeen(now, p.RestLSN, out)
}

func (f *CapacityPlusFollower) csbk(now time.Duration, c *pdu.CSBK, out []Output) []Output {
	if p, ok := c.AsCapacityPlusSiteStatus(); ok {
		i := 0
		if p.Slot2 {
			i = 1
		}
		out = f.restSeen(now, p.RestLSN, out)
		if done, err := f.status[i].Add(&p); done && err == nil {
			act, _ := f.status[i].Activity()
			out = f.activity(now, &act, out)
		}
		return out
	}
	if p, ok := c.AsCapacityPlusNeighbors(); ok {
		f.site, f.hasSite = p.Site, true
		return f.restSeen(now, p.RestLSN, out)
	}
	if p, ok := c.AsCapacityPlusPreamble(); ok && f.state != StatePayload {
		f.rest = p.RestLSN
		if f.state == StateControl {
			f.lastRest = now
		}
	}
	return out
}

func (f *CapacityPlusFollower) activity(now time.Duration, a *pdu.CapacityPlusActivity, out []Output) []Output {
	if f.state != StateControl {
		return out
	}
	for lsn := uint8(1); lsn <= pdu.CapacityPlusMaxLSN; lsn++ {
		g := Grant{Opcode: pdu.CSBKCapacityPlusSiteStatus, LateEntry: true}
		if tg, ok := a.Talkgroup(lsn); ok && f.wantsGroup(uint32(tg)) {
			g.Service, g.Group, g.Target = enums.ServiceTalkgroupVoice, true, uint32(tg)
		} else if r, ok := a.RadioTarget(lsn); ok && f.indiv {
			g.Service, g.Target = enums.ServiceIndividualVoice, uint32(r)
		} else {
			continue
		}
		return f.follow(now, g, lsn, out)
	}
	return out
}

func (f *CapacityPlusFollower) fullLC(now time.Duration, lc *pdu.FullLinkControl, slot enums.Slot, out []Output) []Output {
	if f.b.SlotType.DataType == enums.DataTypeTerminatorWithLC {
		if f.state == StatePayload && slot == f.call.Slot && f.hangtime > 0 {
			f.inHang, f.hangEnd = true, now+f.hangtime
		}
		return out
	}
	if f.b.SlotType.DataType != enums.DataTypeVoiceLCHeader || f.state != StateControl {
		return out
	}
	var g Grant
	var rest uint8
	if p, ok := lc.AsCapacityPlusGroupVoiceGrant(); ok {
		g = Grant{Service: enums.ServiceTalkgroupVoice, Group: true, Target: uint32(p.GroupAddress), Source: uint32(p.SourceAddress), Emergency: p.ServiceOptions.IsEmergency}
		rest = p.RestLSN
		if !f.wantsGroup(g.Target) {
			return f.restSeen(now, rest, out)
		}
	} else if p, ok := lc.AsCapacityPlusPrivateVoiceGrant(); ok {
		g = Grant{Service: enums.ServiceIndividualVoice, Target: p.TargetAddress, Source: uint32(p.SourceAddress), Emergency: p.ServiceOptions.IsEmergency}
		rest = p.RestLSN
		if !f.indiv {
			return f.restSeen(now, rest, out)
		}
	} else {
		return out
	}
	if f.repeater == 0 || slot == enums.SlotUnknown {
		return out
	}
	lsn := 2*uint8(f.repeater&0x7F) - 1
	if slot == enums.Slot2 {
		lsn++
	}
	out = f.follow(now, g, lsn, out)
	if rest != 0 && rest <= pdu.CapacityPlusMaxLSN {
		f.rest = rest
	}
	return out
}

func (f *CapacityPlusFollower) wantsGroup(tg uint32) bool {
	if f.talkLen == 0 {
		return true
	}
	for _, t := range f.talk[:f.talkLen] {
		if t == tg {
			return true
		}
	}
	return false
}

func (f *CapacityPlusFollower) follow(now time.Duration, g Grant, lsn uint8, out []Output) []Output {
	t := f.tune(lsn)
	g.Channel, g.Slot, g.RXHz, g.TXHz = t.Channel, t.Slot, t.RXHz, t.TXHz
	out = append(out, Output{Kind: OutputGrant, Grant: g})
	if f.noFollow {
		return out
	}
	f.call, f.state, f.inHang = g, StatePayload, false
	f.lastBurst = now
	if t.Channel != f.repeater {
		f.repeater = t.Channel
		out = append(out, Output{Kind: OutputTune, Tune: t})
	}
	return out
}

func (f *CapacityPlusFollower) payload(now time.Duration) {
	f.lastBurst = now
	if f.b.Kind == layer2.KindVoice {
		f.inHang = false
	}
}

func (f *CapacityPlusFollower) leave(now time.Duration, reason Reason, out []Output) []Output {
	out = append(out, Output{Kind: OutputCallEnd, Grant: f.call, Reason: reason})
	f.state, f.inHang, f.call = StateControl, false, Grant{}
	if f.rest == 0 {
		f.state = StateHunting
		return out
	}
	t := f.tune(f.rest)
	f.repeater = t.Channel
	f.lastRest = now
	return append(out, Output{Kind: OutputTune, Tune: t})
}
