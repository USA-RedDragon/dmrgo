TEXT KFWD(SB), NOSPLIT, $0-84
	KPRE
	FWDARGS

fl:
	LEAQ 8(BX), AX
	CMPQ AX, CX
	JGT  ft
	FWDLOAD
	FWDLANE(0, 16, 32, DI, fl0)
	FWDLANE(4, 20, 36, R8, fl1)
	FWDLANE(8, 24, 40, R9, fl2)
	FWDLANE(12, 28, 44, R10, fl3)
	ADDQ $8, BX
	JMP  fl

ft:
	CMPQ BX, CX
	JGE  fdone
	TAILLOAD
	FWDLANET(0, 16, 32, DI, ft0)
	FWDLANET(4, 20, 36, R8, ft1)
	FWDLANET(8, 24, 40, R9, ft2)
	FWDLANET(12, 28, 44, R10, ft3)

fdone:
	HMAXRET
	VMOVSS X15, ret+80(FP)
	VZEROUPPER
	RET

TEXT KBWD(SB), NOSPLIT, $16-112
	KPRE
	MOVQ    src+0(FP), SI
	MOVQ    e0+8(FP), DI
	MOVQ    e1+16(FP), R8
	MOVQ    e2+24(FP), R9
	MOVQ    e3+32(FP), R10
	MOVQ    dst+40(FP), R11
	MOVQ    cs+48(FP), R13
	MOVQ    sn+56(FP), R14
	MOVQ    n+64(FP), CX
	MOVQ    yb+96(FP), R12
	MOVQ    best+88(FP), AX
	VMOVUPS (AX), X4
	VMOVUPS X4, 0(SP)
	XORQ    BX, BX

bl:
	LEAQ         8(BX), AX
	CMPQ         AX, CX
	JGT          bt
	VMOVUPS      (SI)(BX*4), Y0
	VMOVUPS      (R13)(BX*4), Y1
	VMOVUPS      (R14)(BX*4), Y2
	VBROADCASTSS thr+80(FP), Y3
	VCMPPS       $0x11, Y3, Y0, Y3
	MOVQ         l+72(FP), DX
	BWDV(0, 16, 32, (DI)(BX*4), Y4)
	BWDV(4, 20, 36, (R8)(BX*4), Y5)
	BWDV(8, 24, 40, (R9)(BX*4), Y6)
	BWDV(12, 28, 44, (R10)(BX*4), Y15)
	TPTAIL(bla, blb, blc)
	VMOVUPS      Y4, (R11)(BX*4)
	ADDQ         $128, R12
	ADDQ         $8, BX
	JMP          bl

bt:
	CMPQ         BX, CX
	JGE          bp0
	MOVQ         CX, AX
	SUBQ         BX, AX
	NEGQ         AX
	LEAQ         xtail<>+32(SB), DX
	VMOVUPS      (DX)(AX*4), Y13
	VMASKMOVPS   (SI)(BX*4), Y13, Y0
	VMASKMOVPS   (R13)(BX*4), Y13, Y1
	VMASKMOVPS   (R14)(BX*4), Y13, Y2
	VBROADCASTSS thr+80(FP), Y3
	VCMPPS       $0x11, Y3, Y0, Y3
	VPCMPEQD     Y4, Y4, Y4
	VXORPS       Y4, Y13, Y4
	VORPS        Y4, Y3, Y3
	MOVQ         l+72(FP), DX
	VMASKMOVPS   (DI)(BX*4), Y13, Y11
	BWDV(0, 16, 32, Y11, Y4)
	VMASKMOVPS   (R8)(BX*4), Y13, Y11
	BWDV(4, 20, 36, Y11, Y5)
	VMASKMOVPS   (R9)(BX*4), Y13, Y11
	BWDV(8, 24, 40, Y11, Y6)
	VMASKMOVPS   (R10)(BX*4), Y13, Y11
	BWDV(12, 28, 44, Y11, Y15)
	TPTAIL(bta, btb, btc)
	VMASKMOVPS   Y4, Y13, (R11)(BX*4)

bp0:
	MOVQ yb+96(FP), R8
	MOVQ pm+104(FP), SI
	MOVQ n+64(FP), DI
	ADDQ $7, DI
	SHRQ $3, DI

bp:
	MOVBLZX (SI), R13
	CHAIN2(0, 0, bc0, bd0)
	CHAIN2(32, 4, bc1, bd1)
	CHAIN2(64, 8, bc2, bd2)
	CHAIN2(96, 12, bc3, bd3)
	ADDQ    $128, R8
	INCQ    SI
	DECQ    DI
	JNE     bp
	MOVQ    best+88(FP), AX
	VMOVUPS 0(SP), X4
	VMOVUPS X4, (AX)
	VZEROUPPER
	RET
