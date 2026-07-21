//go:build !(go1.26 && (amd64 || arm64) && cgo && !O0)

package gd

// Fallbacks where the sticky fast path is unavailable: never armed, so
// IterationHoldingP is never called (panics defensively if it somehow is).

func StickyFastPathArmed() bool { return false }

func IterationHoldingP(obj, method, shape uintptr) bool {
	panic("gd: IterationHoldingP called without the sticky fast path")
}
