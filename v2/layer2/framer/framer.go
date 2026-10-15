package framer

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// BurstBits is the length of a burst.
//
// Deprecated: use layer2.BurstBits.
const BurstBits = layer2.BurstBits

const (
	// SyncOffset is the burst bit at which the SYNC field starts.
	SyncOffset = 108
	// SyncBits is the length of the SYNC field.
	SyncBits = 48
	// OutboundPeriod is the bit period of bursts in a BS outbound stream:
	// one CACH and one burst.
	OutboundPeriod = layer2.CACHBits + layer2.BurstBits
	// SlotPeriod is the bit period of the bursts of one TDMA channel.
	SlotPeriod = 2 * OutboundPeriod
	// DefaultMaxMisses is the default number of bursts without a SYNC that
	// a locked Framer emits before it loses lock.
	DefaultMaxMisses = 12
)

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
	// Pattern is the matched SYNC pattern.
	Pattern enums.SyncPattern
	// Distance is the Hamming distance from Pattern.
	Distance int
}

func nearest(window uint64) (enums.SyncPattern, int) {
	best, bestDistance := enums.SyncNone, SyncBits+1
	for _, p := range syncPatterns {
		if d := bits.OnesCount64(window ^ uint64(p)); d < bestDistance {
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
// start and is within threshold bits of a SYNC pattern. A negative
// threshold means layer2.DefaultSyncThreshold. ok is false when there is none.
func Search(stream []bit.Bit, start, threshold int) (m Match, ok bool) {
	if threshold < 0 {
		threshold = layer2.DefaultSyncThreshold
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
	Data layer2.RawBurst
	// Offset is the stream index of burst bit 0, counted from the first
	// bit written to the Framer.
	Offset int64
	// Sync is the SYNC pattern of the SYNC field, or enums.SyncNone when the
	// field is not within the threshold of one, as in voice bursts B to F.
	Sync enums.SyncPattern
	// Distance is the Hamming distance of the field from the nearest
	// pattern.
	Distance int
	// HasCACH is set when CACH holds the 24 bits before the burst, in
	// logical order (see layer2.CACHDeinterleave).
	HasCACH bool
	// CACH is the CACH decoded from the 24 bits before the burst when
	// HasCACH is set.
	CACH layer2.CACH
	// Channel is the TDMA channel, 1 or 2, or 0 when unknown. It comes
	// from the TC bit of the CACH, from a direct mode TDMA SYNC pattern,
	// or from the channel of the previous burst.
	Channel int
}

// Config configures a Framer.
type Config struct {
	// Threshold is the largest SYNC distance accepted. A negative value
	// means layer2.DefaultSyncThreshold.
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
// size, as the package doc describes. A Framer is not safe for concurrent
// use.
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
		cfg.Threshold = layer2.DefaultSyncThreshold
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
	lowbit.Mask(f.buf[n:])
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
		if f.next+layer2.BurstBits > len(f.buf) {
			break
		}
		out = append(out, f.cut())
	}
	f.trim()
	return out
}

func (f *Framer) cut() Burst {
	start := f.next
	var raw [layer2.BurstBits]bit.Bit
	copy(raw[:], f.buf[start:start+layer2.BurstBits])
	b := Burst{Offset: f.base + int64(start), Data: bit.Pack264(raw)}
	b.Sync, b.Distance = nearest(window(f.buf, start+SyncOffset))
	if b.Distance > f.cfg.Threshold {
		b.Sync = enums.SyncNone
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
		enums.SyncMSSourcedRC, enums.SyncReserved, enums.SyncNone:
	}
	if f.cfg.CACH && start >= layer2.CACHBits {
		var c [layer2.CACHBits]bit.Bit
		copy(c[:], f.buf[start-layer2.CACHBits:start])
		b.CACH, b.HasCACH = layer2.DecodeCACHField(c), true
		if !b.CACH.TACT.FEC.Uncorrectable {
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
