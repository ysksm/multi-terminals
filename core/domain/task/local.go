package task

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Source はタスクの取込元。空(旧データ)は jira として扱う。
type Source string

const (
	SourceJira  Source = "jira"  // Jira から取り込み(概要・ステータスは読み取り専用、同期で更新)
	SourceLocal Source = "local" // 手動で作成(概要・ステータス等をこのアプリで編集)
)

// Normalize は空を jira に寄せる。
func (s Source) Normalize() Source {
	if s == "" {
		return SourceJira
	}
	return s
}

// IsValid は既知の取込元か。
func (s Source) IsValid() bool {
	switch s.Normalize() {
	case SourceJira, SourceLocal:
		return true
	}
	return false
}

// LocalMeta はローカルタスクだけが持つ情報。概要・説明・優先度は Task.Jira
// (表示用の共通スナップショット)に入れ、ここには Jira に無い項目だけを置く。
type LocalMeta struct {
	Labels   []string `json:"labels"`
	Links    []string `json:"links"`
	StatusID string   `json:"statusId"` // Settings.LocalTask.Statuses の ID
}

// StatusCategory はローカルステータスの分類(Jira の statusCategory と同じ語彙)。
type StatusCategory string

const (
	CategoryNew           StatusCategory = "new"
	CategoryIndeterminate StatusCategory = "indeterminate"
	CategoryDone          StatusCategory = "done"
)

// LocalStatus はユーザー定義のステータス。
type LocalStatus struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category StatusCategory `json:"category"`
}

// LocalTaskSettings はローカルタスクの採番とステータス定義。
type LocalTaskSettings struct {
	Prefix   string        `json:"prefix"`   // 番号の接頭辞(例: T → T-001)
	Digits   int           `json:"digits"`   // ゼロ埋め桁数
	NextSeq  int           `json:"nextSeq"`  // 次に採番する番号
	Statuses []LocalStatus `json:"statuses"` // 並び順 = 表示順
}

// DefaultLocalTaskSettings は既定の採番とステータス。
func DefaultLocalTaskSettings() LocalTaskSettings {
	return LocalTaskSettings{
		Prefix:  "T",
		Digits:  3,
		NextSeq: 1,
		Statuses: []LocalStatus{
			{ID: "todo", Name: "To Do", Category: CategoryNew},
			{ID: "inprogress", Name: "In Progress", Category: CategoryIndeterminate},
			{ID: "done", Name: "Done", Category: CategoryDone},
		},
	}
}

var prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Validate は設定を検査する。
func (l LocalTaskSettings) Validate() error {
	if !prefixRe.MatchString(l.Prefix) {
		return fmt.Errorf("prefix must match [A-Z][A-Z0-9_]* (got %q)", l.Prefix)
	}
	if l.Digits < 1 || l.Digits > 6 {
		return errors.New("digits must be between 1 and 6")
	}
	if l.NextSeq < 1 {
		return errors.New("nextSeq must be >= 1")
	}
	if len(l.Statuses) == 0 {
		return errors.New("at least one status is required")
	}
	ids := map[string]bool{}
	for _, s := range l.Statuses {
		if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Name) == "" {
			return errors.New("status id and name must not be empty")
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate status id %q", s.ID)
		}
		ids[s.ID] = true
		switch s.Category {
		case CategoryNew, CategoryIndeterminate, CategoryDone:
		default:
			return fmt.Errorf("status %q has invalid category %q", s.Name, s.Category)
		}
	}
	return nil
}

// FormatKey は接頭辞と連番から番号を作る(T, 3, 7 → T-007)。
func (l LocalTaskSettings) FormatKey(seq int) string {
	return fmt.Sprintf("%s-%0*d", l.Prefix, l.Digits, seq)
}

// NextKey は次の番号を返す(採番は進めない)。
func (l LocalTaskSettings) NextKey() string { return l.FormatKey(l.NextSeq) }

// Status は ID からステータスを引く。無ければ先頭を返す(削除されたステータスの救済)。
func (l LocalTaskSettings) Status(id string) LocalStatus {
	for _, s := range l.Statuses {
		if s.ID == id {
			return s
		}
	}
	if len(l.Statuses) > 0 {
		return l.Statuses[0]
	}
	return LocalStatus{ID: "", Name: "—", Category: CategoryNew}
}

// SeqOfKey は番号が Prefix-N の形式なら N を返す。違えば (0, false)。
func (l LocalTaskSettings) SeqOfKey(key string) (int, bool) {
	rest, ok := strings.CutPrefix(key, l.Prefix+"-")
	if !ok || rest == "" {
		return 0, false
	}
	n := 0
	for _, r := range rest {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// LocalInput はローカルタスクの作成/編集入力。
type LocalInput struct {
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
	Links       []string `json:"links"`
	StatusID    string   `json:"statusId"`
}

// cleanList は空要素と重複を除き、前後の空白を落とす。
func cleanList(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// NewLocalTask は手動作成のタスクを生成する。key は採番済みの番号。
func NewLocalTask(id, key string, in LocalInput, now time.Time) (*Task, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return nil, errors.New("summary must not be empty")
	}
	t := &Task{
		ID:         id,
		JiraKey:    strings.ToUpper(strings.TrimSpace(key)),
		Source:     SourceLocal,
		LocalState: StateNone,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	t.applyLocal(in, now)
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// IsLocal は手動作成のタスクか。
func (t *Task) IsLocal() bool { return t.Source.Normalize() == SourceLocal }

// applyLocal は編集項目を反映する(検証は呼び出し側)。
func (t *Task) applyLocal(in LocalInput, now time.Time) {
	t.Jira.Summary = strings.TrimSpace(in.Summary)
	t.Jira.Description = in.Description
	t.Jira.Priority = strings.TrimSpace(in.Priority)
	t.Jira.FetchedAt = now
	t.Local = &LocalMeta{
		Labels:   cleanList(in.Labels),
		Links:    cleanList(in.Links),
		StatusID: strings.TrimSpace(in.StatusID),
	}
	t.UpdatedAt = now
}

// UpdateLocal はローカルタスクの概要・説明・優先度・ラベル・リンク・ステータスを更新する。
// Jira タスクには使えない。
func (t *Task) UpdateLocal(in LocalInput, now time.Time) error {
	if !t.IsLocal() {
		return errors.New("only local tasks can be edited; jira tasks are read-only")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errors.New("summary must not be empty")
	}
	t.applyLocal(in, now)
	return nil
}

// LinkJira はローカルタスクを Jira タスクに切り替える。番号は Jira のものになり、
// 概要等は Jira の写しで置き換わる。環境(作業フォルダ・ブランチ)は変えない。
func (t *Task) LinkJira(jiraKey string, snap JiraSnapshot, now time.Time) error {
	if !t.IsLocal() {
		return errors.New("task is already linked to jira")
	}
	key := strings.ToUpper(strings.TrimSpace(jiraKey))
	if !IsJiraKey(key) {
		return fmt.Errorf("invalid jira key %q", key)
	}
	t.JiraKey = key
	t.Source = SourceJira
	t.Jira = snap
	t.Local = nil
	t.UpdatedAt = now
	return nil
}
