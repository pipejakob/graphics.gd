//go:build go1.26 && amd64 && cgo && !O0

// Package sticky implements "sticky-P" dispatch for engine→Go virtual calls
// (the inbound _process/_physics_process path). Instead of paying a full
// runtime.cgocallback transition (exitsyscall to acquire a P on entry,
// entersyscall to release it on return) on every one of the ~20k per-frame
// virtual calls, the engine thread acquires a P ONCE at the start of a frame's
// virtual-call batch and keeps it live across the intervening engine-C gaps,
// so subsequent calls re-enter Go without the transition. The P is released at
// the frame boundary (see [EndFrame]) so stop-the-world is never blocked for
// longer than one frame, and each call checks gcWaiting so a pending STW is
// honoured within one call-gap.
//
// This is the inbound mirror of internal/jumponly (which does the same for the
// OUTBOUND direction — direct ptrcall on the goroutine stack, no cgo). It
// deliberately trades a bounded GC-latency delay for throughput; see DESIGN.md.
//
// SAFETY / VALIDATION STATUS: the C→Go entry itself is the proven
// direct-cgocallback path (resolved runtime.cgocallback, see runtime.link
// api/call/internal/native). The STICKY state machine layered on top — holding
// the P across gaps, reconciling when STW steals it, frame teardown — is NOT
// yet validated at runtime. It depends on three runtime-version-specific facts
// isolated below (mOffsetP, gcWaiting, the reuse stack-switch in the .s file).
// Validate with: go test -run x -bench BenchmarkVirtualCallback ./internal/
// plus the full suite, before trusting for all classes.
package sticky

import (
	"sync/atomic"
	"unsafe"
)

// calls counts how many virtual dispatches went through the fast-path thunk
// (as opposed to the stock cgocallback path). A single LOCK XADD — safe in the
// thunk's no-transition context (no alloc, no P needed). Used by tests to prove
// the fast path was actually exercised.
var calls atomic.Uint64

// Calls returns the number of virtual dispatches serviced by the fast path.
func Calls() uint64 { return calls.Load() }

// Enabled turns the no-switch sticky path on for ALL classes.
//
// DISABLED (2026-07-19) after direct spritebench measurement on BOTH targets:
// the no-switch path reloads g and runs the virtual dispatch on the current g0
// stack without cgocallback's stack switch. That is fine for a pure LEAF virtual
// (the gd-test probe GetMinimumSize, which returns a constant), but every real
// virtual makes OUTBOUND cgo calls — e.g. Node2D.SetPosition flushes the call
// ring via _Cfunc_gd_ring_flush → cgocall → entersyscall — and that faults with
// "morestack on g0" on the small (~33KB) g0 stack, on musl-static AND c-shared
// alike. So the reload-g technique cannot carry a real workload on either
// target; the stock cgocallback path (which switches to a real goroutine stack)
// is required. Left false; the thunk/scaffolding stays as documented research.
// See the crash analysis in the graphics-gd-spritebench-optimization memory and
// [[runtime-link-callback-bench]].
var Enabled = false

// Dispatch (the Go-side virtual-method dispatch) lives in dispatch.go, which
// compiles on arm64 too — the runtime-carried fastcb entry thunk uses it on
// both architectures.

// ---- UNKNOWN #1: m.p field offset ---------------------------------------
// Offset of m.p within runtime.m, for the Go toolchain this is built with.
// g.m is at offset 48 (see internal/threadcheck, stable); m.p must be
// confirmed against the runtime source / a debug print for the exact go1.26
// build. mp() (sticky_amd64.s) reads *(g.m + mOffsetP).
//
// CONFIRM before trusting: `go doc -src runtime.m` field order, or print
// unsafe.Offsetof on a copied struct. Placeholder value MUST be verified.
const mOffsetP = 0 // TODO(validate): real offset of m.p in go1.26

// heldP reports whether this m currently owns a P (m.p != 0), i.e. we are in a
// sticky region and can re-enter Go without exitsyscall.
func heldP() bool { return mp() != 0 }

// mp returns g.m.p (0 if none). currentm() (asm, the proven threadcheck
// pattern) returns g.m; the m.p deref uses mOffsetP so the sole version-
// specific constant stays in Go, not asm.
func mp() uintptr {
	m := currentm()
	if m == 0 {
		return 0
	}
	return *(*uintptr)(unsafe.Pointer(m + mOffsetP))
}

// currentm returns g.m (the OS-thread struct) via the g register. See
// sticky_amd64.s (identical to internal/threadcheck.currentm).
func currentm() uintptr

