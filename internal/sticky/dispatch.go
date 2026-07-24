//go:build go1.26 && (amd64 || arm64) && cgo && !O0

package sticky

// Dispatch is the Go-side virtual-method dispatch, wired by classdb at init to
// the body of On.Extension.Instance.Called. The direct virtual-call entry (the
// runtime-carried fastcb thunk, or this package's amd64 research thunk) calls
// this once the Go execution context (g, P, goroutine stack) is established.
// Kept as an indirect func to avoid an import cycle (sticky must not import
// classdb).
//
//	instance  – ExtensionInstanceID
//	userdata  – pinned virtual-call userdata (the *pinnedVirtualFunc)
//	result    – r_ret   (GDExtensionTypePtr)
//	args      – p_args  (const GDExtensionConstTypePtr*)
//
// All args are raw pointer values (gdextension.Pointer is uintptr); the thunks
// pass the engine's C ABI argument registers through unchanged.
var Dispatch func(instance, userdata, result, args uintptr)
