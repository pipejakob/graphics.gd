//go:build gd

package fastcb

import _ "unsafe" // for go:linkname

// The compiler.gd fork carries the resident-callback machinery in its
// own runtime (no overlay involved) and publishes the hook PCs into
// runtime.graphicsFastcbPCs, which has a handshake linkname for this
// alias. The fork sets the `gd` build tag, so this pull only ever
// resolves against a runtime that defines the symbol.
//
//go:linkname runtimePCs runtime.graphicsFastcbPCs
var runtimePCs [4]uintptr
