//go:build (!go1.26 || (!amd64 && !arm64)) && !wasm

package threadcheck

import "graphics.gd/internal/gdextension"

func Init() {}

// Main falls back to the cgo-based thread check on platforms
// without assembly support or unsupported Go versions.
func Main() bool {
	return gdextension.Host.Threads.Main()
}

// Mark is a no-op on platforms without fast thread identification.
func Mark() {}

// EnterCall is a no-op on platforms without fast thread identification.
func EnterCall() {}

// LeaveCall is a no-op on platforms without fast thread identification.
func LeaveCall() {}

// Engine reports true so that off-main-thread calls are dispatched directly
// (the pre-dispatch-ring behaviour) on platforms where engine-owned threads
// cannot be told apart from user threads.
func Engine() bool { return true }
