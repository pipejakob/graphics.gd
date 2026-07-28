//go:build !generate

package gd_test

import (
	"fmt"
	"sync"
	"testing"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Node"
	gd "graphics.gd/internal"
	"graphics.gd/internal/pointers"
	"graphics.gd/variant/Object"
)

// BenchRootedNode holds a non-node engine reference, so classdb compiles a
// keepalive for it and roots every instance (register_class.go). The field
// type matters: compile_keepalive deliberately skips exported fields that
// implement Node.Any, since the scene tree owns those, so a Node.Instance
// field would produce no keepalive and no root at all. BenchProcessNode, whose
// fields are all plain numbers, is likewise never rooted — which is why the
// two appear side by side below. Without a class that is genuinely rooted, a
// flat keepalive measurement proves nothing.
type BenchRootedNode struct {
	Node.Extension[BenchRootedNode] `gd:"BenchRootedNode"`

	Ref Object.Instance
}

var registerBenchRootedOnce sync.Once

func registerBenchRooted() { classdb.Register[BenchRootedNode]() }

// BenchmarkFrameKeepAlive measures the per-frame keepalive walk against the
// number of live instances. The frame hook calls each root's compiled
// keepalive once per frame, so for a class that has one this is O(instances)
// work a scene pays every frame on top of whatever its nodes do — and being
// per-instance, it lands in a benchmark's per-sprite slope rather than its
// fixed cost, where it is easy to mistake for dispatch or engine-call cost.
func BenchmarkFrameKeepAlive(B *testing.B) {
	for _, rooted := range []bool{false, true} {
		for _, live := range []int{0, 1000, 10000} {
			B.Run(fmt.Sprintf("rooted=%v/live=%d", rooted, live), func(B *testing.B) {
				benchOnMain(B, func(B *channelB) {
					registerBenchProcessOnce.Do(registerBenchProcess)
					registerBenchRootedOnce.Do(registerBenchRooted)
					held := make([]any, 0, live)
					for range live {
						if rooted {
							node := new(BenchRootedNode)
							node.AsObject()
							held = append(held, node)
						} else {
							node := new(BenchProcessNode)
							node.AsObject()
							held = append(held, node)
						}
					}
					B.ReportAllocs()
					B.ResetTimer()
					for B.Loop() {
						keep_reachable_instances_alive()
					}
					B.StopTimer()
					if len(held) != live {
						B.Fatal("instances were collected mid-benchmark")
					}
				})
			})
		}
	}
}

// BenchmarkFramePointersCycle measures the per-frame handle-table sweep, in
// both the state a frame can be in. Cycle skips the scan entirely when nothing
// was allocated since the last one, so a frame that mints no handles is nearly
// free however big the table is; a frame that mints even one pays a walk over
// the whole table. Which of those a scene is in is the difference between the
// handle table costing nothing and it costing a per-frame tax proportional to
// everything alive, so the two are measured separately rather than averaged.
func BenchmarkFramePointersCycle(B *testing.B) {
	for _, mint := range []bool{false, true} {
		for _, live := range []int{0, 1000, 10000} {
			B.Run(fmt.Sprintf("mint=%v/live=%d", mint, live), func(B *testing.B) {
				benchOnMain(B, func(B *channelB) {
					held := make([]gd.StringName, 0, live)
					for i := range live {
						held = append(held, gd.NewStringName(fmt.Sprint("bench", i)))
					}
					B.ReportAllocs()
					B.ResetTimer()
					for B.Loop() {
						if mint {
							// One fresh handle is all it takes to defeat the
							// early-out, which is the point of measuring it.
							gd.NewStringName("mint").Free()
						}
						pointers.Cycle()
					}
					B.StopTimer()
					for _, name := range held {
						name.Free()
					}
				})
			})
		}
	}
}
