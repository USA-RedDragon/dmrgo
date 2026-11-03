//go:build amd64 && !tinygo

#include "textflag.h"

DATA xsign<>+0(SB)/8, $0x8000000080000000
DATA xsign<>+8(SB)/8, $0x8000000080000000
DATA xsign<>+16(SB)/8, $0x8000000080000000
DATA xsign<>+24(SB)/8, $0x8000000080000000
GLOBL xsign<>(SB), RODATA|NOPTR, $32

DATA xeight<>+0(SB)/8, $0x4100000041000000
DATA xeight<>+8(SB)/8, $0x4100000041000000
DATA xeight<>+16(SB)/8, $0x4100000041000000
DATA xeight<>+24(SB)/8, $0x4100000041000000
GLOBL xeight<>(SB), RODATA|NOPTR, $32

DATA xn96<>+0(SB)/8, $0x42c0000042c00000
DATA xn96<>+8(SB)/8, $0x42c0000042c00000
DATA xn96<>+16(SB)/8, $0x42c0000042c00000
DATA xn96<>+24(SB)/8, $0x42c0000042c00000
GLOBL xn96<>(SB), RODATA|NOPTR, $32

DATA xm127<>+0(SB)/8, $0x0000007f0000007f
DATA xm127<>+8(SB)/8, $0x0000007f0000007f
DATA xm127<>+16(SB)/8, $0x0000007f0000007f
DATA xm127<>+24(SB)/8, $0x0000007f0000007f
GLOBL xm127<>(SB), RODATA|NOPTR, $32

DATA xninf<>+0(SB)/8, $0xf149f2caf149f2ca
DATA xninf<>+8(SB)/8, $0xf149f2caf149f2ca
DATA xninf<>+16(SB)/8, $0xf149f2caf149f2ca
DATA xninf<>+24(SB)/8, $0xf149f2caf149f2ca
GLOBL xninf<>(SB), RODATA|NOPTR, $32

DATA xtail<>+0(SB)/8, $0xffffffffffffffff
DATA xtail<>+8(SB)/8, $0xffffffffffffffff
DATA xtail<>+16(SB)/8, $0xffffffffffffffff
DATA xtail<>+24(SB)/8, $0xffffffffffffffff
DATA xtail<>+32(SB)/8, $0
DATA xtail<>+40(SB)/8, $0
DATA xtail<>+48(SB)/8, $0
DATA xtail<>+56(SB)/8, $0
GLOBL xtail<>(SB), RODATA|NOPTR, $64

DATA xupto<>+0(SB)/4, $1
DATA xupto<>+4(SB)/4, $3
DATA xupto<>+8(SB)/4, $7
DATA xupto<>+12(SB)/4, $15
DATA xupto<>+16(SB)/4, $31
DATA xupto<>+20(SB)/4, $63
DATA xupto<>+24(SB)/4, $127
DATA xupto<>+28(SB)/4, $255
GLOBL xupto<>(SB), RODATA|NOPTR, $32

DATA x32z<>+0(SB)/8, $0x0000002000000020
DATA x32z<>+8(SB)/8, $0x0000002000000020
DATA x32z<>+16(SB)/8, $0x0000002000000020
DATA x32z<>+24(SB)/8, $0x0000002000000020
DATA x32z<>+32(SB)/8, $0x0000002000000020
DATA x32z<>+40(SB)/8, $0x0000002000000020
DATA x32z<>+48(SB)/8, $0x0000002000000020
DATA x32z<>+56(SB)/8, $0x0000002000000020
GLOBL x32z<>(SB), RODATA|NOPTR, $64

DATA x64z<>+0(SB)/8, $0x0000004000000040
DATA x64z<>+8(SB)/8, $0x0000004000000040
DATA x64z<>+16(SB)/8, $0x0000004000000040
DATA x64z<>+24(SB)/8, $0x0000004000000040
DATA x64z<>+32(SB)/8, $0x0000004000000040
DATA x64z<>+40(SB)/8, $0x0000004000000040
DATA x64z<>+48(SB)/8, $0x0000004000000040
DATA x64z<>+56(SB)/8, $0x0000004000000040
GLOBL x64z<>(SB), RODATA|NOPTR, $64

