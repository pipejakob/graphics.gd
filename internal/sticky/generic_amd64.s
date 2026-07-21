//go:build go1.26 && amd64 && cgo && !O0

#include "textflag.h"

#define get_tls(r)	MOVQ TLS, r
#define g(r)		0(r)(TLS*1)

// stickyGeneric is the ONE generic C-ABI fast entry for every non-frame
// engine->Go callback during a held-P frame:
//
//   void stickyGeneric(uintptr tag, void *frame)
//
// The per-callback C wrapper (gd.c) packs its arguments into a frame struct and
// calls this with a tag identifying the callback; stickyGenericDispatch unpacks
// by tag and invokes the existing On.X handler, writing any result back into the
// frame. It reloads g and calls Go WITHOUT a cgocallback transition, so it can
// run under a held P (asmcgocall'd Iteration) without the exitsyscall that stock
// cgocallback would do — which is what breaks the held frame.
//
// Args passed on the stack (ABI0). C callee-saved regs saved/restored.
TEXT ·stickyGeneric(SB), NOSPLIT, $64-0
	MOVQ	BX, 16(SP)
	MOVQ	BP, 24(SP)
	MOVQ	R12, 32(SP)
	MOVQ	R13, 40(SP)
	MOVQ	R14, 48(SP)
	MOVQ	R15, 56(SP)
	MOVQ	DI, 0(SP) // tag
	MOVQ	SI, 8(SP) // frame
	get_tls(R15)
	MOVQ	g(R15), R14
	CALL	·stickyGenericDispatch(SB)
	MOVQ	16(SP), BX
	MOVQ	24(SP), BP
	MOVQ	32(SP), R12
	MOVQ	40(SP), R13
	MOVQ	48(SP), R14
	MOVQ	56(SP), R15
	RET

// func stickyGenericAddr() uintptr — the C-ABI entry for the wrappers to call.
TEXT ·stickyGenericAddr(SB), NOSPLIT, $0-8
	LEAQ	·stickyGeneric(SB), AX
	MOVQ	AX, ret+0(FP)
	RET
