//go:build !generate

package gd_test

import (
	"structs"
	"sync"
	"testing"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Node"
	gd "graphics.gd/internal"
	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/noescape"
	"graphics.gd/internal/pointers"
	"graphics.gd/variant/Float"
)

// BenchProcessNode measures the engine→Go dispatch of _process, which
// BenchmarkVirtualCallback (on _get_minimum_size) does not cover: _process is
// the virtual a scene calls once per node per frame, so it is the only one
// whose dispatch cost is multiplied by the node count, and classdb gives it a
// dedicated path (pinnedVirtualFunc.tick).
type BenchProcessNode struct {
	Node.Extension[BenchProcessNode] `gd:"BenchProcessNode"`

	ticks int
}

func (b *BenchProcessNode) Process(delta Float.X) { b.ticks++ }

// notifyArgs is the packed argument frame of Object.notification(what,
// reversed), in the layout a ptrcall expects: the fields are read at the
// offsets shapeNotification describes.
type notifyArgs struct {
	_        structs.HostLayout
	what     int64
	reversed bool
}

// shapeNotification describes Object.notification for the unsafe call path:
// no result (nibble 0), an int argument (nibble 1) and a bool (nibble 2).
const shapeNotification = gdextension.Shape(0 | 4<<4 | 1<<8)

// notificationHash is Godot 4.7's hash for Object.notification(int, bool). A
// hash the engine does not recognise yields a null method bind, which the
// benchmark below catches rather than crashing on.
const notificationHash = 4023243586

var registerBenchProcessOnce sync.Once

func registerBenchProcess() { classdb.Register[BenchProcessNode]() }

// BenchmarkVirtualProcess drives _process the way a frame does. Godot
// dispatches it from Node::_notification(NOTIFICATION_PROCESS), which does
// GDVIRTUAL_CALL(_process, get_process_delta_time()), so notifying the node
// runs exactly the engine→Go path a frame runs — without needing the node in
// a tree or a frame to elapse.
//
// The notification goes out as a method-bind ptrcall rather than through
// gd.ObjectCall: the variant path costs ~750ns and allocates, which would
// bury the dispatch cost this benchmark exists to measure (the round trip
// itself is tens of nanoseconds — see BenchmarkVirtualCallback).
func BenchmarkVirtualProcess(B *testing.B) {
	benchOnMain(B, func(B *channelB) {
		B.ReportAllocs()
		registerBenchProcessOnce.Do(registerBenchProcess)
		node := new(BenchProcessNode)
		object := gd.ObjectChecked(node.AsObject())

		class, method := gd.NewStringName("Object"), gd.NewStringName("notification")
		defer class.Free()
		defer method.Free()
		notify := gdextension.Host.Objects.Method.Lookup(
			pointers.Get(class), pointers.Get(method), notificationHash,
		)
		if notify == 0 {
			B.Fatal("Object.notification method bind not found: the hash has drifted from this engine build")
		}
		args := notifyArgs{what: int64(Node.NotificationProcess)}

		// Prove the notification reaches Process before timing it: a trigger
		// that silently did nothing would benchmark an empty loop.
		noescape.Call[struct{}](object, notify, shapeNotification, &args)
		gd.Flush()
		if node.ticks != 1 {
			B.Fatalf("notification did not reach Process: ticks = %d, want 1", node.ticks)
		}

		B.ResetTimer()
		for B.Loop() {
			noescape.Call[struct{}](object, notify, shapeNotification, &args)
		}
		B.StopTimer()
		gd.Flush()
		if node.ticks < B.N {
			B.Fatalf("Process ran %d times, want at least %d", node.ticks, B.N)
		}
	})
}