#define FWDM(RE, IM, B, OUT) \
	VBROADCASTSS RE(DX), OUT \
	VMULPS       Y1, OUT, OUT \
	VADDPS       Y0, OUT, OUT \
	VBROADCASTSS IM(DX), Y12 \
	VMULPS       Y2, Y12, Y12 \
	VADDPS       Y12, OUT, OUT \
	VBROADCASTSS B(DX), Y12 \
	VADDPS       Y12, OUT, OUT

#define BWDV(RE, IM, B, E, OUT) \
	VBROADCASTSS RE(DX), OUT \
	VMULPS       Y1, OUT, OUT \
	VBROADCASTSS IM(DX), Y12 \
	VMULPS       Y2, Y12, Y12 \
	VADDPS       Y12, OUT, OUT \
	VBROADCASTSS B(DX), Y12 \
	VADDPS       Y12, OUT, OUT \
	VADDPS       E, OUT, OUT

#define VLAG(A, B, R, L) \
	VSUBPS     B, A, Y7 \
	VXORPS     Y14, Y14, Y14 \
	VCMPPS     $0x11, Y14, Y7, Y8 \
	VBLENDVPS  Y8, B, A, R \
	VANDPS     xsign<>(SB), Y8, Y8 \
	VXORPS     Y8, Y7, Y7 \
	VMULPS     xeight<>(SB), Y7, Y7 \
	VCMPPS     $0x1d, xn96<>(SB), Y7, Y8 \
	VORPS      Y3, Y8, Y8 \
	VMOVMSKPS  Y8, AX \
	CMPL       AX, $0xff \
	JEQ        L \
	VCVTTPS2DQ Y7, Y9 \
	VCVTDQ2PS  Y9, Y10 \
	VSUBPS     Y10, Y7, Y10 \
	VPAND      xm127<>(SB), Y9, Y9 \
	LEAQ       ·jacobian(SB), AX \
	VPCMPEQD   Y11, Y11, Y11 \
	VXORPS     Y12, Y12, Y12 \
	VGATHERDPS Y11, (AX)(Y9*4), Y12 \
	VADDPS     Y12, R, Y12 \
	LEAQ       ·jacobianSlope(SB), AX \
	VPCMPEQD   Y11, Y11, Y11 \
	VXORPS     Y14, Y14, Y14 \
	VGATHERDPS Y11, (AX)(Y9*4), Y14 \
	VMULPS     Y14, Y10, Y14 \
	VADDPS     Y14, Y12, Y12 \
	VBLENDVPS  Y8, R, Y12, R \
L:

#define VLAZ(A, B, R, L) \
	VSUBPS     B, A, Y7 \
	VXORPS     Y14, Y14, Y14 \
	VCMPPS     $0x11, Y14, Y7, Y8 \
	VBLENDVPS  Y8, B, A, R \
	VANDPS     xsign<>(SB), Y8, Y8 \
	VXORPS     Y8, Y7, Y7 \
	VMULPS     xeight<>(SB), Y7, Y7 \
	VCMPPS     $0x1d, xn96<>(SB), Y7, Y8 \
	VCVTTPS2DQ Y7, Y9 \
	VCVTDQ2PS  Y9, Y10 \
	VSUBPS     Y10, Y7, Y10 \
	VPAND      xm127<>(SB), Y9, Y9 \
	VPTESTMD   x32z<>(SB), Z9, K1 \
	VPTESTMD   x64z<>(SB), Z9, K2 \
	VMOVAPS    Z9, Z28 \
	VPERMI2PS  Z17, Z16, Z28 \
	VMOVAPS    Z9, Z29 \
	VPERMI2PS  Z19, Z18, Z29 \
	VMOVAPS    Z9, Z30 \
	VPERMI2PS  Z21, Z20, Z30 \
	VMOVAPS    Z29, K1, Z28 \
	VMOVAPS    Z30, K2, Z28 \
	VADDPS     Y28, R, Y12 \
	VMOVAPS    Z9, Z28 \
	VPERMI2PS  Z23, Z22, Z28 \
	VMOVAPS    Z9, Z29 \
	VPERMI2PS  Z25, Z24, Z29 \
	VMOVAPS    Z9, Z30 \
	VPERMI2PS  Z27, Z26, Z30 \
	VMOVAPS    Z29, K1, Z28 \
	VMOVAPS    Z30, K2, Z28 \
	VMULPS     Y28, Y10, Y14 \
	VADDPS     Y14, Y12, Y12 \
	VBLENDVPS  Y8, R, Y12, R

