//go:build windows

package terminal_test

// OS-specific shell and commands for the real-shell integration tests.
// cmd.exe is used for tests because its echo / cd output is simple and stable.
const testShell = "cmd.exe"

func echoLine(marker string) []byte { return []byte("echo " + marker + "\r\n") }

// cmd.exe prints the current directory when `cd` is run with no arguments.
func pwdLine() []byte { return []byte("cd\r\n") }
