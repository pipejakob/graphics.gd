//go:build go1.26 && (amd64 || arm64) && cgo && !O0

package gd

// The C symbols are defined in the root package's gd.c and linked across the
// whole binary; declaring them extern in the cgo preamble below lets this
// package reach them.

// extern int gd_frame_active;
// extern void *gd_sticky_call_virtual;
// extern void *gd_iterate_g0_addr(void);
import "C"

import (
	"unsafe"

	"graphics.gd/internal/pclntab"
)

type iterArgs struct {
	obj    uintptr
	method uintptr
	shape  uint64
	args   unsafe.Pointer
	result uint64
}

// StickyFastPathArmed reports whether the sticky fast path is armed.
func StickyFastPathArmed() bool { return C.gd_sticky_call_virtual != nil }

// SetFrameActiveForTest toggles the frame-active gate. Test-only: lets a test
// exercise the fast-path dispatch for a NON-allocating virtual without driving
// a real held frame (the P is not actually held, so the virtual must not
// allocate).
func SetFrameActiveForTest(v bool) {
	if v {
		C.gd_frame_active = 1
	} else {
		C.gd_frame_active = 0
	}
}

// IterationHoldingP runs one engine main-loop iteration (a bool-returning
// unsafe call, shape passed by the caller) via runtime.asmcgocall — which runs
// a C function on the g0 stack WITHOUT entersyscall — so the P is held across
// the frame, marking it active so the per-node virtual callbacks nested inside
// take the no-transition fast path and can still allocate. The asmcgocall
// entry PC comes from the pclntab (see graphics.gd/internal/pclntab), not a
// //go:linkname pull of the runtime symbol. Returns whether the engine is
// quitting. Main thread only.
func IterationHoldingP(obj, method, shape uintptr) bool {
	var empty struct{}
	a := iterArgs{obj: obj, method: method, shape: uint64(shape), args: unsafe.Pointer(&empty)}
	C.gd_frame_active = 1
	pclntab.Asmcgocall(C.gd_iterate_g0_addr(), unsafe.Pointer(&a))
	C.gd_frame_active = 0
	return a.result != 0
}
