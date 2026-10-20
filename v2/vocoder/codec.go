package vocoder

import (
	"errors"
	"io"
	"slices"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ErrUnknownCodec is returned by New when no codec is registered under the name.
var ErrUnknownCodec = errors.New("vocoder: unknown codec")

// ErrDuplicateCodec is returned by Register when a codec is already
// registered under the name.
var ErrDuplicateCodec = errors.New("vocoder: codec already registered")

// ErrNilFactory is returned by Register when the factory is nil.
var ErrNilFactory = errors.New("vocoder: codec factory is nil")

// ErrRegistryFull is returned by Register when MaxCodecs codecs are
// registered.
var ErrRegistryFull = errors.New("vocoder: codec registry is full")

// Codec converts between 8 kHz 16-bit mono PCM and the 72 raw on-air bits of
// one of the three vocoder frames of a DMR voice burst, packed, the first bit
// in the most significant bit of byte 0, as layer2.RawBurst.VoiceSlots
// returns them. The implementation applies any FEC, scrambling or
// interleaving that the vocoder needs. A Codec is not safe for concurrent use
// unless its documentation says so. Close releases its resources.
// (ETSI TS 102 361-1 clause 6.1)
type Codec interface {
	io.Closer

	// FrameSamples returns the number of PCM samples in one frame.
	FrameSamples() int
	// Encode encodes exactly FrameSamples samples into one 72-bit frame.
	Encode(pcm []int16) ([9]byte, error)
	// Decode decodes one 72-bit frame into FrameSamples samples. It returns
	// at most FrameSamples() samples.
	Decode(slot [9]byte) ([]int16, error)
	// Silence returns a frame that decodes to silence, for padding the last
	// burst of a call.
	Silence() [9]byte
	// FID returns the Feature set ID to send in the Full LC of calls that use
	// this codec. The FID does not identify the vocoder, so a receiver selects
	// the codec by a name that both ends agree on out of band (ETSI TS 102
	// 361-1 Table 9.21 NOTE).
	FID() enums.FeatureSetID
}

// AppendDecoder is a Codec that can decode into a buffer of the caller.
// Match it with a type assertion on a Codec.
type AppendDecoder interface {
	Codec

	// AppendDecode decodes one 72-bit frame as Decode does, appends the
	// FrameSamples samples to dst and returns the extended slice; dst needs
	// room for FrameSamples() samples. On error it returns dst unchanged.
	AppendDecode(dst []int16, slot [9]byte) ([]int16, error)
}

// MaxCodecs is the number of codec factories that Register can hold.
const MaxCodecs = 8

type registry struct {
	mu        sync.RWMutex
	n         int
	names     [MaxCodecs]string
	factories [MaxCodecs]func() (Codec, error)
}

var codecs registry

func (r *registry) register(name string, factory func() (Codec, error)) error {
	if factory == nil {
		return ErrNilFactory
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.Contains(r.names[:r.n], name) {
		return ErrDuplicateCodec
	}
	if r.n == MaxCodecs {
		return ErrRegistryFull
	}
	r.names[r.n], r.factories[r.n] = name, factory
	r.n++
	return nil
}

func (r *registry) factory(name string) func() (Codec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if i := slices.Index(r.names[:r.n], name); i >= 0 {
		return r.factories[i]
	}
	return nil
}

func (r *registry) len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.n
}

func (r *registry) appendNames(dst []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(dst)
	dst = append(dst, r.names[:r.n]...)
	slices.Sort(dst[n:])
	return dst
}

// Register makes a codec factory available under name. A program calls it
// once per codec at startup; no codec package registers itself. The registry
// holds MaxCodecs factories in fixed storage, so Register does not allocate.
// Register returns ErrNilFactory if factory is nil, ErrDuplicateCodec if name
// is already registered and ErrRegistryFull if MaxCodecs codecs are.
func Register(name string, factory func() (Codec, error)) error {
	return codecs.register(name, factory)
}

// New creates a codec with the factory registered under name. It returns
// ErrUnknownCodec when no codec is registered under name. The codec is what
// the factory returns, so New allocates what the factory allocates.
func New(name string) (Codec, error) {
	factory := codecs.factory(name)
	if factory == nil {
		return nil, ErrUnknownCodec
	}
	return factory()
}

// Names returns the sorted names of the registered codecs. It allocates the
// slice; AppendNames does not.
func Names() []string {
	return AppendNames(make([]string, 0, codecs.len()))
}

// AppendNames appends the sorted names of the registered codecs to dst and
// returns the extended slice. It allocates only when dst has no room for
// them.
func AppendNames(dst []string) []string {
	return codecs.appendNames(dst)
}
