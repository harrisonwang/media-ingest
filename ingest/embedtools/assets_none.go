//go:build !embedtools

package embedtools

// Default build: no embedded tool binaries.
var embeddedBinaries = map[string][]byte{}
