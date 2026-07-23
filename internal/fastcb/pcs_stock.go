//go:build !gd

package fastcb

import _ "unsafe" // for go:linkname

// On stock toolchains the resident-callback machinery arrives as the
// gd CLI's runtime/cgocall.go overlay, which pushes the hook PCs into
// this declaration via its own linkname. Without the overlay nothing
// writes it and the slots stay zero.
//
//go:linkname runtimePCs
var runtimePCs [4]uintptr
