package gd_test

import (
	"runtime"
	"testing"

	"graphics.gd/internal/fastcb"
)

// TestFastcbPatch verifies, when the binary carries the fastcb runtime patch
// (gd build/test applies the cgocall.go overlay), that the hook PCs the
// patched runtime published really are the runtime's fastcb hooks — read-only
// evidence the resident-callback machinery is wired on this platform. Skips
// under plain `go test`, which builds without the overlay.
func TestFastcbPatch(t *testing.T) {
	if !fastcb.Available() {
		t.Skip("fastcb runtime patch not present in this binary")
	}
	want := [4]string{
		"runtime.fastcbSetResident",
		"runtime.fastcbClearResident",
		"runtime.fastcbYield",
		"runtime.fastcbCallC",
	}
	for i, pc := range fastcb.PCs() {
		f := runtime.FuncForPC(pc)
		if f == nil {
			t.Fatalf("hook %d (%s): PC %#x resolves to no function", i, want[i], pc)
		}
		if f.Name() != want[i] || f.Entry() != pc {
			t.Fatalf("hook %d: PC %#x resolves to %s (entry %#x), want %s", i, pc, f.Name(), f.Entry(), want[i])
		}
		t.Logf("fastcb hook %s at %#x (%s/%s)", want[i], pc, runtime.GOOS, runtime.GOARCH)
	}
}
