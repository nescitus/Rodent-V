#include "textflag.h"

// 64 neurons = 128 bytes
TEXT ·addSingleAVX2_64(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle64_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $128
	JB addsingle64_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_64(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle64_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $128
	JB subsingle64_loop
	VZEROUPPER
	RET

// 128 neurons = 256 bytes
TEXT ·addSingleAVX2_128(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle128_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $256
	JB addsingle128_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_128(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle128_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $256
	JB subsingle128_loop
	VZEROUPPER
	RET

// 256 neurons = 512 bytes
TEXT ·addSingleAVX2_256(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle256_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $512
	JB addsingle256_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_256(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle256_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $512
	JB subsingle256_loop
	VZEROUPPER
	RET

// 384 neurons = 768 bytes
TEXT ·addSingleAVX2_384(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle384_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $768
	JB addsingle384_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_384(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle384_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $768
	JB subsingle384_loop
	VZEROUPPER
	RET

// 512 neurons = 1024 bytes
TEXT ·addSingleAVX2_512(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle512_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $1024
	JB addsingle512_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_512(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle512_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $1024
	JB subsingle512_loop
	VZEROUPPER
	RET

// 768 neurons = 1536 bytes
TEXT ·addSingleAVX2_768(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
addsingle768_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $1536
	JB addsingle768_loop
	VZEROUPPER
	RET

TEXT ·subSingleAVX2_768(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ w+8(FP), CX
	XORQ R8, R8
subsingle768_loop:
	VMOVDQU (AX)(R8*1), Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)
	ADDQ $32, R8
	CMPQ R8, $1536
	JB subsingle768_loop
	VZEROUPPER
	RET

// Each array contains 64 int16 values = 128 bytes.
// One YMM register holds 16 int16 values = 32 bytes.
// Therefore the loop executes four times.
//
// dst += to - from - captured
//
// func captureAVX2(
//     a0, a1,
//     wTo0, wFrom0, wCap0,
//     wTo1, wFrom1, wCap1 *[64]int16,
// )
TEXT ·captureAVX2_64(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

capture_64_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

	ADDQ $32, R10
	CMPQ R10, $128
	JB capture_64_loop

	VZEROUPPER
	RET

	// func moveAVX2(
//     a0, a1,
//     wFrom0, wTo0,
//     wFrom1, wTo1 *[64]int16,
// )
TEXT ·moveAVX2_64(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $128
	JB move_loop

	VZEROUPPER
	RET

	// func castleAVX2(
//     a0, a1,
//     wKFrom0, wKTo0, wRFrom0, wRTo0,
//     wKFrom1, wKTo1, wRFrom1, wRTo1 *[64]int16,
// )
TEXT ·castleAVX2_64(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_64_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12
	CMPQ R12, $128
	JB castle_64_loop

	VZEROUPPER
	RET

// for 128 HL network

TEXT ·captureAVX2_128(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_128_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

    // loop limit is 256 = 2 * number of hidden neurons
	ADDQ $32, R10
	CMPQ R10, $256	
	JB capture_128_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_128(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_128:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $256
	JB move_loop_128

	VZEROUPPER
	RET

	TEXT ·castleAVX2_128(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_128_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12
	CMPQ R12, $256
	JB castle_128_loop

	VZEROUPPER
	RET

// for 256 HL network

TEXT ·captureAVX2_256(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_256_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

    // loop limit is 512 = 2 * number of hidden neurons
	ADDQ $32, R10
	CMPQ R10, $512	
	JB capture_256_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_256(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_256:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $512
	JB move_loop_256

	VZEROUPPER
	RET

	TEXT ·castleAVX2_256(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_256_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12

	// 256 int16 neurons = 512 bytes
	CMPQ R12, $512
	JB castle_256_loop

	VZEROUPPER
	RET

// for 384 HL network

TEXT ·captureAVX2_384(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_384_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

    // loop limit is 768 = 2 * 384
	ADDQ $32, R10
	CMPQ R10, $768	
	JB capture_384_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_384(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_384:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $768
	JB move_loop_384

	VZEROUPPER
	RET

	TEXT ·castleAVX2_384(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_384_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12

	// 384 int16 neurons = 768 loop steps
	CMPQ R12, $768
	JB castle_384_loop

	VZEROUPPER
	RET

// for 512 HL network

TEXT ·captureAVX2_512(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_512_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

    // loop limit is 1024 = 2 * 512
	ADDQ $32, R10
	CMPQ R10, $1024	
	JB capture_512_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_512(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_512:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $1024
	JB move_loop_512

	VZEROUPPER
	RET

TEXT ·castleAVX2_512(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_512_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12

	// 512 int16 neurons = 1024 bytes
	CMPQ R12, $1024
	JB castle_512_loop

	VZEROUPPER
	RET

// ---- three-operand (dst = src + delta) variants, 512 hidden neurons ----
//
// Same instruction sequence as moveAVX2_512/captureAVX2_512/castleAVX2_512
// above, except the first load of each perspective comes from a separate
// src pointer instead of re-reading (and later re-writing) the same dst
// address. This lets the caller skip the explicit 2048-byte copyFrom that
// used to precede every one of these calls: previously "copy parent to
// child, then child += delta", now "child = parent + delta" in one pass.
// dst may alias src (some callers update an accumulator in place across a
// move sequence, not into a fresh ply); that degrades to the same
// load-modify-store as before with no behavior change.

TEXT ·moveAVX2_512_3op(SB), NOSPLIT, $0-64
	MOVQ dst0+0(FP), AX
	MOVQ src0+8(FP), BX
	MOVQ dst1+16(FP), CX
	MOVQ src1+24(FP), DX

	MOVQ wFrom0+32(FP), SI
	MOVQ wTo0+40(FP), DI

	MOVQ wFrom1+48(FP), R8
	MOVQ wTo1+56(FP), R9

	XORQ R10, R10

move3_loop_512:
	// Perspective 0: dst0 = src0 + wTo0 - wFrom0
	VMOVDQU (BX)(R10*1), Y0
	VPADDW  (DI)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1: dst1 = src1 + wTo1 - wFrom1
	VMOVDQU (DX)(R10*1), Y1
	VPADDW  (R9)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VMOVDQU Y1, (CX)(R10*1)

	ADDQ $32, R10
	CMPQ R10, $1024
	JB move3_loop_512

	VZEROUPPER
	RET

TEXT ·captureAVX2_512_3op(SB), NOSPLIT, $0-80
	MOVQ dst0+0(FP), AX
	MOVQ src0+8(FP), BX
	MOVQ dst1+16(FP), CX
	MOVQ src1+24(FP), DX

	MOVQ wTo0+32(FP), SI
	MOVQ wFrom0+40(FP), DI
	MOVQ wCap0+48(FP), R8

	MOVQ wTo1+56(FP), R9
	MOVQ wFrom1+64(FP), R10
	MOVQ wCap1+72(FP), R11

	XORQ R12, R12

capture3_loop_512:
	// Perspective 0: dst0 = src0 + wTo0 - wFrom0 - wCap0
	VMOVDQU (BX)(R12*1), Y0
	VPADDW  (SI)(R12*1), Y0, Y0
	VPSUBW  (DI)(R12*1), Y0, Y0
	VPSUBW  (R8)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1: dst1 = src1 + wTo1 - wFrom1 - wCap1
	VMOVDQU (DX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VPSUBW  (R11)(R12*1), Y1, Y1
	VMOVDQU Y1, (CX)(R12*1)

	ADDQ $32, R12
	CMPQ R12, $1024
	JB capture3_loop_512

	VZEROUPPER
	RET

TEXT ·castleAVX2_512_3op(SB), NOSPLIT, $0-96
	MOVQ dst0+0(FP), AX
	MOVQ src0+8(FP), BX
	MOVQ dst1+16(FP), CX
	MOVQ src1+24(FP), DX

	MOVQ wKFrom0+32(FP), SI
	MOVQ wKTo0+40(FP), DI
	MOVQ wRFrom0+48(FP), R8
	MOVQ wRTo0+56(FP), R9

	MOVQ wKFrom1+64(FP), R10
	MOVQ wKTo1+72(FP), R11
	MOVQ wRFrom1+80(FP), R12
	MOVQ wRTo1+88(FP), R13

	XORQ R14, R14

castle3_loop_512:
	// Perspective 0: dst0 = src0 + kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R14*1), Y0
	VPADDW  (DI)(R14*1), Y0, Y0
	VPSUBW  (SI)(R14*1), Y0, Y0
	VPADDW  (R9)(R14*1), Y0, Y0
	VPSUBW  (R8)(R14*1), Y0, Y0
	VMOVDQU Y0, (AX)(R14*1)

	// Perspective 1: dst1 = src1 + kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (DX)(R14*1), Y1
	VPADDW  (R11)(R14*1), Y1, Y1
	VPSUBW  (R10)(R14*1), Y1, Y1
	VPADDW  (R13)(R14*1), Y1, Y1
	VPSUBW  (R12)(R14*1), Y1, Y1
	VMOVDQU Y1, (CX)(R14*1)

	ADDQ $32, R14

	// 512 int16 neurons = 1024 bytes
	CMPQ R14, $1024
	JB castle3_loop_512

	VZEROUPPER
	RET

// functions for 768 hidden neurons

TEXT ·captureAVX2_768(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

capture_768_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

	// loop limit is 1536 = 2 * 768
	ADDQ $32, R10
	CMPQ R10, $1536
	JB capture_768_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_768(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_768:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $1536
	JB move_loop_768

	VZEROUPPER
	RET

TEXT ·castleAVX2_768(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_768_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12

	// 768 int16 neurons = 1536 bytes
	CMPQ R12, $1536
	JB castle_768_loop

	VZEROUPPER
	RET

// EVAL

// func getEvalAVX2_64(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// 64 int16 neurons = 128 bytes.
// Each loop iteration processes 16 neurons = 32 bytes.
TEXT ·getEvalAVX2_64(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_64_loop:
	// Perspective 0.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// Clamp accumulator values to [0, 255].
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// Perspective 1.
	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $128
	JB geteval_64_loop

	// Reduce eight int32 lanes to one.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

	// func getEvalAVX2_128(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// 128 int16 neurons = 256 bytes.
// Each loop iteration processes 16 neurons = 32 bytes.
TEXT ·getEvalAVX2_128(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_128_loop:
	// Perspective 0.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// Clamp accumulator values to [0, 255].
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// Perspective 1.
	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $256
	JB geteval_128_loop

	// Reduce eight int32 lanes to one.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_256(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// For every neuron:
//
//     v = clamp(acc, 0, 255)
//     sum += v * v * weight
//
// 256 int16 neurons = 512 bytes.
// Each loop iteration processes 16 neurons.
TEXT ·getEvalAVX2_256(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y8 accumulates eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_256_loop:
	// ------------------------------------------------------------
	// Perspective 0
	// ------------------------------------------------------------

	// Load 16 accumulator values and 16 signed weights.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// SCReLU clipping: max(x, 0), then min(x, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight int16 values -> eight int32 values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	// v * v * weight.
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $512
	JB geteval_256_loop

	// Horizontally reduce eight int32 lanes to one int32.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	// [a,b,c,d] + [c,d,a,b]
	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	// [a,b,...] + [b,a,...]
	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	// Store the low int32 result.
	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_384(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
TEXT ·getEvalAVX2_384(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = zero
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values containing 255
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y8 = int32 accumulation
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

eval384_loop:
	// ------------------------------------------------------------
	// Perspective 0, lower eight neurons
	// ------------------------------------------------------------

	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// clamp accumulator to [0, 255]
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// lower 8 x int16 -> int32
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	// x² * weight
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2

	VPADDD Y2, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 0, upper eight neurons
	// ------------------------------------------------------------

	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4

	VPADDD Y4, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1, lower eight neurons
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2

	VPADDD Y2, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1, upper eight neurons
	// ------------------------------------------------------------

	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4

	VPADDD Y4, Y8, Y8

	// 16 int16 neurons = 32 bytes
	ADDQ $32, R9

	// 384 int16 neurons = 768 bytes
	CMPQ R9, $768
	JL eval384_loop

	// Horizontal sum of eight int32 lanes in Y8.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_512(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
TEXT ·getEvalAVX2_512(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y13 = sixteen int16 values equal to 1 for ceil calculation.
	MOVL $1, R8
	VMOVD R8, X13
	VPBROADCASTW X13, Y13

	// Y8 accumulates eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

eval512_loop:
	// ------------------------------------------------------------
	// Perspective 0 (all 16 neurons in one go)
	// ------------------------------------------------------------

	// Load 16 accumulator values and 16 signed weights.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// SCReLU clipping: v = clamp(acc, 0, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Y2 = floor(v / 2)
	// Y3 = ceil(v / 2) = (v + 1) / 2
	VPSRLW $1, Y0, Y2
	VPADDW Y13, Y0, Y3
	VPSRLW $1, Y3, Y3

	// Y2 = v * floor(v / 2)
	// Y3 = v * ceil(v / 2)
	VPMULLW Y0, Y2, Y2
	VPMULLW Y0, Y3, Y3

	// Multiply partial products by output weights and horizontally add pairs.
	VPMADDWD Y1, Y2, Y2
	VPMADDWD Y1, Y3, Y3

	// Accumulate into Y8
	VPADDD Y2, Y8, Y8
	VPADDD Y3, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1 (all 16 neurons in one go)
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	// SCReLU clipping: v = clamp(acc, 0, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Y2 = floor(v / 2)
	// Y3 = ceil(v / 2) = (v + 1) / 2
	VPSRLW $1, Y0, Y2
	VPADDW Y13, Y0, Y3
	VPSRLW $1, Y3, Y3

	// Y2 = v * floor(v / 2)
	// Y3 = v * ceil(v / 2)
	VPMULLW Y0, Y2, Y2
	VPMULLW Y0, Y3, Y3

	// Multiply partial products by output weights and horizontally add pairs.
	VPMADDWD Y1, Y2, Y2
	VPMADDWD Y1, Y3, Y3

	// Accumulate into Y8
	VPADDD Y2, Y8, Y8
	VPADDD Y3, Y8, Y8

	// 16 int16 neurons = 32 bytes
	ADDQ $32, R9

	// 512 int16 neurons = 1024 bytes
	CMPQ R9, $1024
	JL eval512_loop

	// Horizontal sum of eight int32 lanes in Y8.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_768(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// For every neuron:
//
//     v = clamp(acc, 0, 255)
//     sum += v * v * weight
//
// Lizard-style exact split:
//
//     v² = v * floor(v/2) + v * ceil(v/2)
//
// 768 int16 neurons = 1536 bytes.
// Each loop iteration processes 16 neurons per perspective.
TEXT ·getEvalAVX2_768(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y13 = sixteen int16 values equal to 1 for ceil calculation.
	MOVL $1, R8
	VMOVD R8, X13
	VPBROADCASTW X13, Y13

	// Y8 accumulates eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

eval768_loop:
	// ------------------------------------------------------------
	// Perspective 0
	// ------------------------------------------------------------

	// Load 16 accumulator values and 16 signed weights.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// SCReLU clipping: v = clamp(acc, 0, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Y2 = floor(v / 2)
	// Y3 = ceil(v / 2) = (v + 1) / 2
	VPSRLW $1, Y0, Y2
	VPADDW Y13, Y0, Y3
	VPSRLW $1, Y3, Y3

	// Y2 = v * floor(v / 2)
	// Y3 = v * ceil(v / 2)
	VPMULLW Y0, Y2, Y2
	VPMULLW Y0, Y3, Y3

	// Multiply partial products by output weights and horizontally add pairs.
	VPMADDWD Y1, Y2, Y2
	VPMADDWD Y1, Y3, Y3

	// Accumulate into Y8.
	VPADDD Y2, Y8, Y8
	VPADDD Y3, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	// SCReLU clipping: v = clamp(acc, 0, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Y2 = floor(v / 2)
	// Y3 = ceil(v / 2) = (v + 1) / 2
	VPSRLW $1, Y0, Y2
	VPADDW Y13, Y0, Y3
	VPSRLW $1, Y3, Y3

	// Y2 = v * floor(v / 2)
	// Y3 = v * ceil(v / 2)
	VPMULLW Y0, Y2, Y2
	VPMULLW Y0, Y3, Y3

	// Multiply partial products by output weights and horizontally add pairs.
	VPMADDWD Y1, Y2, Y2
	VPMADDWD Y1, Y3, Y3

	// Accumulate into Y8.
	VPADDD Y2, Y8, Y8
	VPADDD Y3, Y8, Y8

	// 16 int16 neurons = 32 bytes.
	ADDQ $32, R9

	// 768 int16 neurons = 1536 bytes.
	CMPQ R9, $1536
	JL eval768_loop

	// Horizontal sum of eight int32 lanes in Y8.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET
	