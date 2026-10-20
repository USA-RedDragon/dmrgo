package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
// a data message. Preambles reports the first Pre_CSBK, the CSBK, MBC header
// or data header that follows, joined with the addresses of the Pre_CSBKs,
// or that nothing followed by the end of the announced blocks. A repeater
// may send more Pre_CSBKs than the radio did, each with CSBK Blocks to
// Follow 1; each one moves the end. Make it with NewPreambles. A Preambles
// is not safe for concurrent use and must not be copied; go vet reports a
// copy. (ETSI TS 102 361-2 clauses 7.1.2.5 and 7.2.7)
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
// PreamblesConfig. It allocates the Preambles, and its Burst unless
// cfg.Scratch is set.
func NewPreambles(cfg *PreamblesConfig) *Preambles {
	if cfg == nil {
		cfg = &PreamblesConfig{}
	}
	return &Preambles{d: newDecoder(cfg.Scratch)}
}

// Reset forgets the announced transfer without an output.
func (p *Preambles) Reset() {
	p.state = PreamblesIdle
}

// State returns the state of the Preambles.
func (p *Preambles) State() PreamblesState {
	return p.state
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Preambles as Decoded does, and appends the outputs to dst.
// It allocates only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	return p.Decoded(now, p.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil, and appends the
// outputs to dst: OutputAnnounced for the first Pre_CSBK of a transfer,
// OutputJoined for the CSBK, MBC header or data header after it, and
// OutputExpired for a transfer that a Pre_CSBK with other addresses
// replaces. A CSBK whose CRC fails is dropped. It allocates only when dst
// has no room for MaxOutputs outputs.
func (p *Preambles) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	dst = p.Tick(now, dst)
	kind, c, octets := classify(b)
	switch kind {
	case rxCSBK:
		if c.FID == enums.FIDStandardized && c.PreamblePDU != nil {
			return p.preamble(now, c.PreamblePDU, dst)
		}
		return p.join(dst, false, &octets)
	case rxMBCHeader:
		return p.join(dst, false, nil)
	case rxDataHeader:
		return p.join(dst, true, nil)
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

func (p *Preambles) join(out []Output, data bool, c *pdu.CSBKBytes) []Output {
	if p.state != PreamblesAnnounced {
		return out
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
	o.Target, o.Source, o.Group, o.Data, o.Until = p.target, p.source, p.group, p.data, p.until
	return out
}

// Lost tells the Preambles that the transport lost the signal. It runs the
// timers as Tick does; the announced transfer can still come. It allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Lost(now time.Duration, dst []Output) []Output {
	return p.Tick(now, dst)
}

// Tick runs the timer that is due at now and appends its output to dst: an
// OutputExpired one burst after the last announced block, when nothing
// followed the Pre_CSBKs. A Tick before Deadline does nothing. It allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Preambles) Tick(now time.Duration, dst []Output) []Output {
	if p.state == PreamblesAnnounced && now >= p.until+BurstInterval {
		dst = p.appendAt(dst, OutputExpired)
		p.state = PreamblesIdle
	}
	return dst
}

// Deadline returns the time at which the Preambles needs a Tick. ok is
// false when no transfer is announced.
func (p *Preambles) Deadline() (at time.Duration, ok bool) {
	if p.state != PreamblesAnnounced {
		return 0, false
	}
	return p.until + BurstInterval, true
}
