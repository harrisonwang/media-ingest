//go:build !windows && !linux && !darwin && embedtools

package embedtools

// Fallback for other GOOS targets when building with `-tags embedtools`.
var embeddedBinaries = map[string][]byte{}
