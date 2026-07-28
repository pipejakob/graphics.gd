package classdb

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"graphics.gd/internal/gdextension"
)

// allocatedElem reports whether a field type is an instantiation of
// [graphics.gd/classdb/Engine.Allocated], and if so which element type
// it carries.
func allocatedElem(rtype reflect.Type) (reflect.Type, bool) {
	if rtype.Kind() != reflect.Pointer || rtype.PkgPath() != "graphics.gd/classdb/Engine" || !strings.HasPrefix(rtype.Name(), "Allocated[") {
		return nil, false
	}
	return rtype.Elem(), true
}

// allocateFields points every [Engine.Allocated] field of the class
// struct at base at freshly allocated engine memory, zeroed. It runs as
// the instance is created, so the memory is served from wherever the
// engine was just allocating the instance's own object — the two halves
// of the instance end up neighbours. The allocations are returned so the
// instance can hand them back when the engine frees it.
func allocateFields(rtype reflect.Type, base unsafe.Pointer) []gdextension.Pointer {
	var allocs []gdextension.Pointer
	for i := range rtype.NumField() {
		field := rtype.Field(i)
		elem, ok := allocatedElem(field.Type)
		if !ok {
			continue
		}
		if pointersIn(elem) {
			panic(fmt.Sprintf("classdb: %s.%s: Engine.Allocated[%s] can hold a Go pointer, which the collector would not see", rtype, field.Name, elem))
		}
		addr := gdextension.Host.Memory.Malloc(int(elem.Size()))
		gdextension.Host.Memory.Clear(addr, int(elem.Size()))
		*(*unsafe.Pointer)(unsafe.Add(base, field.Offset)) = unsafe.Pointer(uintptr(addr))
		allocs = append(allocs, addr)
	}
	return allocs
}

// pointersIn reports whether a value of the type can hold a pointer the
// garbage collector would be responsible for.
func pointersIn(rtype reflect.Type) bool {
	switch rtype.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return false
	case reflect.Array:
		return rtype.Len() > 0 && pointersIn(rtype.Elem())
	case reflect.Struct:
		for i := range rtype.NumField() {
			if pointersIn(rtype.Field(i).Type) {
				return true
			}
		}
		return false
	default:
		return true
	}
}
