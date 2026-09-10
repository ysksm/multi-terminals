package agentstatus

import (
	"fmt"
	"strconv"
	"strings"
)

// 領域指定(herdr の manifest.rs の region と同名・同義)。
//
//	whole_recent                                画面全体
//	bottom_lines(N) / bottom_non_empty_lines(N)  末尾 N 行 / 末尾の非空 N 行から末尾まで
//	top_non_empty_lines(N)                       先頭の非空 N 行まで
//	after_last_prompt_marker                     codex の最後のプロンプト行(›)より後
//	before_current_prompt_marker                 codex の現在プロンプト行より前
//	whole_recent_without_current_prompt_marker   現在プロンプトが見えていれば ""、無ければ全体
//	prompt_box_body / above_prompt_box           claude の罫線で囲まれた入力ボックス内 / その上
//	last_non_empty_above_prompt_box              入力ボックス上の最後の非空行
//	after_last_horizontal_rule                   最後の罫線より後
//	osc_title / osc_progress                     画面ではなく OSC の値

func validateRegion(spec string) error {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "osc_title", "osc_progress", "whole_recent", "after_last_prompt_marker",
		"before_current_prompt_marker", "whole_recent_without_current_prompt_marker",
		"prompt_box_body", "above_prompt_box", "last_non_empty_above_prompt_box",
		"after_last_horizontal_rule":
		return nil
	}
	for _, name := range []string{"bottom_lines", "bottom_non_empty_lines", "top_non_empty_lines"} {
		if _, ok := regionCount(spec, name); ok {
			return nil
		}
	}
	return fmt.Errorf("unknown region %q", spec)
}

func region(in Input, spec string) string {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "osc_title":
		return in.OSCTitle
	case "osc_progress":
		return in.OSCProgress
	}
	c := in.Screen
	switch spec {
	case "whole_recent":
		return c
	case "after_last_prompt_marker":
		return afterLastPromptMarker(c)
	case "before_current_prompt_marker":
		return beforeCurrentPromptMarker(c)
	case "whole_recent_without_current_prompt_marker":
		if _, ok := currentCodexPromptIndex(strings.Split(c, "\n")); ok {
			return ""
		}
		return c
	case "prompt_box_body":
		return promptBoxBody(c)
	case "above_prompt_box":
		return abovePromptBox(c)
	case "last_non_empty_above_prompt_box":
		return lastNonEmptyLine(abovePromptBox(c))
	case "after_last_horizontal_rule":
		return afterLastHorizontalRule(c)
	}
	if n, ok := regionCount(spec, "bottom_lines"); ok {
		return bottomLines(c, n)
	}
	if n, ok := regionCount(spec, "bottom_non_empty_lines"); ok {
		return bottomNonEmptyLines(c, n)
	}
	if n, ok := regionCount(spec, "top_non_empty_lines"); ok {
		return topNonEmptyLines(c, n)
	}
	return ""
}

func regionCount(spec, name string) (int, bool) {
	rest, ok := strings.CutPrefix(spec, name)
	if !ok || !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return 0, false
	}
	n, err := strconv.Atoi(rest[1 : len(rest)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func joinFrom(lines []string, i int) string {
	if i >= len(lines) {
		return ""
	}
	if i < 0 {
		i = 0
	}
	return strings.Join(lines[i:], "\n")
}

func bottomLines(c string, n int) string {
	lines := strings.Split(c, "\n")
	return joinFrom(lines, len(lines)-n)
}

func bottomNonEmptyLines(c string, n int) string {
	lines := strings.Split(c, "\n")
	seen := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		seen++
		if seen == n {
			return joinFrom(lines, i)
		}
	}
	if seen == 0 {
		return ""
	}
	return c
}

func topNonEmptyLines(c string, n int) string {
	lines := strings.Split(c, "\n")
	seen := 0
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		seen++
		if seen == n {
			return strings.Join(lines[:i+1], "\n")
		}
	}
	if seen == 0 {
		return ""
	}
	return c
}

// codex のプロンプト行(›)とブロック開始行(• ■ ✗ ✓)。
func codexPromptLine(l string) bool { return l == "›" || strings.HasPrefix(l, "› ") }
func codexBlockMarkerLine(l string) bool {
	return strings.HasPrefix(l, "•") || strings.HasPrefix(l, "■") ||
		strings.HasPrefix(l, "✗") || strings.HasPrefix(l, "✓")
}

func lastCodexPromptIndex(lines []string) (int, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		if codexPromptLine(lines[i]) {
			return i, true
		}
	}
	return 0, false
}

// currentCodexPromptIndex は最後のプロンプト行の後にブロック開始行が無い
// (=いまプロンプトが出ている)ときだけその位置を返す。
func currentCodexPromptIndex(lines []string) (int, bool) {
	i, ok := lastCodexPromptIndex(lines)
	if !ok {
		return 0, false
	}
	for _, l := range lines[i+1:] {
		if codexBlockMarkerLine(l) {
			return 0, false
		}
	}
	return i, true
}

func afterLastPromptMarker(c string) string {
	lines := strings.Split(c, "\n")
	i, ok := lastCodexPromptIndex(lines)
	if !ok {
		return c
	}
	return joinFrom(lines, i+1)
}

func beforeCurrentPromptMarker(c string) string {
	lines := strings.Split(c, "\n")
	i, ok := currentCodexPromptIndex(lines)
	if !ok {
		return c
	}
	return strings.Join(lines[:i], "\n")
}

// isHorizontalRule: "─" で始まり、"─" だけか 3 本以上の罫線。
func isHorizontalRule(l string) bool {
	t := strings.TrimSpace(l)
	if t == "" {
		return false
	}
	n := 0
	rest := t
	for strings.HasPrefix(rest, "─") {
		n++
		rest = rest[len("─"):]
	}
	if n == 0 {
		return false
	}
	return strings.TrimSpace(rest) == "" || n >= 3
}

// promptBoxTopBorderIndex は末尾から数えて 2 本目の罫線(=入力ボックス上辺)。
func promptBoxTopBorderIndex(lines []string) (int, bool) {
	count := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if isHorizontalRule(lines[i]) {
			count++
			if count == 2 {
				return i, true
			}
		}
	}
	return 0, false
}

func promptBoxBody(c string) string {
	lines := strings.Split(c, "\n")
	top, ok := promptBoxTopBorderIndex(lines)
	if !ok {
		return ""
	}
	end := len(lines)
	for j := top + 1; j < len(lines); j++ {
		if isHorizontalRule(lines[j]) {
			end = j
			break
		}
	}
	return strings.Join(lines[top+1:end], "\n")
}

func abovePromptBox(c string) string {
	lines := strings.Split(c, "\n")
	top, ok := promptBoxTopBorderIndex(lines)
	if !ok {
		return c
	}
	return strings.Join(lines[:top], "\n")
}

func afterLastHorizontalRule(c string) string {
	lines := strings.Split(c, "\n")
	last := -1
	for i, l := range lines {
		if isHorizontalRule(l) {
			last = i
		}
	}
	if last < 0 {
		return c
	}
	return joinFrom(lines, last+1)
}

func lastNonEmptyLine(c string) string {
	lines := strings.Split(c, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
