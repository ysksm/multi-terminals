//go:build windows

package terminal

import "os"

// defaultShell returns the default shell on Windows.
// Preference order: $MULTI_TERMINALS_SHELL override, then PowerShell (present
// on all supported Windows versions and nicer than cmd.exe for interactive use).
func defaultShell() string {
	if s := os.Getenv("MULTI_TERMINALS_SHELL"); s != "" {
		return s
	}
	return "powershell.exe"
}
