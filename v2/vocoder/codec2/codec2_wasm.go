//go:build !nocodec2 && !tinygo && !codec2native && (!cgo || codec2wasm)

package codec2

import (
	"context"
	_ "embed"
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
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
	msg   string
	cause error
}

func (e *causeError) Error() string {
	return e.msg + ": " + e.cause.Error()
}

func (e *causeError) Unwrap() []error {
	return []error{ErrUnavailable, e.cause}
}

type wasmRuntime struct {
	rt  wazero.Runtime
	err error
}

var (
	rtOnce   sync.Once
	rtShared wasmRuntime
	embOnce  sync.Once
	embMod   *Module
	embErr   error
)

func sharedRuntime() (wazero.Runtime, error) {
	rtOnce.Do(func() {
		ctx := context.Background()
		rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			_ = rt.Close(ctx)
			rtShared.err = &causeError{"codec2: WASI", err}
			return
		}
		rtShared.rt = rt
	})
	return rtShared.rt, rtShared.err
}

func embeddedModule() (*Module, error) {
	embOnce.Do(func() {
		embMod, embErr = NewModule(embeddedWASM)
	})
	return embMod, embErr
}

// Module is a compiled WebAssembly build of libcodec2 for Config.WASM. A
// Module is safe for concurrent use, and its Codecs keep working after
// Close.
type Module struct {
	mu       sync.Mutex
	compiled wazero.CompiledModule
}

var wantExports = [...]struct {
	name    string
	params  int
	results int
}{
	{"malloc", 1, 1},
	{"free", 1, 0},
	{"codec2_create", 1, 1},
	{"codec2_destroy", 1, 0},
	{"codec2_encode", 3, 0},
	{"codec2_decode", 3, 0},
	{"codec2_samples_per_frame", 1, 1},
	{"codec2_bits_per_frame", 1, 1},
}

// NewModule compiles a WebAssembly build of libcodec2, such as one that
// wasm/build.sh makes from a changed libcodec2, for Config.WASM. It returns
// an error that wraps ErrUnavailable when wasm is not a valid module or
// lacks a function of the libcodec2 API, and ErrNotWASM in a build without
// the WebAssembly backend. It allocates the Module and the compiled code.
func NewModule(wasm []byte) (*Module, error) {
	rt, err := sharedRuntime()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, &causeError{"codec2: compile the WebAssembly module", err}
	}
	if !hasExports(compiled) {
		_ = compiled.Close(ctx)
		return nil, errExports
	}
	return &Module{compiled: compiled}, nil
}

func hasExports(compiled wazero.CompiledModule) bool {
	if _, ok := compiled.ExportedMemories()["memory"]; !ok {
		return false
	}
	fns := compiled.ExportedFunctions()
	for _, want := range wantExports {
		def, ok := fns[want.name]
		if !ok || len(def.ParamTypes()) != want.params || len(def.ResultTypes()) != want.results {
			return false
		}
		for _, types := range [][]api.ValueType{def.ParamTypes(), def.ResultTypes()} {
			for _, t := range types {
				if t != api.ValueTypeI32 {
					return false
				}
			}
		}
	}
	return true
}

// Close frees the compiled code once no Codec uses it. Codecs made from m
// keep working, and New with m returns ErrClosed. Close is idempotent.
func (m *Module) Close() error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compiled == nil {
		return nil
	}
	err := m.compiled.Close(context.Background())
	m.compiled = nil
	return err
}

func (m *Module) instantiate() (api.Module, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compiled == nil {
		return nil, ErrClosed
	}
	rt, err := sharedRuntime()
	if err != nil {
		return nil, err
	}
	mod, err := rt.InstantiateModule(context.Background(), m.compiled,
		wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, &causeError{"codec2: instantiate the WebAssembly module", err}
	}
	return mod, nil
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
	mod     api.Module
	mem     api.Memory
	encode  api.Function
	decode  api.Function
	enc     uint64
	dec     [2]uint64
	pcm     uint32
	bits    uint32
	stack   [3]uint64
	mode    Mode
	fid     enums.FID
	nbytes  int
	silence [9]byte
	frame   [72 / 8]byte
	det     detector
	cleanup runtime.Cleanup
}

func closeInstance(mod api.Module) {
	_ = mod.Close(context.Background())
}

func (m Mode) index() int {
	if m == Mode3200 {
		return 1
	}
	return 0
}

