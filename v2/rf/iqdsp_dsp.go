//go:build tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp)

package rf

import "device/arm"

func smuad(x, y uint32) int32 {
	return int32(arm.AsmFull("smuad {}, {x}, {y}", map[string]interface{}{"x": uintptr(x), "y": uintptr(y)}))
}

func smusdx(x, y uint32) int32 {
	return int32(arm.AsmFull("smusdx {}, {x}, {y}", map[string]interface{}{"x": uintptr(x), "y": uintptr(y)}))
}

func smmulr(x, y int32) int32 {
	return int32(arm.AsmFull("smmulr {}, {x}, {y}", map[string]interface{}{"x": uintptr(x), "y": uintptr(y)}))
}
