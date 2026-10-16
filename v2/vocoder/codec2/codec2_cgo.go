//go:build cgo && !nocodec2 && !tinygo

package codec2

/*
#cgo pkg-config: codec2
#include <codec2.h>
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// libMu serializes libcodec2 calls. The decoder draws from a process-wide
// random number generator in libcodec2 that has no locking.

var libMu sync.Mutex

// The errors of New for a failure of libcodec2.
var (
	// errCreate is returned by New when libcodec2 cannot create a codec
	// state.
	errCreate = errors.New("codec2: codec2_create failed")
	// errFrameSize is returned by New when libcodec2 has another frame
	// size for the mode than the package expects.
	errFrameSize = errors.New("codec2: libcodec2 has another frame size for the mode")
)

// Codec is a Codec2 vocoder.Codec with separate libcodec2 encoder and decoder
// states. A Codec is not safe for concurrent use; separate Codecs may be used
// from separate goroutines. Close frees the states, and so does the garbage
// collector. Encoding is deterministic, but decoded PCM depends on earlier
// decodes in the process, from a process-wide random number generator.
type Codec struct {
	enc, dec *C.struct_CODEC2
	mode     Mode
	fid      enums.FeatureSetID
	nbytes   int
	silence  [9]byte
	frame    [72 / 8]byte
	cleanup  runtime.Cleanup
}

type states struct{ enc, dec *C.struct_CODEC2 }

func (s states) destroy() {
	libMu.Lock()
	defer libMu.Unlock()
	if s.enc != nil {
		C.codec2_destroy(s.enc)
	}
	if s.dec != nil {
		C.codec2_destroy(s.dec)
	}
}

// New creates a Codec2 codec with the given configuration. It returns
// ErrUnsupportedMode for a Mode other than zero, Mode2400 or Mode3200, and an
// error when libcodec2 cannot create a codec state. It allocates the Codec,
// and libcodec2 allocates its states.
func New(cfg Config) (*Codec, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	cmode := C.int(C.CODEC2_MODE_2400)
	if cfg.Mode == Mode3200 {
		cmode = C.CODEC2_MODE_3200
	}

	libMu.Lock()
	s := states{enc: C.codec2_create(cmode), dec: C.codec2_create(cmode)}
	var nsam, nbits int
	if s.enc != nil && s.dec != nil {
		nsam = int(C.codec2_samples_per_frame(s.enc))
		nbits = int(C.codec2_bits_per_frame(s.enc))
	}
	libMu.Unlock()
	if s.enc == nil || s.dec == nil {
		s.destroy()
		return nil, errCreate
	}
	if nsam != FrameSamples || nbits != cfg.Mode.frameBytes()*8 {
		s.destroy()
		return nil, errFrameSize
	}

	silence, err := newSilence(cmode, cfg.Mode.frameBytes())
	if err != nil {
		s.destroy()
		return nil, err
	}

	c := &Codec{
		enc:     s.enc,
		dec:     s.dec,
		mode:    cfg.Mode,
		fid:     cfg.FID,
		nbytes:  cfg.Mode.frameBytes(),
		silence: silence,
	}
	c.cleanup = runtime.AddCleanup(c, states.destroy, s)
	return c, nil
}

// newSilence returns 20 ms of silence, encoded by a fresh encoder of mode
// cmode into nbytes bytes.
func newSilence(cmode C.int, nbytes int) ([9]byte, error) {
	libMu.Lock()
	defer libMu.Unlock()
	st := C.codec2_create(cmode)
	if st == nil {
		return [9]byte{}, errCreate
	}
	defer C.codec2_destroy(st)
	var pcm [FrameSamples]C.short
	var slot [9]byte
	C.codec2_encode(st, (*C.uchar)(unsafe.Pointer(&slot[0])), &pcm[0])
	clear(slot[nbytes:])
	return slot, nil
}

// FrameSamples returns FrameSamples.
func (c *Codec) FrameSamples() int { return FrameSamples }

// Mode returns the configured Codec2 mode.
func (c *Codec) Mode() Mode { return c.mode }

// FID returns the configured Feature set ID.
func (c *Codec) FID() enums.FeatureSetID { return c.fid }

// Silence returns 20 ms of digital silence encoded with a fresh encoder
// state in the configured mode.
func (c *Codec) Silence() [9]byte { return c.silence }

// Encode encodes exactly FrameSamples samples into one 72-bit frame. It
// does not allocate when pcm has FrameSamples samples.
func (c *Codec) Encode(pcm []int16) ([9]byte, error) {
	if c.enc == nil {
		return [9]byte{}, ErrClosed
	}
	if len(pcm) != FrameSamples {
		return [9]byte{}, ErrFrameLength
	}
	frame := c.frame[:c.nbytes]
	libMu.Lock()
	C.codec2_encode(c.enc, (*C.uchar)(unsafe.Pointer(&frame[0])), (*C.short)(unsafe.Pointer(&pcm[0])))
	libMu.Unlock()
	runtime.KeepAlive(c)
	var slot [9]byte
	copy(slot[:], frame)
	return slot, nil
}

// Decode decodes one 72-bit frame into FrameSamples samples. The bits after
// the Codec2 frame are ignored. It allocates the samples; AppendDecode does
// not.
func (c *Codec) Decode(slot [9]byte) ([]int16, error) {
	if c.dec == nil {
		return nil, ErrClosed
	}
	return c.AppendDecode(make([]int16, 0, FrameSamples), slot)
}

// AppendDecode decodes one 72-bit frame as Decode does, appends the
// FrameSamples samples to dst and returns the extended slice. It does not
// allocate when dst has room for FrameSamples more samples. On error it
// returns dst unchanged.
func (c *Codec) AppendDecode(dst []int16, slot [9]byte) ([]int16, error) {
	if c.dec == nil {
		return dst, ErrClosed
	}
	frame := c.frame[:c.nbytes]
	copy(frame, slot[:])
	n := len(dst)
	dst = append(dst, make([]int16, FrameSamples)...)
	pcm := dst[n:]
	libMu.Lock()
	C.codec2_decode(c.dec, (*C.short)(unsafe.Pointer(&pcm[0])), (*C.uchar)(unsafe.Pointer(&frame[0])))
	libMu.Unlock()
	runtime.KeepAlive(c)
	return dst, nil
}

// Close frees the libcodec2 states. Later Encode and Decode calls return
// ErrClosed. Close is idempotent. A Close that waits for a libcodec2 call of
// another Codec can allocate once, for the runtime record of the waiting
// goroutine.
func (c *Codec) Close() error {
	if c.enc == nil {
		return nil
	}
	c.cleanup.Stop()
	states{enc: c.enc, dec: c.dec}.destroy()
	c.enc, c.dec = nil, nil
	return nil
}
