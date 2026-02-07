//go:build darwin && embedtools

package embedtools

import _ "embed"

// macOS embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/darwin/yt-dlp
//   assets/darwin/ffmpeg
//   assets/darwin/deno

//go:embed assets/darwin/yt-dlp
var embeddedYtDlp []byte

//go:embed assets/darwin/ffmpeg
var embeddedFFmpeg []byte

//go:embed assets/darwin/deno
var embeddedDeno []byte

var embeddedNode []byte

var embeddedBinaries = map[string][]byte{
	"yt-dlp": embeddedYtDlp,
	"ffmpeg": embeddedFFmpeg,
	"deno":   embeddedDeno,
	"node":   embeddedNode,
}
