//go:build go1.26 && amd64 && cgo && !O0

package gd

// #include "../gd.h"
import "C"

import (
	"os"
	"unsafe"

	"graphics.gd/internal/fastcb"
	"graphics.gd/internal/sticky"
)

// Arm the direct virtual-call entry: a runtime-carried C-ABI thunk (the
// compiler.gd fork's runtime.fastcbentry, or the gd CLI overlay's
// runtime.fastcbentryfast) that, while resident-callback mode is engaged,
// enters Go with the engine's virtual-call arguments still in registers —
// no crosscall2, no cgocallback, no cgo export shim. On the fork the
// install callback runs immediately from this init; on stock toolchains
// the overlay's hooks appear only at the first C->Go callback, so the
// install is deferred until residency setup (see fastcb.ArmEntry). On
// toolchains and platforms without a thunk nothing is ever installed and
// the stock dispatch stays in place.
//
// The thunk self-checks residency on every call and tail-jumps to the stock
// entry (extension_instance_called, via gd_stock_virtual_entry) whenever the
// fast path is not safe — foreign threads, the yielded gap between frames,
// or before residency engages — so ordering against class registration and
// residency setup is not load-bearing (the engine consults
// gd_sticky_call_virtual per call). The engine-side wrapper still runs
// gd_ring_drain after either path.
//
// GD_NO_FASTENTRY=1 disables arming, for A/B measurement.
func init() {
	if os.Getenv("GD_NO_FASTENTRY") != "" {
		return
	}
	fastcb.ArmEntry(dispatchVirtual, unsafe.Pointer(C.gd_stock_virtual_entry()), func(pc uintptr) {
		C.gd_sticky_call_virtual = unsafe.Pointer(pc)
	})
}

// dispatchVirtual forwards a resident virtual call to the classdb dispatch
// wired into sticky.Dispatch (see classdb/sticky_wire.go). The indirection
// tolerates init order: package gd initialises before classdb, but no virtual
// can arrive before classdb has registered a class.
func dispatchVirtual(instance, userdata, result, args uintptr) {
	if dispatch := sticky.Dispatch; dispatch != nil {
		dispatch(instance, userdata, result, args)
	}
}
