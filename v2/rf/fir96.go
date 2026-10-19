//go:build !(tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp))

package rf

// fir96 returns the matched filter output in Q15 of the samples w at
// 9.6 kHz, oldest first. The taps are written out, so that TinyGo folds
// them into the code at every -opt.
func fir96(w *[audio96Taps]int8) int32 {
	return int32(audio96RRCQ15[8])*int32(w[8]) +
		int32(audio96RRCQ15[0])*(int32(w[0])+int32(w[16])) +
		int32(audio96RRCQ15[1])*(int32(w[1])+int32(w[15])) +
		int32(audio96RRCQ15[2])*(int32(w[2])+int32(w[14])) +
		int32(audio96RRCQ15[3])*(int32(w[3])+int32(w[13])) +
		int32(audio96RRCQ15[4])*(int32(w[4])+int32(w[12])) +
		int32(audio96RRCQ15[5])*(int32(w[5])+int32(w[11])) +
		int32(audio96RRCQ15[6])*(int32(w[6])+int32(w[10])) +
		int32(audio96RRCQ15[7])*(int32(w[7])+int32(w[9]))
}
