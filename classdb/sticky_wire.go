//go:build go1.26 && (amd64 || arm64) && cgo && !O0

package classdb

import (
	"unsafe"

	gd "graphics.gd/internal"
	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/gdreference"
	"graphics.gd/internal/sticky"
	"graphics.gd/variant/Float"
)

// Wire the sticky-P dispatch target to the same logic as
// On.Extension.Instance.Called (callbacks.go). The sticky entry thunk — once
// written and armed (see internal/sticky/DESIGN.md) — calls this after
// establishing the Go execution context. Assigning it here is inert until that
// thunk is registered as the engine's call_virtual_with_data_func; the stock
// cgocallback path continues to call the Called handler directly.
func init() {
	dispatch := func(instance, userdata, result, args uintptr) {
		pv := (*pinnedVirtualFunc)(unsafe.Pointer(userdata))
		if pv.tick != nil && instance != 0 {
			pv.tick(unsafe.Pointer(instance), Float.X(gd.UnsafeGet[float64](gdextension.Pointer(args), 0)))
			gdreference.Barrier()
			return
		}
		if ptr, ok := fastInterface(pv.tab, gdextension.ExtensionInstanceID(instance)); ok {
			pv.fn(ptr, gdextension.Pointer(args), gdextension.Pointer(result))
			gdreference.Barrier()
			return
		}
		receiver := instances.Get(gdextension.ExtensionInstanceID(instance))
		if receiver == nil {
			return
		}
		ptr, ok := receiver.Interface()
		if !ok {
			return
		}
		pv.fn(ptr, gdextension.Pointer(args), gdextension.Pointer(result))
		gdreference.Barrier()
	}
	sticky.Dispatch = dispatch
	// Re-arm the direct entry with the closure itself: package gd armed its
	// init-order trampoline (dispatchVirtual) before this package initialised;
	// dispatching straight to the closure drops that hop from every resident
	// virtual call.
	gd.RearmVirtualDispatch(dispatch)
}
