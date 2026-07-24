//go:build go1.26 && amd64 && cgo && !O0

package classdb

import (
	"unsafe"

	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/gdreference"
	"graphics.gd/internal/sticky"
)

// Wire the sticky-P dispatch target to the same logic as
// On.Extension.Instance.Called (callbacks.go). The sticky entry thunk — once
// written and armed (see internal/sticky/DESIGN.md) — calls this after
// establishing the Go execution context. Assigning it here is inert until that
// thunk is registered as the engine's call_virtual_with_data_func; the stock
// cgocallback path continues to call the Called handler directly.
func init() {
	sticky.Dispatch = func(instance, userdata, result, args uintptr) {
		pv := (*pinnedVirtualFunc)(unsafe.Pointer(userdata))
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
}
