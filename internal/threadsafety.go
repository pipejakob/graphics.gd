package gd

import "sync/atomic"

// PhysicsServer2DThreadSafe and PhysicsServer3DThreadSafe gate the direct
// cross-thread dispatch of the physics server singletons, which are only
// thread-safe when they run on their own thread, per
// https://docs.godotengine.org/en/stable/tutorials/performance/thread_safe_apis.html
// They are set at startup from the "physics/2d/run_on_separate_thread" and
// "physics/3d/run_on_separate_thread" project settings (see
// startup/singletons.go); the generated physics server bindings consult them
// through noescape.CallThreadSafeIf.
var (
	PhysicsServer2DThreadSafe atomic.Bool
	PhysicsServer3DThreadSafe atomic.Bool
)
