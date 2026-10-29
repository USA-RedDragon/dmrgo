package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// PreamblesConfig configures NewPreambles. A nil config means the zero
// PreamblesConfig.
type PreamblesConfig struct {
	// Scratch, when set, is the Burst that Burst decodes into, as for
	// RequesterConfig.Scratch.
	Scratch *layer2.Burst
}

// PreamblesState is the state of a Preambles.
type PreamblesState uint8

// The PreamblesState values.
const (
	// PreamblesIdle follows no announced transfer.
	PreamblesIdle PreamblesState = iota
	// PreamblesAnnounced waits for the CSBK or data that Pre_CSBKs
	// announced.
	PreamblesAnnounced
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s PreamblesState) String() string {
	switch s {
	case PreamblesIdle:
		return idleName
	case PreamblesAnnounced:
		return "Announced"
	}
	return "PreamblesState(" + text.Int(int64(s)) + ")"
}

// Preambles follows the Pre_CSBKs of one logical channel. A Pre_CSBK with
// CSBK Blocks to Follow n announces n more blocks: Pre_CSBKs, then a CSBK or
// a data message, with the target and source addresses of the Pre_CSBK.
// Preambles reports the first Pre_CSBK, the CSBK, MBC header or data header
// that follows with the same target and source addresses, or that nothing
// followed by the end of the announced blocks. A CSBK, MBC header or data
// header with other addresses, or without both, is not joined: it ends the
// announced transfer as one that did not come. A repeater
// may send more Pre_CSBKs than the radio did, each with CSBK Blocks to
// Follow 1; each one moves the end. Make it with NewPreambles; a zero
// Preambles does nothing. A Preambles
// is not safe for concurrent use and must not be copied; go vet reports a
// copy. (ETSI TS 102 361-2 clause 7.1.2.5, Table 7.7, and clause 7.2.7)
type Preambles struct {
	d      decoder
	state  PreamblesState
	target uint32
	source uint32
	group  bool
	data   bool
	until  time.Duration
	noCopy noCopy
}

// NewPreambles returns a Preambles for cfg. A nil cfg means the zero
// PreamblesConfig. No PreamblesConfig is out of range, so the error is
// nil; it has the shape of the other machine constructors. It allocates
// the Preambles, and its Burst unless cfg.Scratch is set.
func NewPreambles(cfg *PreamblesConfig) (*Preambles, error) {
	var conf PreamblesConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	return &Preambles{d: newDecoder(cfg.Scratch)}, nil
}

// Reset clears the announced transfer without an output.
func (p *Preambles) Reset() {
	if p == nil {
		return
	}
	p.state = PreamblesIdle
}

// State returns the state of the Preambles.
func (p *Preambles) State() PreamblesState {
	if p == nil {
		return 0
	}
	return p.state
}

// Burst decodes the burst raw with the data in the Hint h, runs it
// through the Preambles as Decoded does, and appends the outputs to dst. A
// nil raw, a transport frame without a burst, only runs the timers, as
// Lost does. It allocates only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if p == nil {
		return dst
	}
	return p.Decoded(now, p.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil, and appends the
// outputs to dst: OutputAnnounced for the first Pre_CSBK of a transfer,
// OutputJoined for the CSBK, MBC header or data header after it with the
// target and source addresses of the Pre_CSBKs, and OutputExpired for a
// transfer that a Pre_CSBK with other addresses replaces, or that a CSBK, MBC
// header or data header with other addresses follows. Preambles gives no
// output for such a burst itself. A CSBK whose CRC fails is dropped. It
// allocates only when dst has no room for MaxOutputs outputs. (ETSI TS 102
// 361-2 clause 7.1.2.5, Table 7.7)
func (p *Preambles) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if p == nil || p.d.b == nil {
		return dst
	}
	dst = p.Tick(now, dst)
	kind, c, octets := classify(b)
	switch kind {
	case rxCSBK:
		if pre, ok := c.AsPreamble(); ok {
			return p.preamble(now, &pre, dst)
		}
		target, source, ok := addresses(c)
		return p.join(dst, false, &octets, ok && p.parties(target, source))
	case rxMBCHeader:
		h, _ := b.Data.(*pdu.MBCHeader)
		target, source, ok := mbcAddresses(h)
		return p.join(dst, false, nil, ok && p.parties(target, source))
	case rxDataHeader:
		h, _ := b.Data.(*pdu.DataHeader)
		target, source, ok := h.Addresses()
		return p.join(dst, true, nil, ok && p.parties(target, source))
	case rxNone, rxOther:
	}
	return dst
}

func (p *Preambles) preamble(now time.Duration, pre *pdu.PreamblePDU, out []Output) []Output {
	until := now + time.Duration(pre.CSBKBlocksToFollow)*BurstInterval
	if p.state == PreamblesAnnounced {
		if pre.TargetAddress == p.target && pre.SourceAddress == p.source && pre.Group == p.group && pre.Data == p.data {
			p.until = max(p.until, until)
			return out
		}
		out = p.appendAt(out, OutputExpired)
	}
	p.state = PreamblesAnnounced
	p.target, p.source, p.group, p.data, p.until = pre.TargetAddress, pre.SourceAddress, pre.Group, pre.Data, until
	return p.appendAt(out, OutputAnnounced)
}

func (p *Preambles) parties(target, source uint32) bool {
	return target == p.target && source == p.source
}

func (p *Preambles) join(out []Output, data bool, c *pdu.CSBKBytes, same bool) []Output {
	if p.state != PreamblesAnnounced {
		return out
	}
	if !same {
		p.state = PreamblesIdle
		return p.appendAt(out, OutputExpired)
	}
	out = p.appendAt(out, OutputJoined)
	o := &out[len(out)-1]
	o.Data, o.Until = data, 0
	if c != nil {
		o.CSBK = *c
	}
	p.state = PreamblesIdle
	return out
}

func (p *Preambles) appendAt(out []Output, k OutputKind) []Output {
	var o *Output
	out, o = appendOutput(out, k)
	o.Call, o.Data, o.Until = layer2.Call{Group: p.group, Target: p.target, Source: p.source}, p.data, p.until
	return out
}

// Lost records that the transport lost the signal. It runs the
// timers as Tick does; the announced transfer can still come. It allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Lost(now time.Duration, dst []Output) []Output {
	if p == nil {
		return dst
	}
	return p.Tick(now, dst)
}

// Tick runs the timer that is due at now and appends its output to dst: an
// OutputExpired one burst after the last announced block, when nothing
// followed the Pre_CSBKs. A Tick before Deadline does nothing. It allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Tick(now time.Duration, dst []Output) []Output {
	if p == nil {
		return dst
	}
	if p.state == PreamblesAnnounced && now >= p.until+BurstInterval {
		dst = p.appendAt(dst, OutputExpired)
		p.state = PreamblesIdle
	}
	return dst
}

// Deadline returns the time at which the Preambles needs a Tick. ok is
// false when no transfer is announced.
func (p *Preambles) Deadline() (at time.Duration, ok bool) {
	if p == nil {
		return 0, false
	}
	if p.state != PreamblesAnnounced {
		return 0, false
	}
	return p.until + BurstInterval, true
}
