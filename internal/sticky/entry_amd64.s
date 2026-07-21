//go:build go1.26 && amd64 && cgo && !O0

#include "textflag.h"

// get_tls / g inlined from runtime/go_tls.h (amd64) — that header is not on the
// public asm include path, but the TLS pseudo-register it uses is.
#define get_tls(r)	MOVQ TLS, r
#define g(r)		0(r)(TLS*1)

// stickyEntry is a System V C-ABI function registered as the engine's
// call_virtual_with_data_func:
//
//   void stickyEntry(instance=DI, name=SI, userdata=DX, args=CX, ret=R8)
//
// It reloads g (the engine's C code between calls clobbers R14, Go's g
// register), shuffles the five SysV integer args into Go's ABIInternal argument
// registers, and calls stickyGoEntry DIRECTLY — no crosscall2, no
// runtime.cgocallback, no exitsyscall/entersyscall transition. It runs on the
// current (engine C) stack, so the handler must be shallow (a morestack would
// fault). This is the aggressive no-stack-switch fast path — an EXPERIMENT to
// see whether the dispatch (made alloc-free by the itab cache) survives musl's
// threading, where the callback arrives on g0 with the P in syscall state.
// Frame layout: args for the ABI0 Go call at 0..32, saved C callee-saved regs
// at 40..87. Plain `CALL ·stickyGoEntry(SB)` hits the ABI0 wrapper, which reads
// its args from the stack (0(FP)..) — so we pass them there, not in registers.
TEXT ·stickyEntry(SB), NOSPLIT, $88-0
	// Save the C ABI callee-saved registers Go's ABI0 wrapper may clobber
	// (including R14, which C uses as a general reg but Go overwrites with g).
	MOVQ	BX, 40(SP)
	MOVQ	BP, 48(SP)
	MOVQ	R12, 56(SP)
	MOVQ	R13, 64(SP)
	MOVQ	R14, 72(SP)
	MOVQ	R15, 80(SP)
	// Marshal the five SysV args into the ABI0 stack slots.
	MOVQ	DI, 0(SP)   // instance
	MOVQ	SI, 8(SP)   // name
	MOVQ	DX, 16(SP)  // userdata
	MOVQ	CX, 24(SP)  // args
	MOVQ	R8, 32(SP)  // ret
	// Reload g into R14 from TLS (engine C clobbered it) for the Go callee.
	get_tls(R15)
	MOVQ	g(R15), R14
	CALL	·stickyGoEntry(SB)
	MOVQ	40(SP), BX
	MOVQ	48(SP), BP
	MOVQ	56(SP), R12
	MOVQ	64(SP), R13
	MOVQ	72(SP), R14
	MOVQ	80(SP), R15
	RET

// func stickyEntryAddr() uintptr — the raw C-ABI entry address to register.
TEXT ·stickyEntryAddr(SB), NOSPLIT, $0-8
	LEAQ	·stickyEntry(SB), AX
	MOVQ	AX, ret+0(FP)
	RET
