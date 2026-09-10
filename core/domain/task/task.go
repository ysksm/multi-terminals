// Package task はタスク管理(Jira 連携 + 環境セットアップ)のドメインモデル。
//
// Workspace 集約(core/domain)とは独立した集約群で、Workspace 側は任意の taskId を
// 持つだけにして既存機能を壊さない。集約の数が多く、フィールドの大半が単純な
// 文字列設定値なので、Workspace のような Value Object + 非公開フィールド方式ではなく
// 「公開フィールド + Validate()」方式を採る。不変条件は Validate と状態遷移
// メソッドに集約し、永続化層・アプリ層はそれを通す。
package task

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrNotFound は対象レコードが存在しないことを示す(全リポジトリ共通)。
var ErrNotFound = errors.New("not found")

// ErrDuplicateKey は同じ Jira 番号のタスクが既にあることを示す。
var ErrDuplicateKey = errors.New("task with the same jira key already exists")

// LocalState はタスクのローカル状態。working(作業中)は保存せず、
// 紐付くワークスペースのライブセッション有無から一覧クエリで導出する。
type LocalState string

const (
	StateNone      LocalState = "none"      // 未準備(作業フォルダなし)
	StatePreparing LocalState = "preparing" // セットアップ実行中
	StateReady     LocalState = "ready"     // 環境あり
	StateDone      LocalState = "done"      // 完了(手動)
)

// IsValid は既知の状態か。
func (s LocalState) IsValid() bool {
	switch s {
	case StateNone, StatePreparing, StateReady, StateDone:
		return true
	}
	return false
}

// SourceKind はリポジトリの取得方法。
type SourceKind string

const (
	SourceClone    SourceKind = "clone"    // git clone
	SourceBaseCopy SourceKind = "baseCopy" // ベースクローンからコピー
)

// RepoSource はリポジトリの取得元。Kind に応じて URL または BaseCloneID を使う。
type RepoSource struct {
	Kind        SourceKind `json:"kind"`
	URL         string     `json:"url,omitempty"`
	BaseCloneID string     `json:"baseCloneId,omitempty"`
}

// Validate は取得元の整合性を検査する。
func (s RepoSource) Validate() error {
	switch s.Kind {
	case SourceClone:
		if strings.TrimSpace(s.URL) == "" {
			return errors.New("clone source requires url")
		}
	case SourceBaseCopy:
		if strings.TrimSpace(s.BaseCloneID) == "" {
			return errors.New("baseCopy source requires baseCloneId")
		}
	default:
		return fmt.Errorf("invalid source kind %q", s.Kind)
	}
	return nil
}

// Repo はタスク/テンプレートに含まれる 1 リポジトリ。Name は作業フォルダ直下の
// ディレクトリ名を兼ねる。SetupCommands は取得後に順に実行するコマンド。
type Repo struct {
	Name          string     `json:"name"`
	Source        RepoSource `json:"source"`
	SetupCommands []string   `json:"setupCommands"`
}

var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Validate はリポジトリ定義を検査する。Name はパス区切りを含めない 1 セグメント。
func (r Repo) Validate() error {
	if !repoNameRe.MatchString(r.Name) || r.Name == "." || r.Name == ".." {
		return fmt.Errorf("invalid repo name %q", r.Name)
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("repo %q: %w", r.Name, err)
	}
	return nil
}

// Command はペインの起動コマンド候補(Workspace の StartupCommand と同じ意味)。
type Command struct {
	Command string `json:"command"`
	AutoRun bool   `json:"autoRun"`
}

// PaneSpec はワークスペース作成時のペイン割当。RepoName が空なら作業フォルダ直下。
type PaneSpec struct {
	Slot     int       `json:"slot"`
	RepoName string    `json:"repoName"`
	Commands []Command `json:"commands"`
}

// Env はタスクの作業環境(テンプレートから展開したもの。タスクごとに編集可)。
type Env struct {
	TemplateID string     `json:"templateId,omitempty"`
	WorkDir    string     `json:"workDir"`
	Branch     string     `json:"branch"`
	Layout     string     `json:"layout"` // domain.LayoutPreset の文字列
	Repos      []Repo     `json:"repos"`
	Panes      []PaneSpec `json:"panes"`
}

