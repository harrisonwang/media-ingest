//go:build windows && embedtools

package embedtools

import (
	"embed"
	"strings"
)

// Windows embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/windows/yt-dlp.exe
//   assets/windows/ffmpeg.exe
//   assets/windows/ffprobe.exe (optional but recommended)
//   assets/windows/deno.exe

//go:embed assets/windows/*
var embeddedFS embed.FS

// embeddedBinaries is populated from embeddedFS at init time.
// Keys are canonical tool names without extension (e.g. "ffmpeg", "ffprobe").
var embeddedBinaries = map[string][]byte{}

func init() {
	entries, err := embeddedFS.ReadDir("assets/windows")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		b, err := embeddedFS.ReadFile("assets/windows/" + name)
		if err != nil || len(b) == 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(name, ".exe"))
		embeddedBinaries[key] = b
	}
}
