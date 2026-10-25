//go:build !nocodec2 && !tinygo && !codec2native && !(js && wasm)

package codec2

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2/internal/aot"
)

const (
	wasiSuccess = 0
	wasiEBADF   = 8
)

type aotInstance struct {
	mod *aot.Module
	mem *[]byte
}

type aotWASI struct {
	mem *[]byte
}

func (w *aotWASI) Xfd_close(int32) int32 { //nolint:revive
	return wasiSuccess
}

func (w *aotWASI) Xfd_seek(int32, int64, int32, int32) int32 { //nolint:revive
	return wasiEBADF
}

func (w *aotWASI) Xfd_write(_, iovs, n, written int32) int32 { //nolint:revive
	mem := *w.mem
	var total uint32
	for i := range addr(n) {
		off := addr(iovs) + 8*i
		if off+8 > uint64(len(mem)) {
			return wasiEBADF
		}
		total += binary.LittleEndian.Uint32(mem[off+4:])
	}
	off := addr(written)
	if off+4 > uint64(len(mem)) {
		return wasiEBADF
	}
	binary.LittleEndian.PutUint32(mem[off:], total)
	return wasiSuccess
}

func addr(x int32) uint64 {
	return uint64(uint32(x)) //nolint:gosec
}

func u32(x int32) uint32 {
	return uint32(x) //nolint:gosec
}

func i32(x uint32) int32 {
	return int32(x) //nolint:gosec
}

func newAOTInstance() (inst *aotInstance, err error) {
	defer func() {
		if recover() != nil {
			inst, err = nil, errTrap
		}
	}()
	w := &aotWASI{}
	mod := aot.New(w)
	w.mem = mod.Xmemory().Slice()
	mod.X_initialize()
	return &aotInstance{mod: mod, mem: w.mem}, nil
}

func (a *aotInstance) call1(fn export, x uint32) (r uint32, err error) {
	defer func() {
		if recover() != nil {
			r, err = 0, errTrap
		}
	}()
	v := i32(x)
	switch fn {
	case fnMalloc:
		return u32(a.mod.Xmalloc(v)), nil
	case fnFree:
		a.mod.Xfree(v)
	case fnCreate:
		return u32(a.mod.Xcodec2_create(v)), nil
	case fnDestroy:
		a.mod.Xcodec2_destroy(v)
	case fnSamplesPerFrame:
		return u32(a.mod.Xcodec2_samples_per_frame(v)), nil
	case fnBitsPerFrame:
		return u32(a.mod.Xcodec2_bits_per_frame(v)), nil
	case fnEncode, fnDecode, exportCount:
		return 0, errExports
	}
	return 0, nil
}

func (a *aotInstance) call3(fn export, x, y, z uint32) (err error) {
	defer func() {
		if recover() != nil {
			err = errTrap
		}
	}()
	switch fn {
	case fnEncode:
		a.mod.Xcodec2_encode(i32(x), i32(y), i32(z))
	case fnDecode:
		a.mod.Xcodec2_decode(i32(x), i32(y), i32(z))
	default:
		return errExports
	}
	return nil
}

func (a *aotInstance) read(off uint32, dst []byte) bool {
	mem := *a.mem
	if uint64(off)+uint64(len(dst)) > uint64(len(mem)) {
		return false
	}
	copy(dst, mem[off:])
	return true
}

func (a *aotInstance) write(off uint32, src []byte) bool {
	mem := *a.mem
	if uint64(off)+uint64(len(src)) > uint64(len(mem)) {
		return false
	}
	copy(mem[off:], src)
	return true
}
