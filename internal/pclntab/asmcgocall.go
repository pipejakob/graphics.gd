//go:build amd64 || arm64

package pclntab

import (
	"sync"
	"unsafe"
)

var asmcgocallPC = sync.OnceValue(func() uintptr {
	pc, ok := EntryPC("runtime.asmcgocall")
	if !ok {
		panic("pclntab: runtime.asmcgocall not found in the pclntab")
	}
	return pc
})

// Asmcgocall is runtime.asmcgocall: it runs the C function fn(arg) on the g0
// stack WITHOUT entersyscall, so the calling goroutine stays _Grunning with
// its P wired for the duration and returns fn's int result. The entry PC is
// resolved from the pclntab on first use instead of a //go:linkname pull.
func Asmcgocall(fn, arg unsafe.Pointer) int32 {
	return callAsmcgocall(asmcgocallPC(), fn, arg)
}

// callAsmcgocall is implemented in asmcgocall_GOARCH.s. The binary may hold
// runtime.asmcgocall as the ABI0 assembly definition, a linker-generated
// ABIInternal wrapper, or both — and the pclntab names them identically — so
// the trampoline stages the arguments for both conventions at once and either
// callee reads its own.
func callAsmcgocall(pc uintptr, fn, arg unsafe.Pointer) int32
