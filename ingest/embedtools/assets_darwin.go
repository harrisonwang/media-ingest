//go:build darwin && embedtools

package embedtools

import (
	"embed"
	"strings"
)

// macOS embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/darwin/yt-dlp
//   assets/darwin/ffmpeg
//   assets/darwin/ffprobe (optional but recommended)
//   assets/darwin/deno

//go:embed assets/darwin/*
var embeddedFS embed.FS

var embeddedBinaries = map[string][]byte{}

func init() {
	entries, err := embeddedFS.ReadDir("assets/darwin")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		b, err := embeddedFS.ReadFile("assets/darwin/" + name)
		if err != nil || len(b) == 0 {
			continue
		}
		key := strings.ToLower(name)
		embeddedBinaries[key] = b
	}
}
