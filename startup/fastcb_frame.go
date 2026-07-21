package startup

import (
	"runtime"

	"graphics.gd/internal/fastcb"
)

// Resident-callback driving for builds where the ENGINE owns the main loop
// (c-shared GDExtension): the first frame locks the engine thread and enables
// residency, every frame requests a yield (so the thread drops to _Gsyscall
// during the engine's idle gap and the GC can interact with it), and engine
// shutdown clears residency so teardown unwinds through the stock protocol.
// All of it no-ops when the binary lacks the fastcb runtime patch — see
// graphics.gd/internal/fastcb.
//
// Static builds (musl/archive), where graphics.gd owns the loop and enters
// the engine via asmcgocall, must NOT drive residency from frame callbacks:
// their Scene loop owns enable/clear outside any callback (fastcbScene) and
// needs no per-frame yield. Those paths set fastcbFrameDriving = false before
// the first frame.
var (
	fastcbFrameDriving = true
	fastcbEngaged      bool
	fastcbDone         bool
)

// fastcbFrame runs at the top of every engine frame, on the engine's main
// thread, inside a C->Go callback.
func fastcbFrame() {
	if !fastcbFrameDriving || fastcbDone {
		return
	}
	if !fastcbEngaged {
		runtime.LockOSThread()
		if !fastcb.SetResident() {
			runtime.UnlockOSThread()
			fastcbDone = true
			return
		}
		fastcbEngaged = true
	}
	fastcb.Yield()
}

// fastcbShutdown runs from the MainLoop shutdown callback. Clearing residency
// from inside a callback is safe on the engine-owns-loop path: the deferred
// transition performs the pending reentersyscall when the innermost callback
// returns, and every later callback takes the stock path.
func fastcbShutdown() {
	fastcbDone = true
	if fastcbEngaged {
		fastcbEngaged = false
		fastcb.ClearResident()
		runtime.UnlockOSThread()
	}
}
