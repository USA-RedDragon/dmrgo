package trunk

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// MaxChannels is the number of channels that a ChannelPlan and a HuntList
// hold, the storage of a hunt list in ETSI TS 102 361-4 Annex D.
const MaxChannels = 64

// MaxRules is the number of rules that a ChannelPlan holds.
const MaxRules = 4

// ChannelNull is CHNULL, the channel number that names no channel (ETSI
// TS 102 361-4 clause 7.2.19.1).
const ChannelNull = 0

// ChannelAbsolute is the channel number of a grant or C_MOVE whose
// frequencies follow in a CG_AP or MV_AP block (ETSI TS 102 361-4 clause
// 7.1.1.1.1).
const ChannelAbsolute = 0xFFF

// Channel is a logical physical channel number (LPCN) of a site with its
// frequencies, as an MS uses them (ETSI TS 102 361-4 Annex C).
type Channel struct {
	// Number is the 12-bit channel number, 1 to 4094.
	Number uint16
	// RXHz is the frequency that the MS receives, the outbound channel of
	// the BS.
	RXHz uint64
	// TXHz is the frequency that the MS transmits, the inbound channel of
	// the BS. It is 0 for a receive-only plan.
	TXHz uint64
}

// ChannelRule gives the frequencies of a range of channel numbers by a
// fixed step, as the channel plan of a site or a radio codeplug does:
// channel n receives on RXHz + (n - First) * StepHz and transmits
// TXOffsetHz from there.
type ChannelRule struct {
	// First is the first channel number of the rule.
	First uint16
	// Last is the last channel number of the rule, at least First.
	Last uint16
	// RXHz is the receive frequency of channel First.
	RXHz uint64
	// StepHz is the frequency step from one channel number to the next.
	StepHz uint64
	// TXOffsetHz is the transmit frequency minus the receive frequency.
	TXOffsetHz int64
}

// ChannelPlan maps the channel numbers of a site to frequencies. Entries
// from Add take precedence over rules from AddRule, and a later Add of the
// same number replaces the entry. The zero ChannelPlan is empty and ready
// to use. It has no pointers, so it can be copied.
type ChannelPlan struct {
	channels [MaxChannels]Channel
	n        int
	rules    [MaxRules]ChannelRule
	nRules   int
}

// Add adds c to the plan, or replaces the entry with the same number. It
// returns ErrChannelNumber for a number that is CHNULL, ChannelAbsolute or
// over 12 bits, and ErrPlanFull when MaxChannels other numbers are in the
// plan.
func (p *ChannelPlan) Add(c Channel) error {
	if p == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.Number == ChannelNull || c.Number >= ChannelAbsolute {
		return ErrChannelNumber
	}
	for i := range p.n {
		if p.channels[i].Number == c.Number {
			p.channels[i] = c
			return nil
		}
	}
	if p.n == MaxChannels {
		return ErrPlanFull
	}
	p.channels[p.n] = c
	p.n++
	return nil
}

// AddCdef adds the channel of the CdefParms of a CG_AP, MV_AP or BC_AP
// block to the plan, as Add does. Its TX and RX frequencies are those of
// the MS. It returns an error for a nil c (ETSI TS 102 361-4 clause
// C.1.1.4).
func (p *ChannelPlan) AddCdef(c *layer3.CdefParms) error {
	if p == nil {
		return dmrgo.ErrNilReceiver
	}

	if c == nil {
		return dmrgo.ErrNilArgument
	}
	return p.Add(Channel{Number: c.Channel, RXHz: c.RXFrequencyHz(), TXHz: c.TXFrequencyHz()})
}

// AddRule adds r to the plan. A rule added earlier wins where two rules
// overlap. It returns ErrChannelNumber when r.First is CHNULL, r.Last is
// below r.First or not below ChannelAbsolute, and ErrPlanFull when the plan
// holds MaxRules rules.
func (p *ChannelPlan) AddRule(r ChannelRule) error {
	if p == nil {
		return dmrgo.ErrNilReceiver
	}

	if r.First == ChannelNull || r.Last < r.First || r.Last >= ChannelAbsolute {
		return ErrChannelNumber
	}
	if p.nRules == MaxRules {
		return ErrPlanFull
	}
	p.rules[p.nRules] = r
	p.nRules++
	return nil
}

// Lookup returns the channel with number n. ok is false when no entry and
// no rule gives it.
func (p *ChannelPlan) Lookup(n uint16) (c Channel, ok bool) {
	if p == nil {
		return Channel{}, false
	}

	for i := range p.n {
		if p.channels[i].Number == n {
			return p.channels[i], true
		}
	}
	for i := range p.nRules {
		r := &p.rules[i]
		if n >= r.First && n <= r.Last {
			c = Channel{Number: n, RXHz: r.RXHz + uint64(n-r.First)*r.StepHz}
			if r.TXOffsetHz < 0 {
				c.TXHz = c.RXHz - uint64(-r.TXOffsetHz)
			} else {
				c.TXHz = c.RXHz + uint64(r.TXOffsetHz)
			}
			return c, true
		}
	}
	return c, false
}

// Len returns the number of entries that Add made.
func (p *ChannelPlan) Len() int {
	if p == nil {
		return 0
	}

	return p.n
}

// HuntEntry is a channel of a HuntList.
type HuntEntry struct {
	// Channel is the channel number.
	Channel uint16
	// ColorCode is the color code of the TSCC on the channel, when
	// HasColorCode is set.
	ColorCode enums.ColorCode
	// HasColorCode reports whether ColorCode is known.
	HasColorCode bool
}

// HuntList is the list of channels on which an MS looks for a TSCC (ETSI
// TS 102 361-4 clause 6.3.2, Annex D). An Ann_WD_TSCC broadcast adds and
// withdraws channels. The zero HuntList is empty and ready to use.
type HuntList struct {
	entries [MaxChannels]HuntEntry
	n       int
}

// Add adds e to the end of the list, or updates the entry with the same
// channel. It returns ErrChannelNumber for CHNULL or a number that is not
// below ChannelAbsolute, and ErrPlanFull when the list is full.
func (l *HuntList) Add(e HuntEntry) error {
	if l == nil {
		return dmrgo.ErrNilReceiver
	}

	if e.Channel == ChannelNull || e.Channel >= ChannelAbsolute {
		return ErrChannelNumber
	}
	if i := l.index(e.Channel); i >= 0 {
		l.entries[i] = e
		return nil
	}
	if l.n == MaxChannels {
		return ErrPlanFull
	}
	l.entries[l.n] = e
	l.n++
	return nil
}

// Withdraw removes the channel from the list and keeps the order of the
// others. It reports whether the channel was in the list.
func (l *HuntList) Withdraw(channel uint16) bool {
	if l == nil {
		return false
	}

	i := l.index(channel)
	if i < 0 {
		return false
	}
	copy(l.entries[i:l.n], l.entries[i+1:l.n])
	l.n--
	l.entries[l.n] = HuntEntry{}
	return true
}

// Len returns the number of channels in the list.
func (l *HuntList) Len() int {
	if l == nil {
		return 0
	}

	return l.n
}

// At returns entry i, for i from 0 to Len() - 1. It returns the zero
// HuntEntry for any other i.
func (l *HuntList) At(i int) HuntEntry {
	if l == nil {
		return HuntEntry{}
	}

	if i < 0 || i >= l.n {
		return HuntEntry{}
	}
	return l.entries[i]
}

func (l *HuntList) index(channel uint16) int {
	for i := range l.n {
		if l.entries[i].Channel == channel {
			return i
		}
	}
	return -1
}
