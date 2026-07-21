#include "textflag.h"
#include "funcdata.h"

// func callAsmcgocall(pc uintptr, fn, arg unsafe.Pointer) int32
//
// Calls runtime.asmcgocall(fn, arg) at entry pc. The callee is either the
// ABI0 assembly definition (arguments at 8(RSP)/16(RSP), result at 24(RSP) —
// note the one-slot skew from amd64: the callee's FP reaches over its own
// saved-LR slot) or a linker-generated ABIInternal wrapper (fn in R0, arg in
// R1, result in R0); the pclntab names both "runtime.asmcgocall", so the
// arguments are staged for both conventions and the result slot is pre-filled
// with a sentinel: if the callee wrote the slot (ABI0) take it, otherwise
// take R0 (ABIInternal).
//
// Stack safety and NO_LOCAL_POINTERS: same reasoning as asmcgocall_amd64.s —
// the ABI0 definition is NOSPLIT and consumes the slots at entry, the
// ABIInternal wrapper only reads the register copies, and arg's pointee
// escapes to the heap, so verbatim copying of this frame is fine.
TEXT ·callAsmcgocall(SB), NOSPLIT, $32-28
	NO_LOCAL_POINTERS
	MOVD	fn+8(FP), R0
	MOVD	arg+16(FP), R1
	STP	(R0, R1), 8(RSP)
	MOVW	$0x5EFACADE, R2
	MOVW	R2, 24(RSP)
	MOVD	pc+0(FP), R3
	CALL	(R3)
	MOVW	24(RSP), R2
	MOVW	$0x5EFACADE, R3
	CMPW	R3, R2
	BNE	useslot
	MOVW	R0, ret+24(FP)
	RET
useslot:
	MOVW	R2, ret+24(FP)
	RET
