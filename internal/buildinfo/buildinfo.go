// Package buildinfo exposes non-consensus software build metadata.
// Values may be replaced with -ldflags at build time. They never enter
// canonical state, transaction IDs, object IDs, or application hashes.
package buildinfo

import "fmt"

var (
	Version   = "v0.1.0-alpha.1"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func String(program string) string {
	return fmt.Sprintf("%s %s (commit=%s built=%s)", program, Version, Commit, BuildDate)
}
