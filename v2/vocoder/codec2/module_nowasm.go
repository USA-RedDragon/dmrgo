//go:build nocodec2 || tinygo || codec2native || (cgo && !codec2wasm)

package codec2

// Module is a compiled WebAssembly build of libcodec2 for Config.WASM. This
// build has no WebAssembly backend, so NewModule always returns ErrNotWASM.
type Module struct{}

// NewModule returns ErrNotWASM because this build has no WebAssembly
// backend.
func NewModule([]byte) (*Module, error) {
	return nil, ErrNotWASM
}

// Close does nothing.
func (m *Module) Close() error {
	return nil
}
