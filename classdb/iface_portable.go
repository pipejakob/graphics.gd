//go:build !gc || go1.27

package classdb

import (
	"unsafe"

	"graphics.gd/internal/gdclass"
)

// Portable fallback for compilers or Go versions where the interface layout
// assumed by iface_gc.go has not been validated: the itab cache is disabled
// and [instanceImplementation.Interface] always goes through reflect.
func (instance *instanceImplementation) cachedInterface(data unsafe.Pointer) (gdclass.Pointer, bool) {
	return nil, false
}

func (instance *instanceImplementation) cacheInterface(iface gdclass.Pointer) {}
