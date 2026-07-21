//go:build !cgo && !wasm

package noescape

import (
	"sync/atomic"

	"graphics.gd/internal/gdextension"
)

func Call[T any](object gdextension.Object, method gdextension.MethodForClass, shape gdextension.Shape, args any) T {
	panic("not implemented")
}

func CallThreadSafe[T any](object gdextension.Object, method gdextension.MethodForClass, shape gdextension.Shape, args any) T {
	panic("not implemented")
}

func CallThreadSafeIf[T any](safe *atomic.Bool, object gdextension.Object, method gdextension.MethodForClass, shape gdextension.Shape, args any) T {
	panic("not implemented")
}

func (method MethodForClass) Call(self gdextension.Object, args ...gdextension.Variant) (gdextension.Variant, error) {
	panic("not implemented")
}

// ScriptCallResident always reports false without cgo: resident-callback mode
// requires the fastcb runtime patch, which is native-only.
func ScriptCallResident(object gdextension.Object, name gdextension.StringName, args []gdextension.Variant) (gdextension.Variant, gdextension.CallError, bool) {
	return gdextension.Variant{}, gdextension.CallError{}, false
}