#define ZPRE \
	LEAQ    ·jacobian(SB), AX \
	VMOVUPS 0(AX), Z16 \
	VMOVUPS 64(AX), Z17 \
	VMOVUPS 128(AX), Z18 \
	VMOVUPS 192(AX), Z19 \
	VMOVUPS 256(AX), Z20 \
	VMOVUPS 320(AX), Z21 \
	LEAQ    ·jacobianSlope(SB), AX \
	VMOVUPS 0(AX), Z22 \
	VMOVUPS 64(AX), Z23 \
	VMOVUPS 128(AX), Z24 \
	VMOVUPS 192(AX), Z25 \
	VMOVUPS 256(AX), Z26 \
	VMOVUPS 320(AX), Z27

#define FWDLANE(RE, IM, B, D, L) \
	FWDM(RE, IM, B, Y4) \
	VMOVUPS   (D)(BX*4), Y5 \
	VLA(Y5, Y4, Y6, L) \
	VBLENDVPS Y3, Y5, Y6, Y4 \
	VMOVUPS   Y4, (D)(BX*4) \
	VBLENDVPS Y3, Y15, Y6, Y6 \
	VMAXPS    Y15, Y6, Y15

#define FWDLANET(RE, IM, B, D, L) \
	FWDM(RE, IM, B, Y4) \
	VMASKMOVPS (D)(BX*4), Y13, Y5 \
	VLA(Y5, Y4, Y6, L) \
	VBLENDVPS  Y3, Y5, Y6, Y4 \
	VMASKMOVPS Y4, Y13, (D)(BX*4) \
	VBLENDVPS  Y3, Y15, Y6, Y6 \
	VMAXPS     Y15, Y6, Y15

#define FIRSTLANE(RE, IM, B, D) \
	FWDM(RE, IM, B, Y4) \
	VBLENDVPS Y3, xninf<>(SB), Y4, Y5 \
	VMOVUPS   Y5, (D)(BX*4) \
	VBLENDVPS Y3, Y15, Y4, Y4 \
	VMAXPS    Y15, Y4, Y15

#define FIRSTLANET(RE, IM, B, D) \
	FWDM(RE, IM, B, Y4) \
	VBLENDVPS  Y3, xninf<>(SB), Y4, Y5 \
	VMASKMOVPS Y5, Y13, (D)(BX*4) \
	VBLENDVPS  Y3, Y15, Y4, Y4 \
	VMAXPS     Y15, Y4, Y15

#define FWDARGS \
	MOVQ         src+0(FP), SI \
	MOVQ         d0+8(FP), DI \
	MOVQ         d1+16(FP), R8 \
	MOVQ         d2+24(FP), R9 \
	MOVQ         d3+32(FP), R10 \
	MOVQ         cs+40(FP), R13 \
	MOVQ         sn+48(FP), R14 \
	MOVQ         n+56(FP), CX \
	MOVQ         l+64(FP), DX \
	VBROADCASTSS top+76(FP), Y15 \
	XORQ         BX, BX

#define FWDLOAD \
	VMOVUPS      (SI)(BX*4), Y0 \
	VMOVUPS      (R13)(BX*4), Y1 \
	VMOVUPS      (R14)(BX*4), Y2 \
	VBROADCASTSS thr+72(FP), Y3 \
	VCMPPS       $0x11, Y3, Y0, Y3

#define TAILLOAD \
	MOVQ         CX, AX \
	SUBQ         BX, AX \
	NEGQ         AX \
	LEAQ         xtail<>+32(SB), R11 \
	VMOVUPS      (R11)(AX*4), Y13 \
	VMASKMOVPS   (SI)(BX*4), Y13, Y0 \
	VMASKMOVPS   (R13)(BX*4), Y13, Y1 \
	VMASKMOVPS   (R14)(BX*4), Y13, Y2 \
	VBROADCASTSS thr+72(FP), Y3 \
	VCMPPS       $0x11, Y3, Y0, Y3 \
	VPCMPEQD     Y4, Y4, Y4 \
	VXORPS       Y4, Y13, Y4 \
	VORPS        Y4, Y3, Y3

#define HMAXRET \
	VEXTRACTF128 $1, Y15, X14 \
	VMAXPS       X14, X15, X15 \
	VPERMILPS    $0x4e, X15, X14 \
	VMAXPS       X14, X15, X15 \
	VPERMILPS    $0xb1, X15, X14 \
	VMAXPS       X14, X15, X15

