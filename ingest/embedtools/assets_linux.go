//go:build linux && embedtools

package embedtools

import (
	"embed"
	"strings"
)

// Linux embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/linux/yt-dlp
//   assets/linux/ffmpeg
//   assets/linux/ffprobe (optional but recommended)
//   assets/linux/deno

//go:embed assets/linux/*
var embeddedFS embed.FS

var embeddedBinaries = map[string][]byte{}

func init() {
	entries, err := embeddedFS.ReadDir("assets/linux")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		b, err := embeddedFS.ReadFile("assets/linux/" + name)
		if err != nil || len(b) == 0 {
			continue
		}
		key := strings.ToLower(name)
		embeddedBinaries[key] = b
	}
}
