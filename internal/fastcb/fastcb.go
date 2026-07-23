// Package fastcb is the bridge to the resident-callback runtime patch (the
// "fastcb" cgocall.go overlay bundled with the gd CLI, canonical copy in
// cmd/gd/internal/builder/bundled/fastcb). Resident-callback mode keeps the
// engine's thread _Grunning with its P wired across C->Go callbacks, removing
// the per-callback exitsyscall/reentersyscall GC coordination.
//
// The patched runtime publishes the entry PCs of its three hooks into
// runtimePCs on the first C->Go callback. When the binary was built without
// the overlay the slots stay zero and everything here no-ops, so this package
// is always compiled — no build tags and no app wiring required. The overlay
// is applied automatically by `gd build` on verified platforms.
package fastcb

import (
	"runtime"
	"unsafe"
)

// runtimePCs holds the entry PCs of runtime.fastcbSetResident,
// fastcbClearResident, fastcbYield and fastcbCallC, in that order,
// written by the patched runtime on the first C->Go callback. How the
// var is wired depends on the toolchain — see pcs_stock.go (the gd
// CLI's cgocall.go overlay pushes into our declaration) and
// pcs_gd.go (the compiler.gd fork carries the machinery in its own
// runtime and sets the `gd` build tag; we alias its published array).

type funcval struct{ fn uintptr }

// callPC invokes a zero-argument, zero-result ABIInternal function by entry
// PC, by materialising a func value around it (a Go func value is a pointer
// to a funcval whose first word is the code pointer; top-level functions
// ignore the closure context register).
func callPC(pc uintptr) {
	fv := funcval{fn: pc}
	fp := unsafe.Pointer(&fv)
	(*(*func())(unsafe.Pointer(&fp)))()
	runtime.KeepAlive(&fv)
}

// Available reports whether the resident-callback runtime patch is present in
// this binary (and at least one C->Go callback has occurred, which is always
// true by the time the engine is running).
func Available() bool { return runtimePCs[0] != 0 }

// PCs returns the hook entry PCs published by the patched runtime (zero when
// the patch is absent): SetResident, ClearResident, Yield, CallC, in that
// order. Diagnostic use (tests verify they resolve to the runtime.fastcb*
// hooks).
func PCs() [4]uintptr { return runtimePCs }

// resident tracks whether residency is currently engaged. Written by
// SetResident/ClearResident and read by Resident, all on the main thread
// (residency is a main-thread-only mode), so no synchronisation is needed.
var resident bool

// Resident reports whether resident-callback mode is currently engaged.
// Main thread only. When true, outbound engine calls should go through CallC
// (with non-moving staging memory) so callbacks nested inside them take the
// resident fast path.
func Resident() bool { return resident }

// SetResident marks the current m as hosting resident callbacks and reports
// whether it did. The caller must be locked to its OS thread.
func SetResident() bool {
	if runtimePCs[0] == 0 {
		return false
	}
	callPC(runtimePCs[0])
	resident = true
	return true
}

// ClearResident ends residency for the current m. No-op without the patch.
func ClearResident() {
	resident = false
	if runtimePCs[1] != 0 {
		callPC(runtimePCs[1])
	}
}

// Yield asks the current resident callback chain to drop to _Gsyscall when
// the innermost callback returns to C, so the GC and scheduler can interact
// with this thread during the engine's idle gap; the next callback re-engages
// residency through the stock path. Call once per frame. No-op without the
// patch.
func Yield() {
	if runtimePCs[2] != 0 {
		callPC(runtimePCs[2])
	}
}

// CallC invokes the C function fn(arg) on the system stack via the patched
// runtime's fastcbCallC: the goroutine stays _Grunning with its P wired, so
// engine->Go callbacks nested inside fn take the resident fast path instead
// of a full cgocallback transition. The caller must be the resident goroutine
// (Resident() true implies the patch is present).
//
// arg must NOT point into any goroutine stack: a nested callback can grow and
// therefore move the stack, and nothing adjusts the raw pointer C holds (cgo
// shims survive this only via their _cgo_topofstack adjustment, which does not
// help pointers the C or engine code holds across the nested callback). Pass
// globals or heap memory only.
func CallC(fn, arg unsafe.Pointer) int32 {
	fv := funcval{fn: runtimePCs[3]}
	fp := unsafe.Pointer(&fv)
	errno := (*(*func(fn, arg unsafe.Pointer) int32)(unsafe.Pointer(&fp)))(fn, arg)
	runtime.KeepAlive(&fv)
	return errno
}