#define SLOGADD(L1, L2) \
	VSUBSS      X5, X4, X6 \
	VXORPS      X7, X7, X7 \
	VUCOMISS    X6, X7 \
	JLS         L1 \
	VMOVAPS     X5, X4 \
	VXORPS      xsign<>(SB), X6, X6 \
L1: \
	VMULSS      xeight<>(SB), X6, X6 \
	VUCOMISS    xn96<>(SB), X6 \
	JCC         L2 \
	VCVTTSS2SIQ X6, AX \
	VCVTSI2SSQ  AX, X7, X7 \
	VSUBSS      X7, X6, X6 \
	ANDQ        $127, AX \
	LEAQ        ·jacobian(SB), DX \
	VADDSS      (DX)(AX*4), X4, X4 \
	LEAQ        ·jacobianSlope(SB), DX \
	VMULSS      (DX)(AX*4), X6, X6 \
	VADDSS      X6, X4, X4 \
L2:

#define SLOGADDB \
	VSUBSS      X5, X12, X6 \
	VXORPS      X7, X7, X7 \
	VCMPSS      $0x11, X7, X6, X7 \
	VBLENDVPS   X7, X5, X12, X4 \
	VANDPS      xsign<>(SB), X7, X7 \
	VXORPS      X7, X6, X6 \
	VMULSS      xeight<>(SB), X6, X6 \
	VCMPSS      $0x1d, xn96<>(SB), X6, X8 \
	VCVTTSS2SIQ X6, R12 \
	VCVTSI2SSQ  R12, X7, X7 \
	VSUBSS      X7, X6, X6 \
	ANDQ        $127, R12 \
	LEAQ        ·jacobian(SB), DX \
	VADDSS      (DX)(R12*4), X4, X7 \
	LEAQ        ·jacobianSlope(SB), DX \
	VMULSS      (DX)(R12*4), X6, X6 \
	VADDSS      X6, X7, X7 \
	VBLENDVPS   X8, X4, X7, X12

#define CHAIN(YOFF, POFF, LL, LD, L1, L2) \
	VMOVMSKPS    Y3, R12 \
	VMOVSS       POFF(SP), X12 \
	VBROADCASTSS X12, Y10 \
	VSUBPS       YOFF(SP), Y10, Y11 \
	VMULPS       xeight<>(SB), Y11, Y11 \
	VCMPPS       $0x1d, xn96<>(SB), Y11, Y11 \
	VMOVMSKPS    Y11, AX \
	ORL          R12, AX \
	NOTL         AX \
	ANDL         $0xff, AX \
	JEQ          LD \
LL: \
	BSFL         AX, DX \
	LEAL         -1(AX), R12 \
	ANDL         R12, AX \
	VMOVSS       YOFF(SP)(DX*4), X5 \
	SLOGADDB \
	TESTL        AX, AX \
	JNE          LL \
	VMOVSS       X12, POFF(SP) \
LD:

#define BWDTAIL(LA1, LA2, LA3) \
	VADDPS    Y4, Y0, Y7 \
	VMOVUPS   Y7, 0(SP) \
	VADDPS    Y5, Y0, Y7 \
	VMOVUPS   Y7, 32(SP) \
	VADDPS    Y6, Y0, Y7 \
	VMOVUPS   Y7, 64(SP) \
	VADDPS    Y15, Y0, Y7 \
	VMOVUPS   Y7, 96(SP) \
	VLA(Y4, Y5, Y4, LA1) \
	VLA(Y4, Y6, Y4, LA2) \
	VLA(Y4, Y15, Y4, LA3) \
	VBLENDVPS Y3, xninf<>(SB), Y4, Y4

#define CHAINS(A0, B0, C0, D0, A1, B1, C1, D1, A2, B2, C2, D2, A3, B3, C3, D3) \
	CHAIN(0, 128, A0, B0, C0, D0) \
	CHAIN(32, 132, A1, B1, C1, D1) \
	CHAIN(64, 136, A2, B2, C2, D2) \
	CHAIN(96, 140, A3, B3, C3, D3)

