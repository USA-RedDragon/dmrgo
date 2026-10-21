//go:build tinygo && (atsamd51 || atsame5x || nrf52 || nrf52833 || nrf52840 || stm32f4 || stm32f7 || stm32l4 || stm32wlx || mimxrt1062 || mk66f18 || rp2350 || mps2_an386 || dmrgo_dsp)

package rf

import (
	"device/arm"
	"unsafe"
)

const iqLowHalfAsm = `
	.pushsection .text.dmrgo_iqlow_half, "ax", %progbits
	.globl dmrgo_iqlow_half
	.globl dmrgo_iqlow_fill
	.p2align 2
	.type dmrgo_iqlow_fill, %function
	.thumb_func
dmrgo_iqlow_fill:
	cmp r1, #16
	it hi
	movhi r1, #16
	strb r1, [r2, #34]
	cbz r1, 9f
	push {r1, r2, r4-r11, lr}
	mov r3, r2
	add r2, r2, #36
	movw r9, #19200
	b 8f
9:
	movs r0, #0
	bx lr
	.type dmrgo_iqlow_half, %function
	.thumb_func
dmrgo_iqlow_half:
	ldr r12, [sp]
	push {r1, r3, r4-r11, lr}
	mov r9, r12
8:
	ldr r8, [r3, #24]
	ldm r3, {r3, r5, r6, r7}
	tst r1, #1
	add r1, r0, r1, lsl #2
	beq 1f
	mov r4, r3
	ldr r3, [r0], #4
	b 5f
1:
	ldr r4, [r0], #4
	smuad r10, r4, r3
	smusdx r11, r3, r4
	smuad r12, r4, r4
	add r12, r12, #512
	add r7, r7, r12, asr #10
	add r12, r10, #512
	add r5, r5, r12, asr #10
	add r12, r11, #512
	add r6, r6, r12, asr #10
	eor r12, r10, r10, asr #31
	add r12, r12, r10, lsr #31
	eor lr, r11, r11, asr #31
	add lr, lr, r11, lsr #31
	eor r11, r11, r10
	cmp lr, r12
	mov r3, r12
	it hi
	movhi r3, lr
	it ls
	movls r12, lr
	clz lr, r3
	lsl r3, r3, lr
	lsl r12, r12, lr
	lsr r3, r3, #25
	adr lr, 3f
	lsr r12, r12, #16
	ldrh r3, [lr, r3, lsl #1]
	mul r3, r12, r3
	ubfx r3, r3, #24, #6
	and r10, r10, #0x80000000
	ldrh r12, [lr, r3, lsl #1]
	ldr r3, [r0], #4
	it hi
	rsbhi r12, r12, #0x4000
	eor r12, r12, r11, asr #31
	sub r12, r12, r11, asr #31
	rsb r12, r8, r12, lsl #16
	eor r12, r12, r10
	smmulr r12, r12, r9
	strh r12, [r2], #2
5:
	smuad r10, r3, r4
	smusdx r11, r4, r3
	smuad r12, r3, r3
	add r12, r12, #512
	add r7, r7, r12, asr #10
	add r12, r10, #512
	add r5, r5, r12, asr #10
	add r12, r11, #512
	add r6, r6, r12, asr #10
	eor r12, r10, r10, asr #31
	add r12, r12, r10, lsr #31
	eor lr, r11, r11, asr #31
	add lr, lr, r11, lsr #31
	eor r11, r11, r10
	cmp lr, r12
	mov r4, r12
	it hi
	movhi r4, lr
	it ls
	movls r12, lr
	clz lr, r4
	lsl r4, r4, lr
	lsl r12, r12, lr
	lsr r4, r4, #25
	adr lr, 3f
	lsr r12, r12, #16
	ldrh r4, [lr, r4, lsl #1]
	mul r4, r12, r4
	ubfx r4, r4, #24, #6
	and r10, r10, #0x80000000
	ldrh r12, [lr, r4, lsl #1]
	it hi
	rsbhi r12, r12, #0x4000
	eor r12, r12, r11, asr #31
	sub r12, r12, r11, asr #31
	rsb r12, r8, r12, lsl #16
	eor r12, r12, r10
	smmulr r12, r12, r9
	strh r12, [r2], #2
	cmp r0, r1
	bne 1b
	ldr r10, [sp, #4]
	stm r10, {r3, r5, r6, r7}
	pop {r0, r1, r4-r11, pc}
	.p2align 2
3:
	.short 81, 244, 407, 570, 732, 894, 1056, 1217, 1377, 1537, 1696, 1854, 2012, 2168, 2324, 2478
	.short 2632, 2784, 2935, 3085, 3233, 3380, 3526, 3670, 3813, 3955, 4095, 4233, 4370, 4505, 4639, 4771
	.short 4901, 5030, 5157, 5282, 5406, 5528, 5649, 5768, 5885, 6000, 6114, 6227, 6337, 6446, 6554, 6660
	.short 6764, 6867, 6968, 7068, 7166, 7262, 7358, 7451, 7544, 7635, 7724, 7812, 7899, 7984, 8068, 8151
	.short 32263, 31775, 31300, 30840, 30393, 29959, 29537, 29127, 28728, 28339, 27962, 27594, 27235, 26886, 26546, 26214
	.short 25890, 25575, 25266, 24966, 24672, 24385, 24105, 23831, 23563, 23301, 23045, 22795, 22550, 22310, 22075, 21845
	.short 21620, 21399, 21183, 20971, 20763, 20560, 20360, 20164, 19972, 19784, 19599, 19418, 19239, 19065, 18893, 18724
	.short 18558, 18396, 18236, 18078, 17924, 17772, 17623, 17476, 17331, 17189, 17050, 16912, 16777, 16644, 16513, 16384
	.size dmrgo_iqlow_half, . - dmrgo_iqlow_half
	.popsection
`

