package vocoder

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ErrUnknownCodec is returned by New when no codec is registered under the name.
var ErrUnknownCodec = errors.New("vocoder: unknown codec")

// Codec converts between 8 kHz 16-bit mono PCM and the 72 raw on-air bits of
// one of the three vocoder frames of a DMR voice burst (ETSI TS 102 361-1
// clause 6.1). The implementation applies any FEC, scrambling or interleaving
// that the vocoder needs. A Codec is not safe for concurrent use unless its
// documentation says so. Close releases its resources.
type Codec interface {
	io.Closer

	// FrameSamples returns the number of PCM samples in one frame.
	FrameSamples() int
	// Encode encodes exactly FrameSamples samples into one 72-bit frame.
	Encode(pcm []int16) ([72]bit.Bit, error)
	// Decode decodes one 72-bit frame into FrameSamples samples.
	Decode(slot [72]bit.Bit) ([]int16, error)
	// Silence returns a frame that decodes to silence, for padding the last
	// burst of a call.
	Silence() [72]bit.Bit
	// FID returns the Feature set ID to send in the Full LC of calls that use
	// this codec. The FID does not identify the vocoder, so a receiver selects
	// the codec by a name that both ends agree on out of band (ETSI TS 102
	// 361-1 Table 9.21 NOTE).
	FID() enums.FeatureSetID
}

var (
	registryMu sync.RWMutex
	registry   = map[string]func() (Codec, error){}
)

// Register makes a codec factory available under name. It is meant to be
// called from an init function. Register panics if name is already
// registered or factory is nil.
func Register(name string, factory func() (Codec, error)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if factory == nil {
		panic("vocoder: Register factory is nil")
	}
	if _, dup := registry[name]; dup {
		panic("vocoder: Register called twice for codec " + name)
	}
	registry[name] = factory
}

// New creates a codec with the factory registered under name. It returns an
// error that wraps ErrUnknownCodec when no codec is registered under name.
func New(name string) (Codec, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownCodec, name)
	}
	return factory()
}

// Names returns the sorted names of the registered codecs.
func Names() []string {
	registryMu.RLock()
	n := len(registry)
	registryMu.RUnlock()
	return AppendNames(make([]string, 0, n))
}

// AppendNames appends the sorted names of the registered codecs to dst and
// returns the extended slice. It does not allocate when dst has room for
// them.
func AppendNames(dst []string) []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	n := len(dst)
	for name := range registry {
		dst = append(dst, name)
	}
	slices.Sort(dst[n:])
	return dst
}
