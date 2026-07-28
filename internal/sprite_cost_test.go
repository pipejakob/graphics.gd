//go:build !generate

package gd_test

import (
	"fmt"
	"structs"
	"testing"

	"graphics.gd/classdb/Node2D"
	gd "graphics.gd/internal"
	"graphics.gd/internal/gdextension"
	"graphics.gd/internal/noescape"
	"graphics.gd/internal/pointers"
	"graphics.gd/variant/Angle"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// This file decomposes the per-sprite, per-frame work of spritebench's Go
// implementation, whose Process body is
//
//	s.pos = Vector2.Add(s.pos, Vector2.MulX(Angle.CosSin(s.angle), s.speed*delta))
//	s.AsNode2D().SetPosition(s.pos)
//	...two bounds comparisons
//
// The full-benchmark slope is ~190ns/sprite, most of which is engine-side
// (set_position's transform notification). These measure only the parts Go
// controls, so an optimisation can be aimed at the one that is actually
// expensive rather than at whichever is easiest to reach.

// setPositionArgs is the argument frame of Node2D.set_position, matching what
// the generated wrapper builds.
type setPositionArgs struct {
	_        structs.HostLayout
	position Vector2.XY
}

const (
	shapeSetPosition = gdextension.Shape(0 | gdextension.SizeVector2<<4)
	setPositionHash  = 743155724
)

// BenchmarkSpriteSetPositionWrapped is the call spritebench actually makes:
// the generated Node2D wrapper, which resolves the object, builds the argument
// frame and keeps the receiver alive around the call.
//
// It is measured over a range of node counts because one node is not a
// miniature of many. GetObject's fast path chases obj.sentinel, a separate
// allocation per node; with a single node that pointer is L1-hot and the
// wrapper looks nearly free, while a scene of 100k sprites touches a cold
// sentinel per sprite per frame. Anything that shows up only in the larger
// counts is a cache effect, and cache effects are what a benchmark with one
// hot node is structurally unable to see.
func BenchmarkSpriteSetPositionWrapped(B *testing.B) {
	for _, nodes := range []int{1, 1000, 50000} {
		B.Run(fmt.Sprintf("nodes=%d", nodes), func(B *testing.B) {
			benchOnMain(B, func(B *channelB) {
				held := newNode2Ds(B, nodes)
				pos := Vector2.New(1, 2)
				B.ReportAllocs()
				B.ResetTimer()
				for i := 0; B.Loop(); i++ {
					held[i%len(held)].SetPosition(pos)
				}
				B.StopTimer()
				gd.Flush()
			})
		})
	}
}

// newNode2Ds builds n nodes and frees them when the benchmark ends.
func newNode2Ds(B *channelB, n int) []Node2D.Instance {
	held := make([]Node2D.Instance, n)
	for i := range held {
		held[i] = Node2D.New()
	}
	B.Cleanup(func() {
		for _, node := range held {
			gd.ObjectFree(node.AsObject()[0])
		}
	})
	return held
}

// BenchmarkSpriteSetPositionDirect makes the same engine call without the
// generated wrapper: same ring path, same ptrcall, but the object pointer and
// argument frame are hoisted out of the loop. The difference against
// BenchmarkSpriteSetPositionWrapped is what the wrapper costs per sprite per
// frame — ObjectChecked, AsObject, the argument-frame literal and the
// keepalive — and so what a fast path for hot setters could recover.
func BenchmarkSpriteSetPositionDirect(B *testing.B) {
	for _, nodes := range []int{1, 1000, 50000} {
		B.Run(fmt.Sprintf("nodes=%d", nodes), func(B *testing.B) {
			benchOnMain(B, func(B *channelB) {
				held := newNode2Ds(B, nodes)
				objects := make([]gdextension.Object, len(held))
				for i, node := range held {
					objects[i] = gd.ObjectChecked(node.AsObject())
				}
				class, method := gd.NewStringName("Node2D"), gd.NewStringName("set_position")
				defer class.Free()
				defer method.Free()
				bind := gdextension.Host.Objects.Method.Lookup(
					pointers.Get(class), pointers.Get(method), setPositionHash,
				)
				if bind == 0 {
					B.Fatal("Node2D.set_position method bind not found: the hash has drifted from this engine build")
				}
				args := setPositionArgs{position: Vector2.New(1, 2)}
				B.ReportAllocs()
				B.ResetTimer()
				for i := 0; B.Loop(); i++ {
					noescape.Call[struct{}](objects[i%len(objects)], bind, shapeSetPosition, &args)
				}
				B.StopTimer()
				gd.Flush()
			})
		})
	}
}

// BenchmarkSpriteMath is the movement arithmetic alone, with no engine call:
// the part of the sprite's frame that is pure Go and could in principle be as
// fast as the assembled version's.
func BenchmarkSpriteMath(B *testing.B) {
	var (
		pos   = Vector2.New(100, 100)
		angle = Angle.Radians(0.5)
		speed = Float.X(300)
		delta = Float.X(1.0 / 60)
		half  = Vector2.New(16, 16)
		size  = Vector2.New(1920, 1080)
	)
	B.ReportAllocs()
	for B.Loop() {
		pos = Vector2.Add(pos, Vector2.MulX(Angle.CosSin(angle), speed*delta))
		if pos.X < half.X || pos.X > size.X-half.X {
			angle = Angle.Pi - angle
		}
		if pos.Y < half.Y || pos.Y > size.Y-half.Y {
			angle = -angle
		}
	}
	sinkVector2 = pos
}

var sinkVector2 Vector2.XY
