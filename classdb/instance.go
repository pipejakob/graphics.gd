package classdb

import (
	"reflect"
	"strings"

	gd "graphics.gd/internal"
	"graphics.gd/internal/gdclass"
	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/threadsafe"
)

// instances resolves the instance word Godot hands back on every extension
// callback to the Go-side record. On layout-known builds (see iface_gc.go)
// the word IS the address of the user's pinned Go struct, so the hot
// virtual-dispatch path can rebuild the receiver interface from the word
// plus the class's cached itab without touching this table at all — only
// cold paths (properties, notifications, reference counting, free) come
// here. On portable builds the word is an opaque counter and every path
// resolves through the table.
//
// The record (and, via its pin, the user's struct) stays alive from
// instanceID until the engine's Free callback drops it — the engine owns
// the lifetime, matching how the C++ bindings treat extension instances.
var instances instanceTable

type instanceTable struct {
	table threadsafe.Map[gdextension.ExtensionInstanceID, *instanceImplementation]
}

func (t *instanceTable) Get(id gdextension.ExtensionInstanceID) *instanceImplementation {
	impl, _ := t.table.Lookup(id)
	return impl
}

// New registers the instance under its dispatch word: the address of the
// user's struct (data) on layout-known builds, an opaque counter otherwise
// (see instanceID). data must be the pointer wrapped by instance.strong /
// instance.weak.
func (t *instanceTable) New(instance *instanceImplementation, data reflect.Value) gdextension.ExtensionInstanceID {
	id := instanceID(instance, data)
	t.table.Insert(id, instance)
	return id
}

// Del forgets the instance and releases the pin taken by instanceID.
// Callers must not use id with the engine after this returns. The
// memory behind the class's [Engine.Allocated] fields goes back to the
// engine here, which is what ends its life.
func (t *instanceTable) Del(id gdextension.ExtensionInstanceID) {
	if impl, ok := t.table.Lookup(id); ok && impl != nil {
		impl.pinner.Unpin()
		for _, addr := range impl.engineMemory {
			gdextension.Host.Memory.Free(addr)
		}
		impl.engineMemory = nil
	}
	t.table.Remove(id)
}

func (t *instanceTable) All(yield func(*instanceImplementation) bool) {
	for _, impl := range t.table.Iter() {
		if !yield(impl) {
			return
		}
	}
}

func nameOf(rtype reflect.Type) string {
	if rtype.Kind() == reflect.Array {
		return rtype.Elem().Name()
	}
	if rtype.Kind() == reflect.Pointer {
		return nameOf(rtype.Elem())
	}
	isClass := reflect.PointerTo(rtype).Implements(reflect.TypeFor[gd.IsClass]()) || rtype.Implements(reflect.TypeFor[gd.IsClass]())
	if rtype.Kind() == reflect.Struct && rtype.NumField() > 0 && isClass {
		if rtype.Field(0).Anonymous {
			if rename, ok := rtype.Field(0).Tag.Lookup("gd"); ok {
				return rename
			}
			if rtype.Name() == "" || !rtype.Implements(reflect.TypeFor[gdclass.Interface]()) {
				return nameOf(rtype.Field(0).Type)
			}
		}
		return strings.TrimPrefix(rtype.Name(), "class")
	}
	return ""
}
