package Engine

// Allocated declares a class-struct field that lives in the engine's
// allocator rather than the Go heap. When an instance of the class is
// created, the field is pointed at freshly allocated engine memory —
// allocated alongside the engine's own half of the instance, which keeps
// the pair in the same neighbourhood of memory — and when the engine
// frees the instance the memory is returned with it.
//
// The garbage collector never sees the allocation, so T must not hold
// anything the collector would need to keep alive: no pointers, slices,
// maps, strings, channels, functions or interfaces. Registering a class
// with an [Allocated] field of such a type panics.
//
// The field is nil until the engine half of the instance exists, and the
// memory dies with the instance: a pointer into it must not outlive the
// object.
type Allocated[T any] *T
