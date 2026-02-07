//go:build linux && embedtools

package embedtools

import _ "embed"

// Linux embedded tools. These files are expected to exist at build time.
//
// Layout:
//   assets/linux/yt-dlp
//   assets/linux/ffmpeg
//   assets/linux/deno

//go:embed assets/linux/yt-dlp
var embeddedYtDlp []byte

//go:embed assets/linux/ffmpeg
var embeddedFFmpeg []byte

//go:embed assets/linux/deno
var embeddedDeno []byte

var embeddedNode []byte

var embeddedBinaries = map[string][]byte{
	"yt-dlp": embeddedYtDlp,
	"ffmpeg": embeddedFFmpeg,
	"deno":   embeddedDeno,
	"node":   embeddedNode,
}
