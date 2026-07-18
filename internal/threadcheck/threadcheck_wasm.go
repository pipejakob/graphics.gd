//go:build wasm

package threadcheck

func Init() {}

// Main always returns true on wasm, which is single-threaded.
func Main() bool { return true }

// Mark is a no-op on wasm, which is single-threaded.
func Mark() {}

// EnterCall is a no-op on wasm, which is single-threaded.
func EnterCall() {}

// LeaveCall is a no-op on wasm, which is single-threaded.
func LeaveCall() {}

// Engine always returns true on wasm, which is single-threaded.
func Engine() bool { return true }
