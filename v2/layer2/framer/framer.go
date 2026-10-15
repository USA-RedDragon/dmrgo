// Package framer finds DMR bursts in a demodulated bit stream.
//
// A burst is 264 bits with the 48-bit SYNC or embedded signalling field at
// burst bits 108 to 155 (ETSI TS 102 361-1 §6.1, §9.1.1). In a BS outbound
// stream each burst follows a 24-bit CACH, so the bursts repeat every 288
// bits and the TC bit of the CACH numbers the TDMA channel of the burst
// that follows it (§6.3, Table 9.24). Search finds a SYNC at any bit
// offset; Framer locks on one and then cuts bursts at a fixed period.
package framer

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	// BurstBits is the length of a burst.
	BurstBits = 264
	// SyncOffset is the burst bit at which the SYNC field starts.
	SyncOffset = 108
	// SyncBits is the length of the SYNC field.
	SyncBits = 48
	// OutboundPeriod is the bit period of bursts in a BS outbound stream:
	// one CACH and one burst.
	OutboundPeriod = layer2.CACHBits + BurstBits
	// SlotPeriod is the bit period of the bursts of one TDMA channel.
	SlotPeriod = 2 * OutboundPeriod
	// DefaultMaxMisses is the default number of bursts without a SYNC that
	// a locked Framer emits before it loses lock.
	DefaultMaxMisses = 12
)

//nolint:gochecknoglobals
var syncPatterns = [...]enums.SyncPattern{
	enums.SyncBSSourcedVoice,
	enums.SyncBSSourcedData,
	enums.SyncMSSourcedVoice,
	enums.SyncMSSourcedData,
	enums.SyncMSSourcedRC,
	enums.SyncTDMA1Voice,
	enums.SyncTDMA1Data,
	enums.SyncTDMA2Voice,
	enums.SyncTDMA2Data,
	enums.SyncReserved,
}

// Match is a SYNC field found in a bit stream.
type Match struct {
	// Offset is the index of the first SYNC bit.
	Offset int
	// Pattern is the matched Table 9.2 pattern.
	Pattern enums.SyncPattern
	// Distance is the Hamming distance from Pattern.
	Distance int
}

func nearest(window uint64) (enums.SyncPattern, int) {
	best, bestDistance := enums.SyncEmbeddedSignalling, SyncBits+1
	for _, p := range syncPatterns {
		if d := bits.OnesCount64(window ^ uint64(p)); d < bestDistance { //nolint:gosec // patterns are 48-bit positive constants
			best, bestDistance = p, d
		}
	}
	return best, bestDistance
}

func window(stream []bit.Bit, offset int) uint64 {
	var w uint64
	for _, b := range stream[offset : offset+SyncBits] {
		w = w<<1 | uint64(b&1)
	}
	return w
}

// Search returns the first SYNC field in stream that starts at or after
// start and is within threshold bits of a Table 9.2 pattern. A negative
// threshold means enums.SyncMatchThreshold. ok is false when there is none.
func Search(stream []bit.Bit, start, threshold int) (m Match, ok bool) {
	if threshold < 0 {
		threshold = enums.SyncMatchThreshold
	}
	start = max(start, 0)
	if start+SyncBits > len(stream) {
		return Match{}, false
	}
	const mask = 1<<SyncBits - 1
	w := window(stream, start) >> 1
	for i := start; i+SyncBits <= len(stream); i++ {
		w = (w<<1 | uint64(stream[i+SyncBits-1]&1)) & mask
		if p, d := nearest(w); d <= threshold {
			return Match{Offset: i, Pattern: p, Distance: d}, true
		}
	}
	return Match{}, false
}

// Burst is a burst cut from the stream.
type Burst struct {
	// Data is the burst, ready for layer2.NewBurstFromBytes.
	Data [33]byte
	// Offset is the stream index of burst bit 0, counted from the first
	// bit written to the Framer.
	Offset int64
	// Sync is the Table 9.2 pattern of the SYNC field, or
	// enums.SyncEmbeddedSignalling when the field is not within the
	// threshold of one, as in voice bursts B to F.
	Sync enums.SyncPattern
	// Distance is the Hamming distance of the field from the nearest
	// pattern.
	Distance int
	// HasCACH is set when CACH holds the 24 bits before the burst, in
	// logical order (see layer2.CACHDeinterleave).
	HasCACH bool
	CACH    layer2.CACH
	// Channel is the TDMA channel, 1 or 2, or 0 when unknown. It comes
	// from the TC bit of the CACH, from a direct mode TDMA SYNC pattern,
	// or from the channel of the previous burst.
	Channel int
}

