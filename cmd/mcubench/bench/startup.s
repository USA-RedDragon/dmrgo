.cfi_sections .debug_frame
.syntax unified

.section .text.Default_Handler
.global  Default_Handler
.type    Default_Handler, %function
Default_Handler:
    .cfi_startproc
    wfe
    b    Default_Handler
    .cfi_endproc
.size Default_Handler, .-Default_Handler

.macro IRQ handler
    .weak  \handler
    .set   \handler, Default_Handler
.endm

.section .isr_vector, "a", %progbits
.global  __isr_vector
__isr_vector:
    .long _stack_top
    .long FPU_Reset
    .long NMI_Handler
    .long HardFault_Handler
    .long MemoryManagement_Handler
    .long BusFault_Handler
    .long UsageFault_Handler
    .long 0
    .long 0
    .long 0
    .long 0
    .long SVC_Handler
    .long DebugMon_Handler
    .long 0
    .long PendSV_Handler
    .long SysTick_Handler

    IRQ NMI_Handler
    IRQ HardFault_Handler
    IRQ MemoryManagement_Handler
    IRQ BusFault_Handler
    IRQ UsageFault_Handler
    IRQ SVC_Handler
    IRQ DebugMon_Handler
    IRQ PendSV_Handler
    IRQ SysTick_Handler

.size __isr_vector, .-__isr_vector

.section .text.FPU_Reset
.global  FPU_Reset
.type    FPU_Reset, %function
.thumb_func
FPU_Reset:
    ldr  r0, =0xE000ED88
    ldr  r1, [r0]
    orr  r1, r1, #(0xF << 20)
    str  r1, [r0]
    dsb
    isb
    b    Reset_Handler
.size FPU_Reset, .-FPU_Reset
