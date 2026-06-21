//go:build !windows

package terminal_test

// OS-specific shell and commands for the real-shell integration tests.
const testShell = "/bin/sh"

func echoLine(marker string) []byte { return []byte("echo " + marker + "\n") }

func pwdLine() []byte { return []byte("pwd\n") }
