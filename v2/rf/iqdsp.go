//go:build !(tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp))

package rf

// The multiplies of the Cortex-M4 DSP instructions that iqLowRx uses, in
// portable Go. A pair is a word with two signed halves, the low half
// first. iqdsp_dsp.go has the instructions; both give the same result.

func lo16(x uint32) int32 { return int32(int16(x)) }

func hi16(x uint32) int32 { return int32(x) >> 16 }

// smuad returns lo·lo + hi·hi of the pairs x and y.
func smuad(x, y uint32) int32 { return lo16(x)*lo16(y) + hi16(x)*hi16(y) }

// smusdx returns lo·hi - hi·lo of the pairs x and y.
func smusdx(x, y uint32) int32 { return lo16(x)*hi16(y) - hi16(x)*lo16(y) }

// smmulr returns the high word of x·y, rounded.
func smmulr(x, y int32) int32 { return int32((int64(x)*int64(y) + 1<<31) >> 32) }
