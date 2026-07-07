package agentstatus

import (
	"testing"
	"time"
)

func TestClassifyState(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	idle := now.Add(-3 * time.Second) // WaitIdleThreshold(2s) 超え
	recent := now.Add(-500 * time.Millisecond)

	cases := []struct {
		name       string
		tail       string
		lastOutput time.Time
		want       State
	}{
		{"許可プロンプト + idle → wait", "Do you want to proceed?\n❯ 1. Yes", idle, StateWait},
		{"claude 選択プロンプト + idle → wait", "some output\n❯ 1. Yes\n  2. No", idle, StateWait},
		{"codex 承認プロンプト + idle → wait", "Allow command? [y/n]", idle, StateWait},
		{"プロンプトがあっても直近に出力あり → active", "Do you want to proceed?", recent, StateActive},
		{"プロンプトなし + idle → active", "$ compiling...", idle, StateActive},
		{"空の tail → active", "", idle, StateActive},
	}
	for _, c := range cases {
		if got := ClassifyState([]byte(c.tail), c.lastOutput, now); got != c.want {
			t.Errorf("%s: ClassifyState = %q, want %q", c.name, got, c.want)
		}
	}
}
