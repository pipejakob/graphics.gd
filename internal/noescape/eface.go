package noescape

import "unsafe"

// eface mirrors the runtime's layout of an empty interface value: a type word
// followed by a data word. Callers of [Call] and friends always pass args as
// a pointer boxed into an any, so the data word IS the argument pointer.
type eface struct {
	typ, data unsafe.Pointer
}

// argPointer extracts the pointer carried by args without going through
// reflect (reflect.ValueOf(args).UnsafePointer() was measured at ~10ns per
// engine call in spritebench — pure overhead on the hottest outbound path).
//
// The extracted pointer is only valid for use DURING the engine call being
// staged: every consumer either copies the pointed-to arguments immediately
// (ring.Ring.Buffer and ring.MPSC.fill copy by shape) or performs the
// crossing synchronously (call_noescape and the sized variants), so the
// caller's stack-allocated argument struct never needs to outlive the call.
//
//go:nosplit
func argPointer(args any) unsafe.Pointer {
	return (*eface)(unsafe.Pointer(&args)).data
}
