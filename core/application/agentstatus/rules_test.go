package agentstatus

import "testing"

func detect(t *testing.T, tool string, in Input) Detection {
	t.Helper()
	m := ManifestFor(tool)
	if m == nil {
		t.Fatalf("no manifest for %s", tool)
	}
	return m.Detect(in)
}

func TestClaudeManifest(t *testing.T) {
	cases := []struct {
		name    string
		in      Input
		want    State
		rule    string
		visible string // "idle" / "blocker" / "working" / ""
	}{
		{
			"OSC タイトルのスピナー → working",
			Input{OSCTitle: "⠋ Claude Code", Screen: "anything"},
			StateWorking, "osc_title_working", "working",
		},
		{
			"esc to interrupt 行 → working",
			Input{Screen: "✻ Thinking…\n\n⏵ Reticulating splines… (12s · esc to interrupt)\n"},
			StateWorking, "live_turn_working", "working",
		},
		{
			"プロンプトボックスの ❯ → idle",
			Input{Screen: "done.\n────────────\n❯ \n────────────\n  ? for shortcuts"},
			StateIdle, "live_prompt_box", "idle",
		},
		{
			"Bash 許可プロンプト → blocked",
			Input{Screen: "Bash command\n  ls -la\nDo you want to proceed?\n❯ 1. Yes\n  2. Yes, and don't ask again for: ls\n  3. No\n\nEsc to cancel"},
			StateBlocked, "bash_permission_prompt", "blocker",
		},
		{
			"汎用許可プロンプト(罫線の後) → blocked",
			Input{Screen: "old stuff\n────────────\nEdit file foo.go\nDo you want to proceed?\n❯ 1. Yes\n  2. No\nEsc to cancel"},
			StateBlocked, "generic_permission_prompt", "blocker",
		},
		{
			"入力待ちフォーム(enter to confirm) → blocked",
			Input{Screen: "────────────\nWhich option?\n❯ A\n  B\nEnter to confirm · Esc to cancel"},
			StateBlocked, "live_blocked_form", "blocker",
		},
		{
			"トランスクリプト表示 → skip",
			Input{Screen: "...\nShowing detailed transcript · ctrl+o to toggle"},
			StateUnknown, "transcript_viewer", "",
		},
		{
			"OSC ✳ タイトル → idle",
			Input{OSCTitle: "✳ Claude Code", Screen: "random text"},
			StateIdle, "osc_title_idle", "idle",
		},
		{
			"何も一致しない → フォールバック idle(ライブ UI 無し)",
			Input{Screen: "$ ls\nfoo bar"},
			StateIdle, "", "",
		},
		{
			"スクロールバックに古い許可プロンプトが残っていても現在の ❯ が優先",
			Input{Screen: "Do you want to proceed?\n  1. Yes\n  2. No\n\n────────────\n❯ \n────────────"},
			StateIdle, "live_prompt_box", "idle",
		},
	}
	for _, c := range cases {
		got := detect(t, "claude", c.in)
		if got.State != c.want || got.MatchedRule != c.rule {
			t.Errorf("%s: got (%s, %q), want (%s, %q)", c.name, got.State, got.MatchedRule, c.want, c.rule)
		}
		vis := ""
		switch {
		case got.VisibleIdle:
			vis = "idle"
		case got.VisibleBlocker:
			vis = "blocker"
		case got.VisibleWorking:
			vis = "working"
		}
		if vis != c.visible {
			t.Errorf("%s: visible = %q, want %q", c.name, vis, c.visible)
		}
		if c.rule == "transcript_viewer" && !got.SkipStateUpdate {
			t.Errorf("%s: SkipStateUpdate should be true", c.name)
		}
	}
}

func TestCodexManifest(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want State
		rule string
	}{
		{"OSC Action Required → blocked", Input{OSCTitle: "Codex — Action Required"}, StateBlocked, "osc_title_blocked"},
		{"OSC スピナー → working", Input{OSCTitle: "⠹ codex"}, StateWorking, "osc_title_working"},
		{"Working (esc to interrupt) → working", Input{Screen: "• Working (3s • esc to interrupt)"}, StateWorking, "screen_working_fallback"},
		{"allow command? → blocked", Input{Screen: "› fix it\n• running\nAllow command? [y/n]"}, StateBlocked, "live_strong_blocker"},
		{"現在プロンプトが出ていれば弱い blocker は無視", Input{Screen: "Do you want to continue? yes\n› "}, StateIdle, ""},
		{"ディレクトリ信頼確認 → blocked", Input{Screen: "> You are in /tmp/x\n\nDo you trust the contents of this directory?\n"}, StateBlocked, "trust_directory"},
		{"OSC に通常タイトル → idle", Input{OSCTitle: "codex", Screen: "…"}, StateIdle, "osc_title_idle"},
	}
	for _, c := range cases {
		got := detect(t, "codex", c.in)
		if got.State != c.want || got.MatchedRule != c.rule {
			t.Errorf("%s: got (%s, %q), want (%s, %q)", c.name, got.State, got.MatchedRule, c.want, c.rule)
		}
	}
}

func TestRegions(t *testing.T) {
	screen := "a\n\nb\n─────\nbody1\nbody2\n─────\nfooter"
	in := Input{Screen: screen, OSCTitle: "T", OSCProgress: "4;0;0"}
	cases := map[string]string{
		"osc_title":                       "T",
		"osc_progress":                    "4;0;0",
		"whole_recent":                    screen,
		"bottom_lines(2)":                 "─────\nfooter",
		"bottom_non_empty_lines(3)":       "body2\n─────\nfooter",
		"top_non_empty_lines(2)":          "a\n\nb",
		"prompt_box_body":                 "body1\nbody2",
		"above_prompt_box":                "a\n\nb",
		"last_non_empty_above_prompt_box": "b",
		"after_last_horizontal_rule":      "footer",
		"bogus":                           "",
	}
	for spec, want := range cases {
		if got := region(in, spec); got != want {
			t.Errorf("region(%q) = %q, want %q", spec, got, want)
		}
	}
	if err := validateRegion("bogus"); err == nil {
		t.Error("validateRegion(bogus) should fail")
	}
	codex := "› do x\n• step\nout\n› \n"
	if got := region(Input{Screen: codex}, "after_last_prompt_marker"); got != "" {
		t.Errorf("after_last_prompt_marker = %q", got)
	}
	if got := region(Input{Screen: codex}, "whole_recent_without_current_prompt_marker"); got != "" {
		t.Errorf("current prompt visible → want empty, got %q", got)
	}
	if got := region(Input{Screen: "› do x\n• step"}, "before_current_prompt_marker"); got != "› do x\n• step" {
		t.Errorf("no current prompt → whole content, got %q", got)
	}
}

func TestCompileRejectsBadRuleAndRegion(t *testing.T) {
	if _, err := Compile(Manifest{Tool: "x", Rules: []Rule{{ID: "r", Region: "whole_recent", Gate: Gate{Regex: []string{"("}}}}}); err == nil {
		t.Error("bad regex should fail")
	}
	if _, err := Compile(Manifest{Tool: "x", Rules: []Rule{{ID: "r", Region: "nope"}}}); err == nil {
		t.Error("bad region should fail")
	}
}