func (m Mode) wasmMode() uint64 {
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
		if m, err = embeddedModule(); err != nil {
			return nil, err
		}
	}
	mod, err := m.instantiate()
	if err != nil {
		return nil, err
	}
	encMode := cfg.Mode
	if encMode == ModeAuto {
		encMode = Mode2400
	}
	c := &Codec{
		mod:    mod,
		mem:    mod.Memory(),
		encode: mod.ExportedFunction("codec2_encode"),
		decode: mod.ExportedFunction("codec2_decode"),
		mode:   cfg.Mode,
		fid:    cfg.FID,
		nbytes: encMode.frameBytes(),
	}
	if err := c.setup(cfg.Mode, encMode); err != nil {
		closeInstance(mod)
		return nil, err
	}
	c.cleanup = runtime.AddCleanup(c, closeInstance, mod)
	return c, nil
}

func (c *Codec) call(name string, params ...uint64) (uint64, error) {
	res, err := c.mod.ExportedFunction(name).Call(context.Background(), params...)
	if err != nil {
		return 0, &causeError{"codec2: " + name, err}
	}
	if len(res) == 0 {
		return 0, nil
	}
	return res[0], nil
}

func (c *Codec) create(m Mode) (uint64, error) {
	st, err := c.call("codec2_create", m.wasmMode())
	if err != nil {
		return 0, err
	}
	if uint32(st) == 0 {
		return 0, errCreate
	}
	spf, err := c.call("codec2_samples_per_frame", st)
	if err != nil {
		return 0, err
	}
	bpf, err := c.call("codec2_bits_per_frame", st)
	if err != nil {
		return 0, err
	}
	if int32(spf) != FrameSamples || int32(bpf) != int32(m.frameBytes()*8) {
		return 0, errFrameSize
	}
	return st, nil
}

func (c *Codec) setup(mode, encMode Mode) error {
	buf, err := c.call("malloc", 2*FrameSamples+8)
	if err != nil {
		return err
	}
	if uint32(buf) == 0 {
		return errMalloc
	}
	c.pcm, c.bits = uint32(buf), uint32(buf)+2*FrameSamples

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

	if !c.mem.Write(c.pcm, make([]byte, 2*FrameSamples)) {
		return errMalloc
	}
	if _, err := c.call("codec2_encode", tmp, uint64(c.bits), uint64(c.pcm)); err != nil {
		return err
	}
	frame, ok := c.mem.Read(c.bits, uint32(c.nbytes))
	if !ok {
		return errMalloc
	}
	copy(c.silence[:], frame)
	_, err = c.call("codec2_destroy", tmp)
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
// libcodec2 traps. It does not allocate when pcm has FrameSamples samples.
func (c *Codec) Encode(pcm []int16) ([9]byte, error) {
	if c == nil {
		return [9]byte{}, dmrgo.ErrNilReceiver
	}

	if c.mod == nil {
		return [9]byte{}, ErrClosed
	}
	if len(pcm) != FrameSamples {
		return [9]byte{}, ErrFrameLength
	}
	in, ok := c.mem.Read(c.pcm, 2*FrameSamples)
	if !ok {
		return [9]byte{}, errTrap
	}
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(in[2*i:], uint16(s))
	}
	c.stack = [3]uint64{c.enc, uint64(c.bits), uint64(c.pcm)}
	if c.encode.CallWithStack(context.Background(), c.stack[:]) != nil {
		return [9]byte{}, errTrap
	}
	out, ok := c.mem.Read(c.bits, uint32(c.nbytes))
	if !ok {
		return [9]byte{}, errTrap
	}
	var slot [9]byte
	copy(slot[:], out)
	return slot, nil
}

// Decode decodes one 72-bit frame, appends its FrameSamples samples to dst
// and returns the extended slice. The bits after the Codec2 frame are
// ignored, except that ModeAuto detects the mode from them. It returns
// ErrClosed after Close, with dst unchanged, and an error that wraps
// ErrUnavailable when libcodec2 traps, with dst extended by FrameSamples
// zero samples. It does not allocate when dst has room for FrameSamples
// more samples.
func (c *Codec) Decode(dst []int16, slot [9]byte) ([]int16, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	if c.mod == nil {
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
	if !c.mem.Write(c.bits, c.frame[:8]) {
		return dst, errTrap
	}
	c.stack = [3]uint64{c.dec[m.index()], uint64(c.pcm), uint64(c.bits)}
	if c.decode.CallWithStack(context.Background(), c.stack[:]) != nil {
		return dst, errTrap
	}
	out, ok := c.mem.Read(c.pcm, 2*FrameSamples)
	if !ok {
		return dst, errTrap
	}
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(out[2*i:]))
	}
	return dst, nil
}

// Close frees the module instance and its libcodec2 states. Later Encode
// and Decode calls return ErrClosed. Close is idempotent.
func (c *Codec) Close() error {
	if c == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.mod == nil {
		return nil
	}
	c.cleanup.Stop()
	closeInstance(c.mod)
	c.mod, c.mem, c.encode, c.decode = nil, nil, nil, nil
	return nil
}
