//go:build !windows

package terminal

import "os"

// defaultShell returns the default login shell on Unix-like systems.
// It honors $SHELL and falls back to /bin/sh.
func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}
