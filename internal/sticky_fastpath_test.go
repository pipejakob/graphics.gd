//go:build go1.26 && amd64 && cgo && !O0 && musl

package gd_test

import (
	"testing"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Control"
	gd "graphics.gd/internal"
	"graphics.gd/internal/sticky"
	"graphics.gd/variant/Vector2"
)

// stickyProbeControl overrides a virtual (_get_minimum_size) with a known
// non-zero result. Calling it from Go triggers Go→engine ptrcall →
// GDVIRTUAL_CALL → call_virtual_with_data_func → (armed) sticky fast-path thunk.
type stickyProbeControl struct {
	Control.Extension[stickyProbeControl] `gd:"StickyProbeControl"`
}

func (c *stickyProbeControl) GetMinimumSize() Vector2.XY { return Vector2.XY{X: 12, Y: 34} }

// TestVirtualCallbackFastPath proves the fast path both (a) is actually taken
// for a real engine→Go virtual call and (b) returns the correct result through
// it — i.e. the no-cgocallback-transition thunk works under musl's threading.
func TestVirtualCallbackFastPath(t *testing.T) {
	if !sticky.Enabled {
		t.Skip("sticky no-switch fast path disabled: it faults (morestack on g0) " +
			"whenever a virtual makes an outbound cgo call, which every real " +
			"virtual does — see sticky.Enabled. Kept as research; re-enable to probe.")
	}
	runOnMain(t, func(t testing.TB) {
		classdb.Register[stickyProbeControl]()
		ctrl := new(stickyProbeControl)
		instance := ctrl.AsControl()

		entryAddr, cGlobal := gd.ArmDiagnostics()
		t.Logf("arming: sticky.EntryAddr=%#x  C.gd_sticky_call_virtual=%#x  sticky.Enabled=%v", entryAddr, cGlobal, sticky.Enabled)

		before := sticky.Calls()
		got := instance.GetMinimumSize()
		after := sticky.Calls()

		if got != (Vector2.XY{X: 12, Y: 34}) {
			t.Fatalf("virtual returned %v through the fast path, want {12 34}", got)
		}
		if after == before {
			t.Fatalf("virtual call did NOT route through the sticky fast path (Calls stayed %d)", before)
		}
		t.Logf("fast path serviced the engine→Go virtual (alloc-free): Calls %d->%d, result %v", before, after, got)
		_ = gd.StickyFastPathArmed
	})
}
