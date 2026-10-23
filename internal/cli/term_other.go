//go:build !windows

package cli

import "os"

// enableVT reports whether the terminal understands ANSI escapes; Unix
// terminals do.
func enableVT(*os.File) bool { return true }
