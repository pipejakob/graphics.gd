//go:build musl || archive

package startup

// Resident-callback mode for static (engine-from-Go) builds. Requires the
// fastcb runtime patch (runtime/cgocall.go overlay, bundled with and applied
// automatically by `gd build`); without it fastcb.SetResident reports false
// and the caller falls back to the stock iteration loop.
//
// The main loop drives each engine frame through IterationResident
// (asmcgocall), so this goroutine stays _Grunning with its P wired for the
// frame; with residency set, every nested engine->Go callback skips the
// cgocallback exitsyscall/reentersyscall transition entirely. Between
// iterations control is back in ordinary Go, fully preemptible and scannable,
// so no per-frame yield is needed.

import (
	"runtime"

	"graphics.gd/internal/fastcb"
)

func fastcbScene(engine *engineAsStaticLibrary) bool {
	// The static Scene loop owns residency enable/clear OUTSIDE any callback;
	// the per-frame driver must stay out of the way (clearing residency from
	// inside a callback mid-frame leaves that frame's remaining callbacks on
	// the stock path while _Grunning, which is fatal on this target).
	fastcbFrameDriving = false
	runtime.LockOSThread()
	if !fastcb.SetResident() {
		runtime.UnlockOSThread()
		return false
	}
	for !engine.Library.IterationResident() {
	}
	fastcb.ClearResident()
	runtime.UnlockOSThread()
	return true
}
