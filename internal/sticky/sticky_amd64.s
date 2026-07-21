//go:build go1.26 && amd64 && cgo && !O0

#include "textflag.h"

// func currentm() uintptr
// Returns g.m by dereferencing g.m at offset 48 (R14 = g on amd64, Go 1.17+).
// Identical to internal/threadcheck.currentm — the proven house pattern.
TEXT ·currentm(SB),NOSPLIT,$0-8
	MOVQ	48(R14), AX
	MOVQ	AX, ret+0(FP)
	RET

// func releaseStickyP()
//
// UNKNOWN #3 (part a): frame-boundary teardown. Hand the sticky P back to the
// scheduler and park the sticky goroutine so stop-the-world can proceed during
// the inter-frame engine work. This must mirror what runtime.entersyscall does
// on a normal cgocallback return, done here explicitly because the sticky fast
// path skipped it. The correct sequence resolves and calls runtime internals
// (entersyscall / a park) — it is NOT yet written, because getting it wrong
// corrupts the scheduler and the only way to confirm is to run it.
//
// Left as an explicit RET stub so the package compiles and the Go-side control
// flow (EndFrame → heldP → releaseStickyP) can be reviewed and unit-exercised
// with the fast path disabled (gcWaitingImpl returns true). TODO(validate):
// implement against resolved runtime.entersyscall for go1.26.
TEXT ·releaseStickyP(SB),NOSPLIT,$0-0
	RET

// The C-ABI entry thunk that replaces extension_instance_called as the engine's
// call_virtual_with_data_func is INTENTIONALLY NOT WRITTEN HERE YET. It is the
// core that needs runtime validation, and its shape is:
//
//   thunk(instance=RDI, name=RSI, userdata=RDX, args=RCX, ret=R8):
//     if sticky.Enabled && heldP() && !gcWaiting():
//         // FAST: g+P already live from earlier this frame.
//         // switch to the sticky goroutine stack, call Dispatch(instance,
//         // userdata, ret, args), switch back, return — NO exitsyscall.
//     else:
//         // COLD: establish g+P via the proven direct-cgocallback path
//         // (resolved runtime.cgocallback — see runtime.link
//         // api/call/internal/native.MakeCgocallbackDirect, already tested),
//         // but DO NOT release the P on return: leave it sticky for the rest
//         // of the frame. Record that we now hold it.
//
// UNKNOWN #3 (part b): the FAST-path stack switch that reuses a held P without
// re-entering the scheduler. This is a custom, lean cgocallback variant. Until
// it is written and validated, classdb registration must keep using the stock
// extension_instance_called entry (see DESIGN.md, "Wiring").
