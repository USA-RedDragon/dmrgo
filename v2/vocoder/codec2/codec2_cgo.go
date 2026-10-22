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

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

var libMu sync.Mutex

var (
	errCreate    = errors.New("codec2: codec2_create failed")
	errFrameSize = errors.New("codec2: libcodec2 has another frame size for the mode")
)

// Codec is a Codec2 vocoder.Codec with separate libcodec2 encoder and decoder
// states. A Codec is not safe for concurrent use; separate Codecs may be used
// from separate goroutines. Close frees the states, and so does the garbage
// collector. Encoding is deterministic, but decoded PCM depends on earlier
// decodes in the process, from a process-wide random number generator.
type Codec struct {
	enc     *C.struct_CODEC2
	dec     [2]*C.struct_CODEC2
	mode    Mode
	fid     enums.FeatureSetID
	nbytes  int
	silence [9]byte
	frame   [72 / 8]byte
	det     detector
	cleanup runtime.Cleanup
}

type states struct {
	enc *C.struct_CODEC2
	dec [2]*C.struct_CODEC2
}

func (s states) destroy() {
	libMu.Lock()
	defer libMu.Unlock()
	for _, st := range [...]*C.struct_CODEC2{s.enc, s.dec[0], s.dec[1]} {
		if st != nil {
			C.codec2_destroy(st)
		}
	}
}

func (m Mode) index() int {
	if m == Mode3200 {
		return 1
	}
	return 0
}

func (m Mode) cmode() C.int {
	if m == Mode3200 {
		return C.CODEC2_MODE_3200
	}
	return C.CODEC2_MODE_2400
}

// New creates a Codec2 codec with the given configuration. It returns
// ErrUnsupportedMode for a Mode other than ModeAuto, Mode2400 or Mode3200,
// and an error when libcodec2 cannot create a codec state. It allocates the
// Codec, and libcodec2 allocates its states: for ModeAuto a decoder state for
// each mode. A nil cfg means the zero Config.
func New(p *Config) (*Codec, error) {
	var cfg Config
	if p != nil {
		cfg = *p
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	encMode := cfg.Mode
	if encMode == ModeAuto {
		encMode = Mode2400
	}
	s, err := newStates(cfg.Mode, encMode)
	if err != nil {
		return nil, err
	}
	silence, err := newSilence(encMode.cmode(), encMode.frameBytes())
	if err != nil {
		s.destroy()
		return nil, err
	}

	c := &Codec{
		enc:     s.enc,
		dec:     s.dec,
		mode:    cfg.Mode,
		fid:     cfg.FID,
		nbytes:  encMode.frameBytes(),
		silence: silence,
	}
	c.cleanup = runtime.AddCleanup(c, states.destroy, s)
	return c, nil
}

func newStates(mode, encMode Mode) (states, error) {
	libMu.Lock()
	s := states{enc: C.codec2_create(encMode.cmode())}
	err := checkState(s.enc, encMode)
	for _, m := range [...]Mode{Mode2400, Mode3200} {
		if err == nil && (mode == ModeAuto || mode == m) {
			s.dec[m.index()] = C.codec2_create(m.cmode())
			err = checkState(s.dec[m.index()], m)
		}
	}
	libMu.Unlock()
	if err != nil {
		s.destroy()
		return states{}, err
	}
	return s, nil
}

func checkState(st *C.struct_CODEC2, m Mode) error {
	switch {
	case st == nil:
		return errCreate
	case int(C.codec2_samples_per_frame(st)) != FrameSamples || int(C.codec2_bits_per_frame(st)) != m.frameBytes()*8:
		return errFrameSize
	default:
		return nil
	}
}

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
func (c *Codec) FrameSamples() int {
	if c == nil {
		return 0
	}
	return FrameSamples
}

// Mode returns the configured Codec2 mode, ModeAuto for a Codec that detects
// the mode.
func (c *Codec) Mode() Mode {
	if c == nil {
		return 0
	}
	return c.mode
}

// DecodeMode returns the mode of the frames of the current call: the
// configured mode, or for ModeAuto the detected mode. It returns ModeAuto
// while the frames since New or Reset do not decide the mode; 5 clean frames
// decide it. Until then Decode uses the more likely mode, Mode2400 when no
// frame favors either.
func (c *Codec) DecodeMode() Mode {
	if c == nil {
		return 0
	}

	if c.mode != ModeAuto {
		return c.mode
	}
	return c.det.mode()
}

// Reset starts a new call: a ModeAuto Codec detects the mode again from the
// next frame. Without Reset, Decode follows a change of mode after at most 9
// frames. Reset does nothing for a fixed mode.
func (c *Codec) Reset() {
	if c == nil {
		return
	}
	c.det.reset()
}

// FID returns the configured Feature set ID.
func (c *Codec) FID() enums.FeatureSetID {
	if c == nil {
		return 0
	}
	return c.fid
}

// Silence returns 20 ms of digital silence encoded with a fresh encoder
// state in the mode that Encode uses.
func (c *Codec) Silence() [9]byte {
	if c == nil {
		return [9]byte{}
	}
	return c.silence
}

// Encode encodes exactly FrameSamples samples into one 72-bit frame, in
// Mode2400 for ModeAuto. It does not allocate when pcm has FrameSamples
// samples.
func (c *Codec) Encode(pcm []int16) ([9]byte, error) {
	if c == nil {
		return [9]byte{}, dmrgo.ErrNilReceiver
	}

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

// Decode decodes one 72-bit frame, appends its FrameSamples samples to dst
// and returns the extended slice. The bits after the Codec2 frame are
// ignored, except that ModeAuto detects the mode from them. It returns
// ErrClosed after Close, with dst unchanged. It does not allocate when dst
// has room for FrameSamples more samples.
func (c *Codec) Decode(dst []int16, slot [9]byte) ([]int16, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	if c.enc == nil {
		return dst, ErrClosed
	}
	c.frame = slot
	m := c.mode
	if m == ModeAuto {
		m = c.det.add(&c.frame)
	}
	n := len(dst)
	dst = append(dst, make([]int16, FrameSamples)...)
	pcm := dst[n:]
	libMu.Lock()
	C.codec2_decode(c.dec[m.index()], (*C.short)(unsafe.Pointer(&pcm[0])), (*C.uchar)(unsafe.Pointer(&c.frame[0])))
	libMu.Unlock()
	runtime.KeepAlive(c)
	return dst, nil
}

// Close frees the libcodec2 states. Later Encode and Decode calls return
// ErrClosed. Close is idempotent. A Close that waits for a libcodec2 call of
// another Codec can allocate once, for the runtime record of the waiting
// goroutine.
func (c *Codec) Close() error {
	if c == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.enc == nil {
		return nil
	}
	c.cleanup.Stop()
	states{enc: c.enc, dec: c.dec}.destroy()
	c.enc, c.dec = nil, [2]*C.struct_CODEC2{}
	return nil
}