// Config configures a Framer.
type Config struct {
	// Threshold is the largest SYNC distance accepted. A negative value
	// means enums.SyncMatchThreshold.
	Threshold int
	// Period is the number of bits from one burst to the next while
	// locked. 0 means OutboundPeriod. Use SlotPeriod for the bursts of a
	// single TDMA channel, such as one inbound or direct mode channel.
	Period int
	// CACH decodes the 24 bits before each burst as a CACH, for a BS
	// outbound stream.
	CACH bool
	// MaxMisses is the number of consecutive bursts without a SYNC that
	// keep the lock. 0 means DefaultMaxMisses.
	MaxMisses int
}

// Framer cuts bursts out of a bit stream written to it in pieces of any
// size. It searches for a SYNC, emits the burst around it, and then emits
// a burst every Period bits whether or not it carries a SYNC (a flywheel),
// so that voice bursts B to F are emitted too. A SYNC found at its expected
// place restarts the miss count. After MaxMisses bursts without one the
// Framer searches again, from the bit after the last expected SYNC. A
// Framer is not safe for concurrent use.
type Framer struct {
	cfg     Config
	pre     int
	buf     []bit.Bit
	base    int64
	locked  bool
	next    int
	misses  int
	search  int
	channel int
}

// New returns a Framer with the given configuration.
func New(cfg Config) *Framer {
	if cfg.Threshold < 0 {
		cfg.Threshold = enums.SyncMatchThreshold
	}
	if cfg.Period <= 0 {
		cfg.Period = OutboundPeriod
	}
	if cfg.MaxMisses <= 0 {
		cfg.MaxMisses = DefaultMaxMisses
	}
	f := &Framer{cfg: cfg}
	if cfg.CACH {
		f.pre = layer2.CACHBits
	}
	f.search = f.pre + SyncOffset
	return f
}

// Locked reports whether the Framer is locked on a burst period.
func (f *Framer) Locked() bool {
	return f.locked
}

// Write adds bits to the stream and returns the bursts they complete.
func (f *Framer) Write(stream ...bit.Bit) []Burst {
	n := len(f.buf)
	f.buf = append(f.buf, stream...)
	bit.Mask(f.buf[n:])
	var out []Burst
	for {
		if !f.locked {
			m, ok := Search(f.buf, f.search, f.cfg.Threshold)
			if !ok {
				f.search = max(f.search, len(f.buf)-SyncBits+1)
				break
			}
			f.locked, f.next, f.misses, f.channel = true, m.Offset-SyncOffset, 0, 0
		}
		if f.next+BurstBits > len(f.buf) {
			break
		}
		out = append(out, f.cut())
	}
	f.trim()
	return out
}

func (f *Framer) cut() Burst {
	start := f.next
	var raw [BurstBits]bit.Bit
	copy(raw[:], f.buf[start:start+BurstBits])
	b := Burst{Offset: f.base + int64(start), Data: bit.PackBits264(raw)}
	b.Sync, b.Distance = nearest(window(f.buf, start+SyncOffset))
	if b.Distance > f.cfg.Threshold {
		b.Sync = enums.SyncEmbeddedSignalling
		f.misses++
	} else {
		f.misses = 0
	}
	switch {
	case f.channel == 0:
	case f.cfg.Period%SlotPeriod == 0:
	default:
		f.channel = 3 - f.channel
	}
	switch b.Sync {
	case enums.SyncTDMA1Voice, enums.SyncTDMA1Data:
		f.channel = 1
	case enums.SyncTDMA2Voice, enums.SyncTDMA2Data:
		f.channel = 2
	case enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice, enums.SyncMSSourcedData,
		enums.SyncMSSourcedRC, enums.SyncReserved, enums.SyncEmbeddedSignalling:
	}
	if f.cfg.CACH && start >= layer2.CACHBits {
		var c [layer2.CACHBits]bit.Bit
		copy(c[:], f.buf[start-layer2.CACHBits:start])
		b.CACH, b.HasCACH = layer2.DecodeCACH(layer2.CACHDeinterleave(c)), true
		if !b.CACH.FEC.Uncorrectable {
			f.channel = 1
			if b.CACH.TACT.TDMAChannel {
				f.channel = 2
			}
		}
	}
	b.Channel = f.channel
	f.next += f.cfg.Period
	if f.misses > f.cfg.MaxMisses {
		f.locked = false
		f.search = start + SyncOffset + 1
	}
	return b
}

func (f *Framer) trim() {
	keep := f.search - f.pre - SyncOffset
	if f.locked {
		keep = f.next - f.pre
	}
	if keep <= 0 {
		return
	}
	keep = min(keep, len(f.buf))
	f.buf = append(f.buf[:0], f.buf[keep:]...)
	f.base += int64(keep)
	f.next -= keep
	f.search -= keep
}
