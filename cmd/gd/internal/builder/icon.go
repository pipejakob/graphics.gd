package builder

import (
	"bytes"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gd/internal/project"

	"runtime.link/api/xray"
)

// ensureProjectIcon provisions the project icon the Android export
// plugin bakes into launcher icons. Without application/config/icon in
// project.godot the export fails with:
//
//	ERROR: No project icon specified. Please specify one in the
//	Project Settings under Application -> Config -> Icon
//
// New projects could carry it in the template, but graphics directories
// that predate the template (or hand-written ones) don't — so provision
// at export time instead: write the default icon.svg if the project has
// none (the same fix ios.go ships for its app icon) and point
// config/icon at it. A project that already sets config/icon is left
// completely alone.
func ensureProjectIcon() error {
	path := filepath.Join(project.GraphicsDirectory, "project.godot")
	src, err := os.ReadFile(path)
	if err != nil {
		return xray.New(err)
	}
	if bytes.Contains(src, []byte("config/icon=")) {
		return nil
	}
	if err := project.SetupIcon(); err != nil {
		return xray.New(err)
	}
	entry := []byte("\n\nconfig/icon=\"res://icon.svg\"")
	if header := bytes.Index(src, []byte("[application]")); header >= 0 {
		at := header + len("[application]")
		src = append(src[:at:at], append(entry, src[at:]...)...)
	} else {
		src = append(src, []byte("\n[application]\n\nconfig/icon=\"res://icon.svg\"\n")...)
	}
	if err := os.WriteFile(path, src, 0644); err != nil {
		return xray.New(err)
	}
	return nil
}
