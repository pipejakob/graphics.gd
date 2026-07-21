//go:build gc && !go1.27

package classdb

import (
	"unsafe"

	"graphics.gd/internal/gdclass"
)

// ifaceWords mirrors the gc compiler's layout of a non-empty interface value:
// a type-word (itab) followed by a data-word. This layout is not guaranteed by
// the language spec, so this file is constrained to the gc compiler and to the
// Go versions it has been validated against — after checking a new release,
// bump the !go1.N constraint here and in iface_portable.go. Any other
// compiler or version falls back to reflect in
// [instanceImplementation.Interface].
type ifaceWords struct {
	tab  unsafe.Pointer
	data unsafe.Pointer
}

func (instance *instanceImplementation) cachedInterface(data unsafe.Pointer) (gdclass.Pointer, bool) {
	if instance.itab == nil {
		return nil, false
	}
	var iface gdclass.Pointer
	*(*ifaceWords)(unsafe.Pointer(&iface)) = ifaceWords{tab: instance.itab, data: data}
	return iface, true
}

func (instance *instanceImplementation) cacheInterface(iface gdclass.Pointer) {
	instance.itab = (*ifaceWords)(unsafe.Pointer(&iface)).tab
}