// IsEmpty は環境が未設定か(テンプレート「なし」で取り込んだ直後)。
func (e Env) IsEmpty() bool {
	return strings.TrimSpace(e.WorkDir) == "" && len(e.Repos) == 0 && len(e.Panes) == 0
}

// Validate は環境定義を検査する。未設定(IsEmpty)は許す。
func (e Env) Validate() error {
	if e.IsEmpty() {
		return nil
	}
	if strings.TrimSpace(e.WorkDir) == "" {
		return errors.New("workDir is required")
	}
	return validateReposAndPanes(e.Repos, e.Panes, e.Layout)
}

// validateReposAndPanes はリポジトリ名の一意性とペイン割当の参照整合性を検査する。
func validateReposAndPanes(repos []Repo, panes []PaneSpec, layout string) error {
	names := map[string]bool{}
	for _, r := range repos {
		if err := r.Validate(); err != nil {
			return err
		}
		if names[r.Name] {
			return fmt.Errorf("duplicate repo name %q", r.Name)
		}
		names[r.Name] = true
	}
	slots := map[int]bool{}
	for _, p := range panes {
		if p.Slot < 0 {
			return fmt.Errorf("pane slot must be >= 0, got %d", p.Slot)
		}
		if slots[p.Slot] {
			return fmt.Errorf("duplicate pane slot %d", p.Slot)
		}
		slots[p.Slot] = true
		if p.RepoName != "" && !names[p.RepoName] {
			return fmt.Errorf("pane slot %d refers to unknown repo %q", p.Slot, p.RepoName)
		}
		for _, c := range p.Commands {
			if strings.TrimSpace(c.Command) == "" {
				return fmt.Errorf("pane slot %d has an empty command", p.Slot)
			}
		}
	}
	if len(panes) > 0 && strings.TrimSpace(layout) == "" {
		return errors.New("layout is required when panes are set")
	}
	return nil
}

// JiraSnapshot は取り込み/同期時点の Jira issue の写し(読み取り専用)。
// ローカルタスク(Source=local)では Jira は無く、ユーザーが編集した概要・説明・優先度を
// 同じ構造に入れて表示の共通化に使う(Status / StatusCategory は設定のステータス定義から
// 表示時に埋める)。
type JiraSnapshot struct {
	Summary        string    `json:"summary"`
	Status         string    `json:"status"`
	StatusCategory string    `json:"statusCategory"` // new | indeterminate | done
	Assignee       string    `json:"assignee"`
	Priority       string    `json:"priority"`
	Epic           string    `json:"epic"`
	Sprint         string    `json:"sprint"`
	Description    string    `json:"description"`
	URL            string    `json:"url"`
	FetchedAt      time.Time `json:"fetchedAt"`
}

// Task は Jira issue に対応するローカルの作業単位(集約ルート)。
type Task struct {
	ID string `json:"id"`
	// JiraKey は Jira 番号。ローカルタスクでは採番した "T-007" 形式の番号(一意)。
	JiraKey string `json:"jiraKey"`
	// Source は取込元(jira | local)。空は jira(旧データ)。
	Source Source `json:"source,omitempty"`
	// Local はローカルタスクだけが持つ項目(ラベル・リンク・ステータス ID)。
	Local       *LocalMeta   `json:"local,omitempty"`
	Jira        JiraSnapshot `json:"jira"`
	Note        string       `json:"note"`
	LocalState  LocalState   `json:"localState"`
	Env         Env          `json:"env"`
	WorkspaceID string       `json:"workspaceId,omitempty"`
	SetupRunID  string       `json:"setupRunId,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// GetID は Repository[T] 用の識別子。
func (t *Task) GetID() string { return t.ID }

// jiraKeyRe は "PROJ-1301" 形式。プロジェクトキーは英大文字始まり。
var jiraKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-\d+$`)

// IsJiraKey は文字列が Jira 番号の形式か。
func IsJiraKey(s string) bool { return jiraKeyRe.MatchString(s) }

