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

TEXT KBWD(SB), NOSPLIT, $144-96
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
	MOVQ    best+88(FP), AX
	VMOVUPS (AX), X4
	VMOVUPS X4, 128(SP)
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
	BWDTAIL(bla, blb, blc)
	VMOVUPS      Y4, (R11)(BX*4)
	CHAINS(bc0, bd0, bs0, bx0, bc1, bd1, bs1, bx1, bc2, bd2, bs2, bx2, bc3, bd3, bs3, bx3)
	ADDQ         $8, BX
	JMP          bl

bt:
	CMPQ       BX, CX
	JGE        bdone
	MOVQ       CX, AX
	SUBQ       BX, AX
	NEGQ       AX
	LEAQ       xtail<>+32(SB), DX
	VMOVUPS    (DX)(AX*4), Y13
	VMASKMOVPS (SI)(BX*4), Y13, Y0
	VMASKMOVPS (R13)(BX*4), Y13, Y1
	VMASKMOVPS (R14)(BX*4), Y13, Y2
	VBROADCASTSS thr+80(FP), Y3
	VCMPPS     $0x11, Y3, Y0, Y3
	VPCMPEQD   Y4, Y4, Y4
	VXORPS     Y4, Y13, Y4
	VORPS      Y4, Y3, Y3
	MOVQ       l+72(FP), DX
	VMASKMOVPS (DI)(BX*4), Y13, Y11
	BWDV(0, 16, 32, Y11, Y4)
	VMASKMOVPS (R8)(BX*4), Y13, Y11
	BWDV(4, 20, 36, Y11, Y5)
	VMASKMOVPS (R9)(BX*4), Y13, Y11
	BWDV(8, 24, 40, Y11, Y6)
	VMASKMOVPS (R10)(BX*4), Y13, Y11
	BWDV(12, 28, 44, Y11, Y15)
	BWDTAIL(bta, btb, btc)
	VMASKMOVPS Y4, Y13, (R11)(BX*4)
	CHAINS(tc0, td0, ts0, tx0, tc1, td1, ts1, tx1, tc2, td2, ts2, tx2, tc3, td3, ts3, tx3)

bdone:
	MOVQ    best+88(FP), AX
	VMOVUPS 128(SP), X4
	VMOVUPS X4, (AX)
	VZEROUPPER
	RET
