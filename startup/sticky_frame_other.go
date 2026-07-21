//go:build !(go1.26 && amd64 && cgo && !O0)

package startup

// stickyEndFrame is a no-op where the sticky-P path is unavailable (non-amd64,
// non-cgo, wasm, or O0 builds).
func stickyEndFrame() {}
