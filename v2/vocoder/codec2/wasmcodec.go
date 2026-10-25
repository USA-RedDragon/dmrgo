//go:build !nocodec2 && !tinygo && !codec2native

package codec2

import (
	_ "embed"
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Backend is the name of the backend of this build: "native", "wasm" or
// "stub".
const Backend = "wasm"

//go:embed wasm/codec2.wasm
var embeddedWASM []byte

var (
	errCreate    error = &wrapError{"codec2: codec2_create failed", ErrUnavailable}
	errFrameSize error = &wrapError{"codec2: libcodec2 has another frame size for the mode", ErrUnavailable}
	errExports   error = &wrapError{"codec2: the WebAssembly module does not export the libcodec2 functions", ErrUnavailable}
	errMalloc    error = &wrapError{"codec2: malloc in the WebAssembly module failed", ErrUnavailable}
	errTrap      error = &wrapError{"codec2: libcodec2 trapped in the WebAssembly module", ErrUnavailable}
)

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}

type causeError struct {
	msg  string
	errs []error
}

func newCauseError(msg string, cause error) *causeError {
	return &causeError{msg: msg + ": " + cause.Error(), errs: []error{ErrUnavailable, cause}}
}

func (e *causeError) Error() string {
	return e.msg
}

func (e *causeError) Unwrap() []error {
	return e.errs
}

type export int

const (
	fnMalloc export = iota
	fnFree
	fnCreate
	fnDestroy
	fnEncode
	fnDecode
	fnSamplesPerFrame
	fnBitsPerFrame
	exportCount
)

func (fn export) spec() (name string, params, results int) {
	switch fn {
	case fnMalloc:
		return "malloc", 1, 1
	case fnFree:
		return "free", 1, 0
	case fnCreate:
		return "codec2_create", 1, 1
	case fnDestroy:
		return "codec2_destroy", 1, 0
	case fnEncode:
		return "codec2_encode", 3, 0
	case fnDecode:
		return "codec2_decode", 3, 0
	case fnSamplesPerFrame:
		return "codec2_samples_per_frame", 1, 1
	case fnBitsPerFrame:
		return "codec2_bits_per_frame", 1, 1
	case exportCount:
	}
	return "", 0, 0
}

func (fn export) name() string {
	n, _, _ := fn.spec()
	return n
}

var embedded = sync.OnceValues(func() (*Module, error) {
	c, err := compileEmbedded()
	if err != nil {
		return nil, err
	}
	return &Module{compiled: c}, nil
})

// Module is a compiled WebAssembly build of libcodec2 for Config.WASM. A
// Module is safe for concurrent use, and its Codecs keep working after
// Close.
type Module struct {
	mu       sync.Mutex
	compiled *engineModule
}

// NewModule compiles a WebAssembly build of libcodec2, such as one that
// wasm/build.sh makes from a changed libcodec2, for Config.WASM. The module
// must follow the interface in the package documentation. It returns an
// error that wraps ErrUnavailable when wasm is not a valid module or lacks a
// function of that interface, and ErrNotWASM in a build without the
// WebAssembly backend. It allocates the Module and the compiled code.
//
// wazero runs the module, also on platforms where the embedded module runs
// as translated Go code. wazero compiles it to machine code on arm64 and on
// amd64 with SSE4.1, under Linux, macOS, Windows, FreeBSD and NetBSD, and
// amd64 also under DragonFly BSD, Solaris and illumos. Elsewhere, such as on
// 386 and arm, wazero interprets it and allocates for each call into the
// module, so every Encode and Decode of a Codec from the Module allocates,
// about 1,400 to 1,900 times per frame.
func NewModule(wasm []byte) (*Module, error) {
	c, err := compile(wasm)
	if err != nil {
		return nil, err
	}
	return &Module{compiled: c}, nil
}

// Close frees the compiled code once no Codec uses it. Codecs made from m
// keep working, and New with m returns ErrClosed. Close is idempotent. A
// Close that waits for a New of another goroutine with m can allocate once,
// for the runtime record of the waiting goroutine.
func (m *Module) Close() error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compiled == nil {
		return nil
	}
	err := m.compiled.close()
	m.compiled = nil
	return err
}

func (m *Module) instantiate() (*engineInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compiled == nil {
		return nil, ErrClosed
	}
	return m.compiled.instantiate()
}

