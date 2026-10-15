// Package vocoder converts between PCM and the 72-bit vocoder slots of a DMR
// voice burst. It defines the Codec interface and a name-based codec
// registry, the AMBE+2 frame FEC (EncodeAMBE72, DecodeAMBE72) and the IPSC
// AMBE payload packing. It contains no AMBE+2 codec.
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

// Codec converts between 8 kHz 16-bit mono PCM and the 72 on-air bits of one
// vocoder frame in a DMR voice burst (ETSI TS 102 361-1 clause 6.1). A voice
// burst carries three such frames.
//
// The 72 bits are the raw on-air bits: any FEC, scrambling or interleaving the
// vocoder needs is the implementation's job.
//
// Implementations are not safe for concurrent use unless their documentation
// says otherwise. Close releases the resources of the codec; a codec that
// holds none returns nil.
//
// The method set is frozen for v2: a later feature is an optional interface
// that a caller detects with a type assertion, so existing codecs keep
// compiling. A decoder that can report frame quality, for example, would
// implement an additional QualityDecoder interface.
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
	// this codec. It is advisory: the FID does not identify the vocoder
	// (ETSI TS 102 361-1 Table 9.21 NOTE), so a receiver selects the codec by
	// name, agreed out of band, not by FID.
	FID() enums.FeatureSetID
}

//nolint:gochecknoglobals // codec registry, like database/sql drivers
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
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
