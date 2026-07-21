//go:build go1.26 && amd64 && cgo && !O0 && musl

package gd

// #include "../gd.h"
import "C"

import (
	"unsafe"

	"graphics.gd/internal/sticky"
)

// Arm the sticky-P fast path for virtual dispatch by pointing the engine's
// call_virtual_with_data_func (via gd.c's gd_sticky_call_virtual indirection)
// at the sticky entry thunk. EntryAddr returns 0 when sticky.Enabled is false,
// leaving the stock cgocallback path in place. Runs before any class registers
// (package gd initialises before classdb), and the C wrapper reads the global
// per-call, so ordering is not load-bearing.
//
// MUSL-GATED (2026-07-19): the no-switch fast path reloads g and runs the
// virtual dispatch on the current (g0) stack WITHOUT the cgocallback stack
// switch. That is only safe when the engine's C code is itself running on g0's
// stack — i.e. the musl/archive static build, where graphics.gd owns the main
// loop and enters the engine via cgocall (`for !Iteration()`). In a c-shared
// build the engine callback arrives on a foreign native thread stack far
// outside g0's bounds; the first non-nosplit prologue (or any outbound cgo such
// as Node2D.SetPosition's ring flush) then trips "morestack on g0" / an
// entersyscall fault. So arm ONLY under the musl tag; c-shared keeps the stock
// cgocallback path. See internal/sticky/DESIGN.md and the spritebench crash
// analysis in the graphics-gd-spritebench-optimization memory.
func init() {
	if addr := sticky.EntryAddr(); addr != 0 {
		C.gd_sticky_call_virtual = unsafe.Pointer(addr)
	}
}

// ArmDiagnostics reports (EntryAddr from sticky, the C global as Go sees
// it) so a test can localise whether arming took. Debug helper.
func ArmDiagnostics() (entryAddr, cGlobal uintptr) {
	return sticky.EntryAddr(), uintptr(C.gd_sticky_call_virtual)
}