// Codec is a Codec2 vocoder.Codec that runs libcodec2 compiled to
// WebAssembly, with separate encoder and decoder states in a module
// instance of its own. A Codec is not safe for concurrent use; separate
// Codecs may be used from separate goroutines, and run in parallel. Close
// frees the instance, and so does the garbage collector. Encoding is
// deterministic, and so is decoding: the random number generator of
// libcodec2 belongs to the instance, so the decoded PCM depends only on the
// frames that the Codec decoded before.
type Codec struct {
	inst    *engineInstance
	enc     uint32
	dec     [2]uint32
	pcm     uint32
	bits    uint32
	mode    Mode
	fid     enums.FID
	nbytes  int
	silence [9]byte
	frame   [72 / 8]byte
	buf     [2 * FrameSamples]byte
	det     detector
	cleanup runtime.Cleanup
}

func closeInstance(inst *engineInstance) {
	inst.close()
}

func (m Mode) index() int {
	if m == Mode3200 {
		return 1
	}
	return 0
}

func (m Mode) wasmMode() uint32 {
	if m == Mode3200 {
		return 0
	}
	return 1
}

// New creates a Codec2 codec with the given configuration in a new instance
// of the WebAssembly module, cfg.WASM or the embedded one. It returns
// ErrUnsupportedMode for a Mode other than ModeAuto, Mode2400 or Mode3200,
// ErrClosed for a closed cfg.WASM, and an error that wraps ErrUnavailable
// when the module cannot be compiled or instantiated or libcodec2 cannot
// create a codec state. It allocates the Codec and the instance, and the
// first New of the embedded module compiles it. A nil cfg means the zero
// Config.
func New(p *Config) (*Codec, error) {
	var cfg Config
	if p != nil {
		cfg = *p
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	m := cfg.WASM
	if m == nil {
		if m, err = embedded(); err != nil {
			return nil, err
		}
	}
	inst, err := m.instantiate()
	if err != nil {
		return nil, err
	}
	encMode := cfg.Mode
	if encMode == ModeAuto {
		encMode = Mode2400
	}
	c := &Codec{
		inst:   inst,
		mode:   cfg.Mode,
		fid:    cfg.FID,
		nbytes: encMode.frameBytes(),
	}
	if err := c.setup(cfg.Mode, encMode); err != nil {
		closeInstance(inst)
		return nil, err
	}
	c.cleanup = runtime.AddCleanup(c, closeInstance, inst)
	return c, nil
}

func (c *Codec) call1(fn export, a uint32) (uint32, error) {
	r, err := c.inst.call1(fn, a)
	if err != nil {
		return 0, newCauseError("codec2: "+fn.name(), err)
	}
	return r, nil
}

func (c *Codec) create(m Mode) (uint32, error) {
	st, err := c.call1(fnCreate, m.wasmMode())
	if err != nil {
		return 0, err
	}
	if st == 0 {
		return 0, errCreate
	}
	spf, err := c.call1(fnSamplesPerFrame, st)
	if err != nil {
		return 0, err
	}
	bpf, err := c.call1(fnBitsPerFrame, st)
	if err != nil {
		return 0, err
	}
	if spf != FrameSamples || int(bpf) != m.frameBytes()*8 {
		return 0, errFrameSize
	}
	return st, nil
}

func (c *Codec) setup(mode, encMode Mode) error {
	buf, err := c.call1(fnMalloc, 2*FrameSamples+8)
	if err != nil {
		return err
	}
	if buf == 0 {
		return errMalloc
	}
	c.pcm, c.bits = buf, buf+2*FrameSamples

	tmp, err := c.create(encMode)
	if err != nil {
		return err
	}
	if c.enc, err = c.create(encMode); err != nil {
		return err
	}
	for _, m := range [...]Mode{Mode2400, Mode3200} {
		if mode == ModeAuto || mode == m {
			if c.dec[m.index()], err = c.create(m); err != nil {
				return err
			}
		}
	}

	if !c.inst.write(c.pcm, c.buf[:]) {
		return errMalloc
	}
	if err := c.inst.call3(fnEncode, tmp, c.bits, c.pcm); err != nil {
		return newCauseError("codec2: "+fnEncode.name(), err)
	}
	if !c.inst.read(c.bits, c.silence[:c.nbytes]) {
		return errMalloc
	}
	_, err = c.call1(fnDestroy, tmp)
	return err
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
func (c *Codec) FID() enums.FID {
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
// Mode2400 for ModeAuto. It returns an error that wraps ErrUnavailable when
// libcodec2 traps. It does not allocate when pcm has FrameSamples samples,
// except where wazero interprets the module (a NewModule module on a
// platform where wazero has no compiler, and the embedded module on an amd64
// CPU without SSE4.1 in a build without -tags codec2aot), and under
// GOOS=js, where each call into the module allocates JavaScript values.
func (c *Codec) Encode(pcm []int16) ([9]byte, error) {
	if c == nil {
		return [9]byte{}, dmrgo.ErrNilReceiver
	}

	if c.inst == nil {
		return [9]byte{}, ErrClosed
	}
	if len(pcm) != FrameSamples {
		return [9]byte{}, ErrFrameLength
	}
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(c.buf[2*i:], uint16(s))
	}
	if !c.inst.write(c.pcm, c.buf[:]) {
		return [9]byte{}, errTrap
	}
	if c.inst.call3(fnEncode, c.enc, c.bits, c.pcm) != nil {
		return [9]byte{}, errTrap
	}
	if !c.inst.read(c.bits, c.buf[:c.nbytes]) {
		return [9]byte{}, errTrap
	}
	var slot [9]byte
	copy(slot[:], c.buf[:c.nbytes])
	return slot, nil
}

// Decode decodes one 72-bit frame, appends its FrameSamples samples to dst
// and returns the extended slice and a report on the frame. The bits after
// the Codec2 frame are ignored, except that ModeAuto detects the mode from
// them. The DMR framing of Codec2 has no FEC, so the report has
// CountsKnown and no corrected bits, and Output is always OutputVoice;
// Frame is FrameInvalid when a bit after the Codec2 frame of the decoded
// mode is set, else FrameGood. It returns ErrClosed after Close, with dst
// unchanged, and an error that wraps ErrUnavailable when libcodec2 traps,
// with dst extended by FrameSamples zero samples, both with a zero report. It does not allocate when dst has room for FrameSamples
// more samples, except where wazero interprets the module, as for Encode,
// and under GOOS=js, where each call into the module allocates JavaScript
// values.
func (c *Codec) Decode(dst []int16, slot [9]byte) ([]int16, vocoder.FrameReport, error) {
	if c == nil {
		return dst, vocoder.FrameReport{}, dmrgo.ErrNilReceiver
	}

	if c.inst == nil {
		return dst, vocoder.FrameReport{}, ErrClosed
	}
	c.frame = slot
	m := c.mode
	if m == ModeAuto {
		m = c.det.add(&c.frame)
	}
	n := len(dst)
	dst = append(dst, make([]int16, FrameSamples)...)
	pcm := dst[n:]
	if !c.inst.write(c.bits, c.frame[:8]) {
		return dst, vocoder.FrameReport{}, errTrap
	}
	if c.inst.call3(fnDecode, c.dec[m.index()], c.pcm, c.bits) != nil {
		return dst, vocoder.FrameReport{}, errTrap
	}
	if !c.inst.read(c.pcm, c.buf[:]) {
		return dst, vocoder.FrameReport{}, errTrap
	}
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(c.buf[2*i:]))
	}
	return dst, m.report(&slot), nil
}

// DecodeLost appends FrameSamples zero samples in place of a frame that
// never arrived, and returns the extended slice and a report with Frame
// FrameLost and Output OutputSilence. libcodec2 has no concealment for
// Mode2400 and Mode3200, so the decoder state does not change. It returns
// ErrClosed after Close, with dst unchanged. It does not allocate when dst
// has room for FrameSamples more samples.
func (c *Codec) DecodeLost(dst []int16) ([]int16, vocoder.FrameReport, error) {
	if c == nil {
		return dst, vocoder.FrameReport{}, dmrgo.ErrNilReceiver
	}

	if c.inst == nil {
		return dst, vocoder.FrameReport{}, ErrClosed
	}
	return append(dst, make([]int16, FrameSamples)...), vocoder.FrameReport{Frame: vocoder.FrameLost, Output: vocoder.OutputSilence}, nil
}

// Close frees the module instance and its libcodec2 states. Later Encode,
// Decode and DecodeLost calls return ErrClosed. Close is idempotent.
func (c *Codec) Close() error {
	if c == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.inst == nil {
		return nil
	}
	c.cleanup.Stop()
	closeInstance(c.inst)
	c.inst = nil
	return nil
}
