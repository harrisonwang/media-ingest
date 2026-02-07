//go:build windows && embedtools

package embedtools

import _ "embed"

// Windows embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/windows/yt-dlp.exe
//   assets/windows/ffmpeg.exe
//   assets/windows/deno.exe

//go:embed assets/windows/yt-dlp.exe
var embeddedYtDlp []byte

//go:embed assets/windows/ffmpeg.exe
var embeddedFFmpeg []byte

//go:embed assets/windows/deno.exe
var embeddedDeno []byte

// Optional: if you want to embed node.exe, add a new file with a build tag
// (e.g. windows && embedtools && embednode) to avoid breaking builds when the
// file is missing.
var embeddedNode []byte

var embeddedBinaries = map[string][]byte{
	"yt-dlp": embeddedYtDlp,
	"ffmpeg": embeddedFFmpeg,
	"deno":   embeddedDeno,
	"node":   embeddedNode,
}
