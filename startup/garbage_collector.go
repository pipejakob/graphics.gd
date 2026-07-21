package startup

import (
	gd "graphics.gd/internal"
	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/gdreference"
	"graphics.gd/internal/pointers"
	"graphics.gd/internal/ring"
	"graphics.gd/variant/Callable"

	_ "unsafe"
)

//go:linkname keep_reachable_instances_alive graphics.gd/classdb.keep_reachable_instances_alive
func keep_reachable_instances_alive()

func init() {
	gdextension.On.MainLoop.EveryFrame = func() {
		// Engage/refresh resident-callback mode for engine-owns-loop builds
		// (no-op on static builds and without the fastcb runtime patch).
		fastcbFrame()
		// Hand the main ring to C (idempotent): from here on the callback
		// trampolines drain buffered calls in C as each callback returns, so
		// the flushes below usually find the ring empty.
		ring.Adopt()
		// Release any sticky-P held across this frame's virtual-call batch
		// before the frame-boundary GC/flush work below (which allocates and
		// must not run while a P is held sticky). No-op until sticky is armed.
		stickyEndFrame()
		Callable.Cycle()
		ring.Main.Flush()
		// Drain calls queued by user goroutines (cross-thread dispatch),
		// keeping a frame-period-sized window open for the follow-up calls
		// of goroutines released by the drain (see ring.FollowUpRatio), then
		// flush anything their engine callbacks buffered in response.
		ring.Threads.FlushFrame()
		ring.Main.Flush()
		keep_reachable_instances_alive()
		gd.CycleAnchors()
		gdreference.GC(gd.Free)
		pointers.Cycle()
	}
	gdextension.On.MainLoop.FinalFrame = func() {
		fastcbShutdown()
	}
}
