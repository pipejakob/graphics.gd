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

	}
}
