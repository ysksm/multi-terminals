package task

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Template は環境テンプレート。取り込み時に Jira 番号と概要でパターンを展開し、
// Task.Env を作る。
type Template struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	IsDefault      bool       `json:"isDefault"`
	WorkDirPattern string     `json:"workDirPattern"` // 例: ~/work/{KEY}
	BranchPattern  string     `json:"branchPattern"`  // 例: feature/{KEY}-{slug}
	Layout         string     `json:"layout"`         // domain.LayoutPreset の文字列
	Repos          []Repo     `json:"repos"`
	Panes          []PaneSpec `json:"panes"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// GetID は Repository[T] 用の識別子。
func (t *Template) GetID() string { return t.ID }

// Validate は不変条件を検査する。
func (t *Template) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("template id must not be empty")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("template name must not be empty")
	}
	if strings.TrimSpace(t.WorkDirPattern) == "" {
		return errors.New("workDirPattern must not be empty")
	}
	if !strings.Contains(t.WorkDirPattern, "{KEY}") {
		return errors.New("workDirPattern must contain {KEY} so that tasks get distinct folders")
	}
	if strings.TrimSpace(t.BranchPattern) == "" {
		return errors.New("branchPattern must not be empty")
	}
	if strings.TrimSpace(t.Layout) == "" {
		return errors.New("layout must not be empty")
	}
	return validateReposAndPanes(t.Repos, t.Panes, t.Layout)
}

// Expand はテンプレートを Jira 番号と概要で展開し、タスクの環境を作る。
// {KEY} は番号、{slug} は概要から作った slug。slug が空なら "-{slug}" ごと落とす。
func (t *Template) Expand(jiraKey, summary string) Env {
	vars := map[string]string{"KEY": jiraKey, "slug": Slugify(summary)}
	repos := make([]Repo, len(t.Repos))
	for i, r := range t.Repos {
		repos[i] = r
		repos[i].SetupCommands = append([]string(nil), r.SetupCommands...)
	}
	panes := make([]PaneSpec, len(t.Panes))
	for i, p := range t.Panes {
		panes[i] = p
		panes[i].Commands = append([]Command(nil), p.Commands...)
	}
	return Env{
		TemplateID: t.ID,
		WorkDir:    ExpandPattern(t.WorkDirPattern, vars),
		Branch:     ExpandPattern(t.BranchPattern, vars),
		Layout:     t.Layout,
		Repos:      repos,
		Panes:      panes,
	}
}

// ExpandPattern は {NAME} 形式の変数を置換する。値が空の変数は、直前の "-" / "_"
// ごと取り除く(feature/{KEY}-{slug} → feature/PROJ-1)。未知の変数はそのまま残す。
func ExpandPattern(pattern string, vars map[string]string) string {
	out := pattern
	for name, val := range vars {
		ph := "{" + name + "}"
		if val == "" {
			out = strings.ReplaceAll(out, "-"+ph, "")
			out = strings.ReplaceAll(out, "_"+ph, "")
			out = strings.ReplaceAll(out, ph, "")
			continue
		}
		out = strings.ReplaceAll(out, ph, val)
	}
	return out
}

// SlugMaxLen は slug の最大長。
const SlugMaxLen = 30

// Slugify は概要から ASCII 英数字だけを抜き出し、小文字ハイフン区切りの slug を作る。
// 日本語だけの概要では空になる(呼び出し側が省略する)。
func Slugify(s string) string {
	var b strings.Builder
	lastDash := true // 先頭のダッシュを抑止
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > SlugMaxLen {
		out = strings.TrimRight(out[:SlugMaxLen], "-")
	}
	return out
}

// DefaultTemplate は初回起動時に用意する既定テンプレート(リポジトリなし)。
// ユーザーが編集して使う土台で、そのままでも「作業フォルダ + 1 画面」の環境ができる。
func DefaultTemplate(id string, now time.Time) *Template {
	return &Template{
		ID:             id,
		Name:           "標準",
		IsDefault:      true,
		WorkDirPattern: "~/work/{KEY}",
		BranchPattern:  "feature/{KEY}-{slug}",
		Layout:         "single",
		Repos:          []Repo{},
		Panes:          []PaneSpec{{Slot: 0, RepoName: "", Commands: []Command{}}},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ErrTemplateInUse は将来の参照チェック用(現状はテンプレート削除でタスクは壊れない)。
var ErrTemplateInUse = fmt.Errorf("template is in use")
