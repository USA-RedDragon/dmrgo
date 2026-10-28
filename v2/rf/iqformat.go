package rf

import (
	"encoding/binary"
	"math"
	"unsafe"
)

// IQFormat is the layout of one I/Q sample in the bytes of a Receiver.
// Each sample is I and then Q.
type IQFormat uint8

// The I/Q formats.
const (
	// IQFormatDefault is the zero value: IQFormatCS16.
	IQFormatDefault IQFormat = iota
	// IQFormatCU8 is unsigned 8-bit, as rtl_sdr writes it.
	IQFormatCU8
	// IQFormatCS8 is signed 8-bit, as hackrf_transfer writes it.
	IQFormatCS8
	// IQFormatCS16 is signed 16-bit little-endian, as PlutoSDR, LimeSDR,
	// USRP and the radio chips give it.
	IQFormatCS16
	// IQFormatCF32 is little-endian float32, as GNU Radio gives it, with
	// full scale at 1.
	IQFormatCF32
)

func (f IQFormat) bytes() int {
	switch f {
	case IQFormatCU8, IQFormatCS8:
		return 2
	case IQFormatCF32:
		return 8
	case IQFormatDefault, IQFormatCS16:
	}
	return 4
}

type sampleCarry struct {
	held [2]uint32
	n    uint8
}

func (c *sampleCarry) bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&c.held)), 8)
}

func (c *sampleCarry) fill(x []byte, size int) (rest []byte, done bool) {
	k := copy(c.bytes()[c.n:size], x)
	c.n += uint8(k)
	if int(c.n) < size {
		return nil, false
	}
	c.n = 0
	return x[k:], true
}

func (c *sampleCarry) keep(x []byte) {
	c.n = uint8(copy(c.bytes(), x))
}

func basebandSample(s []byte, f IQFormat) complex64 {
	if f == IQFormatCF32 {
		return complex(math.Float32frombits(binary.LittleEndian.Uint32(s)), math.Float32frombits(binary.LittleEndian.Uint32(s[4:])))
	}
	return complex(float32(int16(binary.LittleEndian.Uint16(s)))/32768, float32(int16(binary.LittleEndian.Uint16(s[2:])))/32768)
}