#define GOMAX(X, Y, OUT) \
	VXORPS Y15, X, Y5 \
	VXORPS Y15, Y, Y6 \
	VMINPS Y6, Y5, Y7 \
	VMINPS Y5, Y7, Y8 \
	VORPS  Y7, Y8, OUT \
	VXORPS Y15, OUT, OUT

#define VLA(A, B, R, L) VLAG(A, B, R, L)
#define KPRE
#define KFWD ·xfwdLogAVX2
#define KBWD ·xbwdLogAVX2
#include "exact_kern_amd64.h"
#undef VLA
#undef KPRE
#undef KFWD
#undef KBWD

#define VLA(A, B, R, L) VLAZ(A, B, R, L)
#define KPRE ZPRE
#define KFWD ·xfwdLog512
#define KBWD ·xbwdLog512
#include "exact_kern_amd64.h"

TEXT ·xfwdFirstAVX2(SB), NOSPLIT, $0-84
	FWDARGS

il:
	LEAQ 8(BX), AX
	CMPQ AX, CX
	JGT  it
	FWDLOAD
	FIRSTLANE(0, 16, 32, DI)
	FIRSTLANE(4, 20, 36, R8)
	FIRSTLANE(8, 24, 40, R9)
	FIRSTLANE(12, 28, 44, R10)
	ADDQ $8, BX
	JMP  il

it:
	CMPQ BX, CX
	JGE  idone
	TAILLOAD
	FIRSTLANET(0, 16, 32, DI)
	FIRSTLANET(4, 20, 36, R8)
	FIRSTLANET(8, 24, 40, R9)
	FIRSTLANET(12, 28, 44, R10)

idone:
	HMAXRET
	VMOVSS X15, ret+80(FP)
	VZEROUPPER
	RET

TEXT ·xdriftAVX2(SB), NOSPLIT, $0-28
	MOVQ         b+0(FP), DI
	MOVQ         ext+8(FP), SI
	MOVQ         n+16(FP), CX
	VBROADCASTSS pen+24(FP), Y14
	VMOVUPS      xsign<>(SB), Y15
	XORQ         BX, BX

dl:
	LEAQ    8(BX), AX
	CMPQ    AX, CX
	JGT     dt
	VMOVUPS (SI)(BX*4), Y0
	VMOVUPS 4(SI)(BX*4), Y1
	VMOVUPS 8(SI)(BX*4), Y2
	GOMAX(Y0, Y2, Y3)
	VSUBPS  Y14, Y3, Y3
	GOMAX(Y1, Y3, Y4)
	VMOVUPS Y4, (DI)(BX*4)
	ADDQ    $8, BX
	JMP     dl

dt:
	CMPQ       BX, CX
	JGE        ddone
	MOVQ       CX, AX
	SUBQ       BX, AX
	NEGQ       AX
	LEAQ       xtail<>+32(SB), DX
	VMOVUPS    (DX)(AX*4), Y13
	VMOVUPS    (SI)(BX*4), Y0
	VMOVUPS    4(SI)(BX*4), Y1
	VMOVUPS    8(SI)(BX*4), Y2
	GOMAX(Y0, Y2, Y3)
	VSUBPS     Y14, Y3, Y3
	GOMAX(Y1, Y3, Y4)
	VMASKMOVPS Y4, Y13, (DI)(BX*4)

ddone:
	VZEROUPPER
	RET

TEXT ·xcorrAVX2(SB), NOSPLIT, $0-68
	MOVQ         seg+0(FP), SI
	MOVQ         sps+8(FP), CX
	MOVQ         tre+16(FP), R8
	MOVQ         tim+24(FP), R9
	MOVQ         nw+32(FP), R10
	MOVQ         corr+56(FP), DI
	VCVTSS2SD    rr+40(FP), X0, X0
	VBROADCASTSD X0, Y12
	VCVTSS2SD    ri+44(FP), X0, X0
	VBROADCASTSD X0, Y13
	VCVTSS2SD    scale+48(FP), X0, X0
	VBROADCASTSD X0, Y14
	VXORPS       X15, X15, X15
	VXORPS       X11, X11, X11
	XORQ         BX, BX

cw:
	CMPQ   BX, R10
	JGE    cdone
	VXORPS X0, X0, X0
	VXORPS X1, X1, X1
	XORQ   DX, DX
	LEAQ   (R8)(BX*4), R11
	LEAQ   (R9)(BX*4), R12

