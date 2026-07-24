package ring

import (
	"structs"
	"unsafe"

	"graphics.gd/internal/gdextension"
)

type Entry struct {
	_      structs.HostLayout
	Object uintptr    // gdextension.Object
	Method uintptr    // gdextension.MethodForClass
	Shape  uint64     // gdextension.Shape
	Args   [256]byte  // copied packed args
	Result [64]byte   // result slot (for future phases)
	Refs   [16]uint16 // intra-buffer references (for future phases)
	Owner  uintptr    // back-pointer for future result offloading
	PC     uintptr    // Go caller PC, captured at enqueue time
}

const Size = 256 // power of 2
const Mask = Size - 1

type Ring struct {
	_       structs.HostLayout
	head    uint32
	tail    uint32
	Entries [Size]Entry
}

var Main Ring

var CrashIndex uint32 = 0xFFFFFFFF

func init() {
	// C drains Main directly (gd_ring_drain reads head/tail and advances tail
	// past executed entries): the prefix layout is load-bearing on both sides.
	if unsafe.Offsetof(Main.Entries) != 8 {
		panic("ring: Ring prefix must be (head, tail uint32) — gd.c's gd_ring_buffer depends on it")
	}
	if unsafe.Offsetof(threadsShared.seq) != 16 || unsafe.Offsetof(threadsShared.kind) != 1040 {
		panic("ring: mpscShared layout drifted — gd.c's gd_mpsc_shared depends on it")
	}
}

func (r *Ring) Pending() bool {
	return r.head != r.tail
}

func (r *Ring) Buffer(object, method uintptr, shape uint64, args unsafe.Pointer, pc uintptr) {
	if r.head-r.tail >= Size {
		r.Flush()
	}
	e := &r.Entries[r.head&Mask]
	e.Object = object
	e.Method = method
	e.Shape = shape
	n := gdextension.Shape(shape).SizeArguments()
	if n > 0 && args != nil {
		copyArgs(&e.Args, args, n)
	}
	e.PC = pc
	r.head++
}

// copyArgs copies the packed argument bytes into an entry. The common
// argument sizes take exact fixed-size copies (direct moves, no memmove
// call): a variable-length copy costs more than the arguments themselves
// for the small packs nearly every buffered call carries.
func copyArgs(dst *[256]byte, args unsafe.Pointer, n int) {
	p := unsafe.Pointer(dst)
	switch n {
	case 4:
		*(*[4]byte)(p) = *(*[4]byte)(args)
	case 8:
		*(*[8]byte)(p) = *(*[8]byte)(args)
	case 12:
		*(*[12]byte)(p) = *(*[12]byte)(args)
	case 16:
		*(*[16]byte)(p) = *(*[16]byte)(args)
	case 24:
		*(*[24]byte)(p) = *(*[24]byte)(args)
	case 32:
		*(*[32]byte)(p) = *(*[32]byte)(args)
	default:
		copy(dst[:n], unsafe.Slice((*byte)(args), n))
	}
}

func (r *Ring) Flush() {
	if r.head == r.tail {
		return
	}
	tail := r.tail
	head := r.head
	r.tail = head // advance tail before entering C, so re-entrant flushes
	// (from Godot callbacks during ptrcall) only see newly-buffered entries.
	flush(unsafe.Pointer(&r.Entries[0]), tail, head)
}
