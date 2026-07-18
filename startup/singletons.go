package startup

import (
	ProjectSettings "graphics.gd/classdb/ProjectSettings"
	gd "graphics.gd/internal"
)

// The generated bindings of thread-safe engine singletons (see
// gdfunc.ThreadSafeSingletons) call the engine directly from any thread. The
// physics servers are only thread-safe when they run on their own thread,
// per https://docs.godotengine.org/en/stable/tutorials/performance/thread_safe_apis.html
// so their direct dispatch is gated on the corresponding project settings,
// read once at startup.
func init() {
	gd.StartupFunctions = append(gd.StartupFunctions, func() {
		enabled2d, _ := ProjectSettings.GetSetting("physics/2d/run_on_separate_thread", false).(bool)
		gd.PhysicsServer2DThreadSafe.Store(enabled2d)
		enabled3d, _ := ProjectSettings.GetSetting("physics/3d/run_on_separate_thread", false).(bool)
		gd.PhysicsServer3DThreadSafe.Store(enabled3d)
	})
}
