//go:build !generate

package gd_test

import (
	"sync"
	"testing"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GDScript"
	gd "graphics.gd/internal"
	"graphics.gd/internal/fastcb"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// growStack forces multiple stack growths (and therefore stack COPIES) on the
// calling goroutine: ~2MB of 1KB frames from an 8KB starting stack.
//
//go:noinline
func growStack(n int) int {
	var pad [1024]byte
	pad[0] = byte(n)
	if n == 0 {
		return int(pad[0])
	}
	return growStack(n-1) + int(pad[0])%1
}

// GrowingVirtualControl's virtual moves the goroutine stack before returning
// its value, modelling a scene callback that does real work.
type GrowingVirtualControl struct {
	Control.Extension[GrowingVirtualControl] `gd:"GrowingVirtualControl"`
}

func (b *GrowingVirtualControl) GetMinimumSize() Vector2.XY {
	growStack(2000)
	return Vector2.XY{X: 12, Y: 34}
}

var registerGrowingVirtualOnce sync.Once

// TestResidentCallStackMove exercises the outbound-call stack-move hazard: an
// engine call whose nested Go callback grows (moves) the goroutine stack while
// the engine holds pointers into the call's argument/result memory. On the
// resident path (fastcb.CallC) the call is staged through non-moving global
// packs; on the stock path the sized results travel by C value through
// cgo-generated shims. Either way the values must arrive intact.
func TestResidentCallStackMove(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		registerGrowingVirtualOnce.Do(func() {
			classdb.Register[GrowingVirtualControl]()
		})
		ctrl := new(GrowingVirtualControl)
		instance := ctrl.AsControl()
		for range 3 {
			size := instance.GetMinimumSize()
			if size.X != 12 || size.Y != 34 {
				t.Fatalf("virtual result corrupted across stack move: got %v", size)
			}
		}

		// Variant-call path: GDScript calls a Go callable that moves the
		// stack; the script's return value must come back intact through the
		// staged variant pack (result AND error slot).
		var script = GDScript.New().AsScript()
		script.SetSourceCode(`extends Object
func bench(c):
	return c.call() + 1000
`)
		script.Reload()
		obj := Object.New()
		obj.SetScript(script)
		result, err := gd.ObjectCall(obj[0], gd.NewStringName("bench"), gd.NewVariant(gd.NewCallable(func() int {
			growStack(2000)
			return 337
		})))
		if err != nil {
			t.Fatalf("variant call error corrupted across stack move: %v", err)
		}
		if got := result.Interface().(int64); got != 1337 {
			t.Fatalf("variant call result corrupted across stack move: got %d", got)
		}
		gd.ObjectFree(obj[0])

		if fastcb.Available() && !fastcb.Resident() {
			t.Errorf("fastcb patch present but residency not engaged during the test")
		}
	})
}