cj:
	CMPQ         DX, CX
	JGE          cfin
	VCVTSS2SD    (SI)(DX*8), X2, X2
	VBROADCASTSD X2, Y2
	VCVTSS2SD    4(SI)(DX*8), X3, X3
	VBROADCASTSD X3, Y3
	VCVTPS2PD    (R11), Y4
	VCVTPS2PD    (R12), Y5
	VMULPD       Y4, Y2, Y6
	VMULPD       Y5, Y3, Y7
	VSUBPD       Y7, Y6, Y6
	VMULPD       Y5, Y2, Y7
	VMULPD       Y4, Y3, Y8
	VADDPD       Y8, Y7, Y7
	VCVTPD2PSY   Y6, X6
	VCVTPD2PSY   Y7, X7
	VADDPS       X6, X0, X0
	VADDPS       X7, X1, X1
	LEAQ         (R11)(R10*4), R11
	LEAQ         (R12)(R10*4), R12
	INCQ         DX
	JMP          cj

cfin:
	VCVTPS2PD  X0, Y2
	VCVTPS2PD  X1, Y3
	VMULPD     Y12, Y2, Y6
	VMULPD     Y13, Y3, Y7
	VSUBPD     Y7, Y6, Y6
	VMULPD     Y13, Y2, Y7
	VMULPD     Y12, Y3, Y8
	VADDPD     Y8, Y7, Y7
	VCVTPD2PSY Y6, X6
	VCVTPD2PSY Y7, X7
	VCVTPS2PD  X6, Y2
	VCVTPS2PD  X7, Y3
	VXORPD     Y9, Y9, Y9
	VMULPD     Y14, Y2, Y6
	VMULPD     Y9, Y3, Y7
	VSUBPD     Y7, Y6, Y6
	VMULPD     Y9, Y2, Y7
	VMULPD     Y14, Y3, Y8
	VADDPD     Y8, Y7, Y7
	VCVTPD2PSY Y6, X6
	VCVTPD2PSY Y7, X7
	VUNPCKLPS  X7, X6, X8
	VUNPCKHPS  X7, X6, X9
	VMOVUPS    X8, (DI)(BX*8)
	VMOVUPS    X9, 16(DI)(BX*8)
	VMULPS     X6, X6, X6
	VMULPS     X7, X7, X7
	VADDPS     X7, X6, X6
	VMOVUPS    xsign<>(SB), X10
	VXORPS     X10, X11, X2
	VXORPS     X10, X6, X3
	VMINPS     X3, X2, X4
	VMINPS     X2, X4, X5
	VORPS      X4, X5, X11
	VXORPS     X10, X11, X11
	ADDQ       $4, BX
	JMP        cw

cdone:
	VMOVUPS   xsign<>(SB), X10
	VPERMILPS $0x4e, X11, X6
	VXORPS    X10, X11, X2
	VXORPS    X10, X6, X3
	VMINPS    X3, X2, X4
	VMINPS    X2, X4, X5
	VORPS     X4, X5, X11
	VXORPS    X10, X11, X11
	VPERMILPS $0xb1, X11, X6
	VXORPS    X10, X11, X2
	VXORPS    X10, X6, X3
	VMINPS    X3, X2, X4
	VMINPS    X2, X4, X5
	VORPS     X4, X5, X11
	VXORPS    X10, X11, X11
	VMOVSS    X11, ret+64(FP)
	VZEROUPPER
	RET

TEXT ·xtISA(SB), NOSPLIT, $0-1
	MOVB  $0, ret+0(FP)
	XORL  AX, AX
	XORL  CX, CX
	CPUID
	CMPL  AX, $7
	JLT   done
	MOVL  $1, AX
	XORL  CX, CX
	CPUID
	ANDL  $0x18000000, CX
	CMPL  CX, $0x18000000
	JNE   done
	XORL  CX, CX
	XGETBV
	MOVL  AX, R8
	ANDL  $6, AX
	CMPL  AX, $6
	JNE   done
	MOVL  $7, AX
	XORL  CX, CX
	CPUID
	BTL   $5, BX
	JCC   done
	MOVB  $1, ret+0(FP)
	ANDL  $0xe6, R8
	CMPL  R8, $0xe6
	JNE   done
	BTL   $16, BX
	JCC   done
	BTL   $31, BX
	JCC   done
	MOVB  $2, ret+0(FP)

done:
	RET
