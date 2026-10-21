package framer

import (
	"errors"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	ibits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

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
	// MaxPeriod is the largest Config.Period.
	MaxPeriod = 1 << 16
)

// ErrPeriod is returned by New for a Config.Period over MaxPeriod. Match it
// with errors.Is.
var ErrPeriod = errors.New("framer: Period is over MaxPeriod")

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
	// Offset is the bit index of the first SYNC bit.
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

// Search returns the first SYNC field in the first nbits bits of stream that
// starts at or after bit start and is within threshold bits of a SYNC
// pattern. stream holds the bits packed, the first bit in the most
// significant bit of stream[0], and start and Match.Offset are bit indexes.
// The bits of stream after the first nbits are ignored, and nbits is
// clamped to [0, 8*len(stream)]. A negative threshold means
// layer2.DefaultSyncThreshold. ok is false when there is none.
func Search(stream []byte, nbits, start, threshold int) (m Match, ok bool) {
	return search(stream, clampBits(stream, nbits), start, threshold)
}

func clampBits(stream []byte, nbits int) int {
	return max(0, min(nbits, len(stream)*8))
}

func search(stream []byte, n, start, threshold int) (m Match, ok bool) {
	if threshold < 0 {
		threshold = layer2.DefaultSyncThreshold
	}
	start = max(start, 0)
	if start > n-SyncBits {
		return Match{}, false
	}
	const mask = 1<<SyncBits - 1
	w := ibits.Field64(stream, start, SyncBits) >> 1
	for i := start; i+SyncBits <= n; i++ {
		w = (w<<1 | uint64(ibits.Bit(stream, i+SyncBits-1))) & mask
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
	// Threshold is the largest SYNC distance accepted. 0 accepts only an
	// exact SYNC, and a negative value means layer2.DefaultSyncThreshold.
	Threshold int
	// Period is the number of bits from one burst to the next while
	// locked. 0 means OutboundPeriod, a Period under layer2.BurstBits
	// means layer2.BurstBits, and at most MaxPeriod. Use SlotPeriod for the bursts of a single
	// TDMA channel, such as one inbound or direct mode channel.
	Period int
	// CACH decodes the 24 bits before each burst as a CACH, for a BS
	// outbound stream.
	CACH bool
	// MaxMisses is the number of consecutive bursts without a SYNC that
	// keep the lock. 0 means DefaultMaxMisses.
	MaxMisses int
}

// Framer cuts bursts out of a bit stream written to it in pieces of any
// number of bits, as the package doc describes. Make one with New: a zero
// Framer has no buffer and drops every bit. A Framer is not safe for
// concurrent use.
type Framer struct {
	cfg     Config
	pre     int
	buf     []byte
	n       int
	base    int64
	locked  bool
	next    int
	misses  int
	search  int
	channel int
}

// New returns a Framer for cfg. A nil cfg means the zero Config. It
// returns ErrPeriod for a Period over MaxPeriod. It allocates the stream
// buffer, which holds at most a CACH and a burst between writes, with room
// for one more Period of bits.
func New(c *Config) (*Framer, error) {
	var cfg Config
	if c != nil {
		cfg = *c
	}
	if cfg.Period > MaxPeriod {
		return nil, ErrPeriod
	}
	if cfg.Threshold < 0 {
		cfg.Threshold = layer2.DefaultSyncThreshold
	}
	switch {
	case cfg.Period <= 0:
		cfg.Period = OutboundPeriod
	case cfg.Period < layer2.BurstBits:
		cfg.Period = layer2.BurstBits
	}
	if cfg.MaxMisses <= 0 {
		cfg.MaxMisses = DefaultMaxMisses
	}
	f := &Framer{cfg: cfg}
	if cfg.CACH {
		f.pre = layer2.CACHBits
	}
	f.search = f.pre + SyncOffset
	f.buf = make([]byte, 0, (f.pre+layer2.BurstBits+cfg.Period)/8+2)
	return f, nil
}

// Locked reports whether the Framer is locked on a burst period.
func (f *Framer) Locked() bool {
	if f == nil {
		return false
	}
	return f.locked
}

// Write adds the first nbits bits of stream, packed, the first bit in the
// most significant bit of stream[0], and returns the bursts they complete,
// or nil when they complete none. The next write continues at the bit after
// them, so a piece can end inside a byte. The bits of stream after the
// first nbits are ignored, and nbits is clamped to [0, 8*len(stream)]. It
// returns at most 2*(min(nbits,8*len(stream))/layer2.BurstBits+1)
// bursts: the bursts of one call end at new bits, and only a burst found
// after the lock is lost can end less than layer2.BurstBits after the burst
// before it, so of two gaps in a row at least one is layer2.BurstBits or
// more. It allocates the slice; AppendBursts does not.
func (f *Framer) Write(stream []byte, nbits int) []Burst {
	return f.AppendBursts(nil, stream, nbits)
}

// AppendBursts adds the first nbits bits of stream as Write does, with
// nbits clamped to [0, 8*len(stream)], appends the bursts they complete to
// dst and returns the extended slice. It allocates only when dst has no
// room for 2*(min(nbits,8*len(stream))/layer2.BurstBits+1) bursts.
func (f *Framer) AppendBursts(dst []Burst, stream []byte, nbits int) []Burst {
	if f == nil {
		return dst
	}
	nbits = clampBits(stream, nbits)
	for from := 0; from < nbits && f.n < cap(f.buf)*8; {
		k := min(nbits-from, cap(f.buf)*8-f.n)
		dst = f.appendPiece(dst, stream, from, k)
		from += k
	}
	return dst
}

func (f *Framer) appendPiece(dst []Burst, stream []byte, from, k int) []Burst {
	f.buf = f.buf[:(f.n+k+7)/8]
	if f.n&7 == 0 && from&7 == 0 {
		copy(f.buf[f.n>>3:], stream[from>>3:(from+k+7)>>3])
	} else {
		ibits.Copy(f.buf, f.n, stream, from, k)
	}
	f.n += k
	n := f.n
	for {
		if !f.locked {
			m, ok := search(f.buf, n, f.search, f.cfg.Threshold)
			if !ok {
				f.search = max(f.search, n-SyncBits+1)
				break
			}
			f.locked, f.next, f.misses, f.channel = true, m.Offset-SyncOffset, 0, 0
		}
		if f.next+layer2.BurstBits > n {
			break
		}
		dst = append(dst, f.cut())
	}
	f.trim()
	return dst
}

func (f *Framer) cut() Burst {
	start := f.next
	b := Burst{Offset: f.base + int64(start)}
	ibits.Copy(b.Data[:], 0, f.buf, start, layer2.BurstBits)
	b.Sync, b.Distance = nearest(ibits.Field64(f.buf, start+SyncOffset, SyncBits))
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
		var c [3]byte
		ibits.Copy(c[:], 0, f.buf, start-layer2.CACHBits, layer2.CACHBits)
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
	drop := min(keep, f.n) >> 3
	f.buf = f.buf[:copy(f.buf, f.buf[drop:])]
	f.n -= drop * 8
	f.base += int64(drop) * 8
	f.next -= drop * 8
	f.search -= drop * 8
}
