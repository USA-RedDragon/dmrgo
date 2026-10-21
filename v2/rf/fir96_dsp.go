//go:build tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp)

package rf

import (
	"device/arm"
	"unsafe"
)

func fir96[_ any](w *[audio96Taps]int8) int32 {
	c := &audio96RRCQ15
	c02 := uint32(uint16(c[0])) | uint32(uint16(c[2]))<<16
	c13 := uint32(uint16(c[1])) | uint32(uint16(c[3]))<<16
	c46 := uint32(uint16(c[4])) | uint32(uint16(c[6]))<<16
	c57 := uint32(uint16(c[5])) | uint32(uint16(c[7]))<<16
	p := uintptr(unsafe.Pointer(w))
	a := arm.AsmFull("ldr {}, [{p}]", map[string]interface{}{"p": p})
	b := arm.AsmFull("ldr {}, [{p}, #13]", map[string]interface{}{"p": p})
	b = arm.AsmFull("rev {}, {x}", map[string]interface{}{"x": b})
	s02 := arm.AsmFull("sadd16 {}, {x}, {y}", map[string]interface{}{
		"x": arm.AsmFull("sxtb16 {}, {x}", map[string]interface{}{"x": a}),
		"y": arm.AsmFull("sxtb16 {}, {x}", map[string]interface{}{"x": b}),
	})
	s13 := arm.AsmFull("sadd16 {}, {x}, {y}", map[string]interface{}{
		"x": arm.AsmFull("sxtb16 {}, {x}, ror #8", map[string]interface{}{"x": a}),
		"y": arm.AsmFull("sxtb16 {}, {x}, ror #8", map[string]interface{}{"x": b}),
	})
	a = arm.AsmFull("ldr {}, [{p}, #4]", map[string]interface{}{"p": p})
	b = arm.AsmFull("ldr {}, [{p}, #9]", map[string]interface{}{"p": p})
	b = arm.AsmFull("rev {}, {x}", map[string]interface{}{"x": b})
	s46 := arm.AsmFull("sadd16 {}, {x}, {y}", map[string]interface{}{
		"x": arm.AsmFull("sxtb16 {}, {x}", map[string]interface{}{"x": a}),
		"y": arm.AsmFull("sxtb16 {}, {x}", map[string]interface{}{"x": b}),
	})
	s57 := arm.AsmFull("sadd16 {}, {x}, {y}", map[string]interface{}{
		"x": arm.AsmFull("sxtb16 {}, {x}, ror #8", map[string]interface{}{"x": a}),
		"y": arm.AsmFull("sxtb16 {}, {x}, ror #8", map[string]interface{}{"x": b}),
	})
	acc := uintptr(int32(c[8]) * int32(arm.AsmFull("ldrsb {}, [{p}, #8]", map[string]interface{}{"p": p})))
	acc = arm.AsmFull("smlad {}, {x}, {y}, {z}", map[string]interface{}{"x": s02, "y": uintptr(c02), "z": acc})
	acc = arm.AsmFull("smlad {}, {x}, {y}, {z}", map[string]interface{}{"x": s13, "y": uintptr(c13), "z": acc})
	acc = arm.AsmFull("smlad {}, {x}, {y}, {z}", map[string]interface{}{"x": s46, "y": uintptr(c46), "z": acc})
	acc = arm.AsmFull("smlad {}, {x}, {y}, {z}", map[string]interface{}{"x": s57, "y": uintptr(c57), "z": acc})
	return int32(acc)
}
