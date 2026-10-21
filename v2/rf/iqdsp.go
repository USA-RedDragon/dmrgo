//go:build !(tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp))

package rf

func lo16(x uint32) int32 { return int32(int16(x)) }

func hi16(x uint32) int32 { return int32(x) >> 16 }

func smuad(x, y uint32) int32 { return lo16(x)*lo16(y) + hi16(x)*hi16(y) }

func smusdx(x, y uint32) int32 { return lo16(x)*hi16(y) - hi16(x)*lo16(y) }

func smmulr(x, y int32) int32 { return int32((int64(x)*int64(y) + 1<<31) >> 32) }

func addAsr10(a, b int32) int32 { return a + b>>10 }

func (r *iqLowRx) halfRun(x []uint32) { r.halfRunGo(x) }

func (r *iqLowRx) fill(x []uint32) int {
	m := min(len(x), iqLowBuf)
	r.n = 0
	r.halfRun(x[:m])
	r.n = uint8(m)
	return m
}