const leanPushAsm = `
	.pushsection .text.dmrgo_lean_push, "ax", %progbits
	.globl dmrgo_lean_push
	.p2align 1
	.type dmrgo_lean_push, %function
	.thumb_func
dmrgo_lean_push:
	ldrb r2, [r0, #154]
	add r3, r0, r2
	strb r1, [r3, #8]
	adds r12, r2, #1
	cmp r12, #146
	it eq
	moveq r12, #0
	strb r12, [r0, #154]
	cmp r2, #24
	ite hs
	ldrsbhs r12, [r3, #-16]
	ldrsblo r12, [r3, #130]
	ldrd r2, r3, [r0, #192]
	sub r12, r1, r12
	add r2, r2, r12
	add r1, r1, r1, lsl #1
	lsl r3, r3, #1
	cmp r2, r1, lsl #3
	it lt
	addlt r3, r3, #1
	strd r2, r3, [r0, #192]
	ldr r2, [r0]
	adds r2, r2, #1
	str r2, [r0]
	bcs 1f
	bx lr
1:
	ldr r3, [r0, #4]
	adds r3, r3, #1
	str r3, [r0, #4]
	bx lr
	.size dmrgo_lean_push, . - dmrgo_lean_push
	.popsection
`

//export dmrgo_lean_push
func leanPush(p, q uintptr)

//go:noinline
//go:section .text.pushSym
func (r *leanCore) pushSym(x float32) {
	arm.Asm(leanPushAsm)
	_ = [1]struct{}{}[(unsafe.Offsetof(r.ring)-unsafe.Offsetof(r.ns)-8)|(unsafe.Offsetof(r.head)-unsafe.Offsetof(r.ns)-154)|(unsafe.Offsetof(r.ringSum)-unsafe.Offsetof(r.ns)-192)|(unsafe.Offsetof(r.signs)-unsafe.Offsetof(r.ns)-196)]
	_ = [1]struct{}{}[leanRing-146]
	_ = [1]struct{}{}[syncSymbols-24]
	leanPush(uintptr(unsafe.Pointer(&r.ns)), uintptr(int32(symQ(x, r.off))))
}

//export dmrgo_iqlow_half
func iqLowHalf(x, n, out, s, k uintptr)

//export dmrgo_iqlow_fill
func iqLowFill(x, n, s uintptr) uintptr

//go:noinline
func (r *iqLowRx) fill(x []uint32) int {
	arm.Asm(iqLowHalfAsm)
	return int(iqLowFill(uintptr(unsafe.Pointer(unsafe.SliceData(x))), uintptr(len(x)), uintptr(unsafe.Pointer(&r.q))))
}

func (r *iqLowRx) halfRun(x []uint32) {
	out := r.buf[r.n : int(r.n)+len(x)]
	if len(x) == 0 {
		return
	}
	iqLowHalf(uintptr(unsafe.Pointer(unsafe.SliceData(x))), uintptr(len(x)), uintptr(unsafe.Pointer(&out[0])), uintptr(unsafe.Pointer(&r.q)), uintptr(2*r.d.hz()))
}