// NewTask は取り込み直後のタスクを生成する(未準備)。
func NewTask(id, jiraKey string, snap JiraSnapshot, now time.Time) (*Task, error) {
	t := &Task{
		ID:         id,
		JiraKey:    strings.ToUpper(strings.TrimSpace(jiraKey)),
		Jira:       snap,
		LocalState: StateNone,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// Validate は不変条件を検査する。
func (t *Task) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("task id must not be empty")
	}
	if !IsJiraKey(t.JiraKey) {
		return fmt.Errorf("invalid jira key %q", t.JiraKey)
	}
	if !t.Source.IsValid() {
		return fmt.Errorf("invalid source %q", t.Source)
	}
	if t.IsLocal() && t.Local == nil {
		return errors.New("local task requires local meta")
	}
	if !t.LocalState.IsValid() {
		return fmt.Errorf("invalid local state %q", t.LocalState)
	}
	if err := t.Env.Validate(); err != nil {
		return fmt.Errorf("env: %w", err)
	}
	return nil
}

// ApplyEnv は作業環境を置き換える。完了・準備中のタスクでも編集は許す
// (再セットアップに備える)。
func (t *Task) ApplyEnv(env Env, now time.Time) error {
	if err := env.Validate(); err != nil {
		return fmt.Errorf("env: %w", err)
	}
	t.Env = env
	t.UpdatedAt = now
	return nil
}

// UpdateJira は Jira の写しを差し替える(同期)。
func (t *Task) UpdateJira(snap JiraSnapshot, now time.Time) {
	t.Jira = snap
	t.UpdatedAt = now
}

// SetNote はローカルメモを更新する。
func (t *Task) SetNote(note string, now time.Time) {
	t.Note = note
	t.UpdatedAt = now
}

// BeginSetup はセットアップ開始を記録する(準備中)。準備中の二重開始はエラー。
func (t *Task) BeginSetup(runID string, now time.Time) error {
	if t.LocalState == StatePreparing {
		return errors.New("setup is already running")
	}
	if t.Env.IsEmpty() {
		return errors.New("env is not configured")
	}
	t.LocalState = StatePreparing
	t.SetupRunID = runID
	t.UpdatedAt = now
	return nil
}

// FinishSetup はセットアップ完了を記録する(環境あり)。作成したワークスペースを紐付ける。
func (t *Task) FinishSetup(workspaceID string, now time.Time) {
	t.LocalState = StateReady
	if workspaceID != "" {
		t.WorkspaceID = workspaceID
	}
	t.UpdatedAt = now
}

// FailSetup はセットアップの失敗/中止を記録する。フォルダが一部できていても
// 「未準備」に戻す(再実行はステップ単位で行う)。
func (t *Task) FailSetup(now time.Time) {
	if t.LocalState == StatePreparing {
		t.LocalState = StateNone
	}
	t.UpdatedAt = now
}

// MarkDone は手動で完了にする。準備中は不可。
func (t *Task) MarkDone(now time.Time) error {
	if t.LocalState == StatePreparing {
		return errors.New("cannot mark done while setup is running")
	}
	t.LocalState = StateDone
	t.UpdatedAt = now
	return nil
}

// Reopen は完了を取り消す。環境(作業フォルダ)があれば ready、なければ none。
func (t *Task) Reopen(hasWorkDir bool, now time.Time) {
	if t.LocalState != StateDone {
		return
	}
	if hasWorkDir {
		t.LocalState = StateReady
	} else {
		t.LocalState = StateNone
	}
	t.UpdatedAt = now
}

// UnlinkWorkspace はワークスペース削除時に紐付けを外す。状態は変えない(フォルダは残る)。
func (t *Task) UnlinkWorkspace(now time.Time) {
	t.WorkspaceID = ""
	t.UpdatedAt = now
}

// LinkWorkspace はワークスペースを紐付ける。
func (t *Task) LinkWorkspace(workspaceID string, now time.Time) {
	t.WorkspaceID = workspaceID
	t.UpdatedAt = now
}

// ClearWorkDir は作業フォルダ削除後の状態にする(未準備。完了なら完了のまま)。
func (t *Task) ClearWorkDir(now time.Time) {
	if t.LocalState == StateReady {
		t.LocalState = StateNone
	}
	t.UpdatedAt = now
}
