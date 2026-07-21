#include "textflag.h"
#include "funcdata.h"

// func callAsmcgocall(pc uintptr, fn, arg unsafe.Pointer) int32
//
// Calls runtime.asmcgocall(fn, arg) at entry pc. The callee is either the
// ABI0 assembly definition (arguments at 0(SP)/8(SP), result at 16(SP)) or a
// linker-generated ABIInternal wrapper (fn in AX, arg in BX, result in AX);
// the pclntab names both "runtime.asmcgocall", so the arguments are staged
// for both conventions and the result slot is pre-filled with a sentinel: if
// the callee wrote the slot (ABI0) take it, otherwise take AX (ABIInternal).
//
// Stack safety: the ABI0 definition is NOSPLIT, so the stack cannot move
// between the stores below and the callee reading its stack arguments; the
// ABIInternal wrapper only reads the register copies, whose pointers the
// runtime tracks and adjusts across any stack growth in its prologue.
//
// NO_LOCAL_POINTERS declares an empty locals map — without one, a stack copy
// (a callback nested in the C call growing the goroutine stack) throws
// "missing stackmap" when it walks this frame. Verbatim (unadjusted) copying
// of the slots is fine: the callee consumed them at entry and never re-reads
// them after a move, arg's pointee escapes to the heap (bodyless assembly
// declarations leak their pointer arguments) so the C side never holds a
// stack address, and liveness is the caller's, exactly as it was when
// runtime.asmcgocall was reached by //go:linkname.
TEXT ·callAsmcgocall(SB), NOSPLIT, $24-28
	NO_LOCAL_POINTERS
	MOVQ	fn+8(FP), AX
	MOVQ	arg+16(FP), BX
	MOVQ	AX, 0(SP)
	MOVQ	BX, 8(SP)
	MOVL	$0x5EFACADE, 16(SP)
	MOVQ	pc+0(FP), CX
	CALL	CX
	MOVL	16(SP), CX
	CMPL	CX, $0x5EFACADE
	JNE	useslot
	MOVL	AX, ret+24(FP)
	RET
useslot:
	MOVL	CX, ret+24(FP)
	RET
