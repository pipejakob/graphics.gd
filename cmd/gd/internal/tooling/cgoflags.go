package tooling

import (
	"os"
	"strings"
)

// CGOCFlags returns the CGO_CFLAGS environment as an argument vector,
// so compiler invocations that bypass cgo (the zig-compiled stubs for
// iOS and Android, and any future direct compiles) honor the same
// flags every cgo build uses — including the deterministic
// floating-point flags gd injects at startup.
func CGOCFlags() []string {
	return strings.Fields(os.Getenv("CGO_CFLAGS"))
}

// CGOLDFlags returns the CGO_LDFLAGS environment as an argument vector
// for link steps performed by an external tool (zig) instead of
// `go build`, which would otherwise drop the user's environment-level
// linker flags. Package-level #cgo LDFLAGS directives are forwarded
// separately by the builders.
func CGOLDFlags() []string {
	return strings.Fields(os.Getenv("CGO_LDFLAGS"))
}
