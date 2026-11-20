//go:build !windows

package engine

import "os"

// setSparse is a no-op: Truncate already creates sparse files on Unix file systems.
func setSparse(*os.File) error { return nil }
