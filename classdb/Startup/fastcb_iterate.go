//go:build musl || archive

package Startup

import (
	gd "graphics.gd/internal"
	"graphics.gd/internal/gdextension"
)

// IterationResident runs one main-loop iteration via gd.IterationHoldingP
// (runtime.asmcgocall): the calling goroutine stays _Grunning with its P wired
// for the whole engine frame. Combined with the fastcb runtime overlay's
// resident-callback mode, every engine->Go callback nested inside the frame
// takes the zero-coordination fast path instead of a full cgocallback
// transition. Main thread only; the caller must have enabled residency.
func (self Instance) IterationResident() bool {
	adv := Advanced(self)
	return gd.IterationHoldingP(uintptr(gd.ObjectChecked(adv.AsObject())), uintptr(methods.iteration), uintptr(gdextension.SizeBool))
}
