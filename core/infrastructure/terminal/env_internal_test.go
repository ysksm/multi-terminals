package terminal

import (
	"slices"
	"testing"
)

func TestEnsureTerminalEnvAddsUTF8LocaleWhenAbsent(t *testing.T) {
	env := ensureTerminalEnv([]string{"PATH=/usr/bin"})
	if !slices.Contains(env, "LANG=ja_JP.UTF-8") {
		t.Errorf("expected LANG=ja_JP.UTF-8 to be appended, got %v", env)
	}
}

func TestEnsureTerminalEnvKeepsExistingLocale(t *testing.T) {
	cases := []struct {
		name string
		set  string
	}{
		{"LANG set", "LANG=en_US.UTF-8"},
		{"LC_ALL set", "LC_ALL=C"},
		{"LC_CTYPE set", "LC_CTYPE=en_US.UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := ensureTerminalEnv([]string{tc.set})
			if !slices.Contains(env, tc.set) {
				t.Errorf("expected %q to be preserved, got %v", tc.set, env)
			}
			if slices.Contains(env, "LANG=ja_JP.UTF-8") && tc.set != "LANG=ja_JP.UTF-8" {
				t.Errorf("expected no LANG default when %q is set, got %v", tc.set, env)
			}
		})
	}
}