// ---- UNKNOWN #2: STW pending ---------------------------------------------
// gcWaiting reports whether a stop-the-world is pending (runtime.sched.gcwaiting
// is set). If so, the sticky fast path must NOT be taken — we do a real
// transition so the runtime can take the P and complete STW, bounding the
// delay to one call-gap. sched is a runtime-internal global; the exact access
// (linkname helper vs. asm offset into runtime.sched) must be pinned for
// go1.26. Returning true here is always SAFE (falls back to stock path); a
// false-negative is the dangerous case, so default conservative.
//
//go:noinline
func gcWaiting() bool {
	return gcWaitingImpl()
}

// gcWaitingImpl is the version-specific probe. TODO(validate): implement via a
// //go:linkname to a runtime accessor or an asm read of sched.gcwaiting at the
// confirmed offset. Until pinned, return true so we ALWAYS take the safe stock
// path (correct but slow — proves the wiring before enabling the fast path).
func gcWaitingImpl() bool { return true }

// active reports whether the cold-path thunk established a sticky region (and
// thus is holding a P) during the current frame. It is set ONLY by that thunk;
// while the entry thunk is unwired (the current scaffold state) it stays false,
// so [EndFrame] is a guaranteed no-op and cannot act on the still-placeholder
// m.p offset. Main-thread only, so no synchronisation is needed.
var active bool

// EndFrame releases the sticky P at the frame boundary. It must be called once
// per frame from the every-frame hook (gd_on_every_frame → On.MainLoop.
// EveryFrame), BEFORE that hook's own GC/flush work (which allocates and must
// not run while we hold a sticky P). Releasing here bounds any STW delay to at
// most one frame and lets the long inter-frame engine work run with the P
// released so GC proceeds normally. Inert until the entry thunk sets active.
func EndFrame() {
	if !active {
		return
	}
	active = false
	if heldP() {
		releaseStickyP()
	}
}

// releaseStickyP does a real entersyscall-equivalent, handing the P back to the
// scheduler and parking the sticky goroutine. Implemented in sticky_amd64.s
// against the resolved runtime entrypoints. TODO(validate).
func releaseStickyP()

// stickyEntry is the C-ABI thunk (entry_amd64.s) registered as the engine's
// call_virtual_with_data_func when the fast path is armed.
func stickyEntry()

// stickyEntryAddr returns stickyEntry's raw entry address for registration.
func stickyEntryAddr() uintptr

// EntryAddr is the C-ABI address to register as call_virtual_with_data_func to
// route virtual dispatch through the fast path. Returns 0 when the fast path is
// disabled, so callers keep the stock entry.
func EntryAddr() uintptr {
	if !Enabled {
		return 0
	}
	return stickyEntryAddr()
}

// GenericDispatch is set by the startup package to the tag-switched dispatcher
// for every non-frame engine->Go callback (unpacks the frame, calls On.X). Held
// as an indirect func to avoid an import cycle (sticky must not import startup).
var GenericDispatch func(tag, frame uintptr)

// stickyGeneric is the generic C-ABI fast entry (generic_amd64.s).
func stickyGeneric()

// stickyGenericAddr returns stickyGeneric's raw C-ABI entry address.
func stickyGenericAddr() uintptr

// GenericEntryAddr is the C-ABI address the gd.c wrappers call to route a
// callback through the fast path during a held frame. 0 when disabled.
func GenericEntryAddr() uintptr {
	if !Enabled {
		return 0
	}
	return stickyGenericAddr()
}

// stickyGenericDispatch is called by the generic thunk with no cgocallback
// transition; it forwards to the registered GenericDispatch. Tiny frame, no
// defer, so it is unlikely to trip morestack on the current stack.
func stickyGenericDispatch(tag, frame uintptr) {
	calls.Add(1)
	if GenericDispatch != nil {
		GenericDispatch(tag, frame)
	}
}

// stickyGoEntry is the Go side of the fast-path thunk: it runs the virtual
// dispatch with NO cgocallback transition. Kept minimal (tiny frame, no defer)
// so it is unlikely to trip morestack on the current stack. If the dispatch
// path is not alloc-free / the P is unavailable, this is where it will fault —
// which is exactly what the gd-test experiment measures.
func stickyGoEntry(instance, name, userdata, args, ret uintptr) {
	calls.Add(1)
	if Dispatch != nil {
		Dispatch(instance, userdata, ret, args)
	}
}
