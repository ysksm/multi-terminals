package task

import (
	"strings"
	"testing"
	"time"
)

var localNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func TestSource_NormalizeAndValidate(t *testing.T) {
	if Source("").Normalize() != SourceJira {
		t.Error("空の source は jira")
	}
	if !Source("").IsValid() || !SourceLocal.IsValid() || !SourceJira.IsValid() {
		t.Error("既知の source は valid")
	}
	if Source("github").IsValid() {
		t.Error("未知の source は invalid")
	}
}

func TestSettings_NormalizedFillsLocalTaskDefaults(t *testing.T) {
	old := Settings{AutoFetchBeforeSetup: false, IncludeNodeModules: true}
	got := old.Normalized()
	if got.LocalTask.Prefix != "T" || got.LocalTask.Digits != 3 || got.LocalTask.NextSeq != 1 || len(got.LocalTask.Statuses) != 3 {
		t.Errorf("既定の LocalTask が補われない: %+v", got.LocalTask)
	}
	if got.AutoFetchBeforeSetup || !got.IncludeNodeModules {
		t.Error("他の設定値は保持される")
	}
	// 設定済みなら触らない
	custom := DefaultSettings()
	custom.LocalTask.Prefix = "X"
	if custom.Normalized().LocalTask.Prefix != "X" {
		t.Error("設定済みの LocalTask は上書きしない")
	}
}

func TestLocalTaskSettings_Validate(t *testing.T) {
	ok := DefaultLocalTaskSettings()
	if err := ok.Validate(); err != nil {
		t.Fatalf("既定は valid: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*LocalTaskSettings)
	}{
		{"小文字の接頭辞", func(l *LocalTaskSettings) { l.Prefix = "t" }},
		{"数字始まりの接頭辞", func(l *LocalTaskSettings) { l.Prefix = "1T" }},
		{"ハイフンを含む接頭辞", func(l *LocalTaskSettings) { l.Prefix = "T-X" }},
		{"桁数 0", func(l *LocalTaskSettings) { l.Digits = 0 }},
		{"桁数 7", func(l *LocalTaskSettings) { l.Digits = 7 }},
		{"NextSeq 0", func(l *LocalTaskSettings) { l.NextSeq = 0 }},
		{"ステータス無し", func(l *LocalTaskSettings) { l.Statuses = nil }},
		{"ID 重複", func(l *LocalTaskSettings) { l.Statuses[1].ID = l.Statuses[0].ID }},
		{"名前が空", func(l *LocalTaskSettings) { l.Statuses[0].Name = " " }},
		{"不正なカテゴリ", func(l *LocalTaskSettings) { l.Statuses[0].Category = "closed" }},
	}
	for _, c := range cases {
		l := DefaultLocalTaskSettings()
		c.mut(&l)
		if err := l.Validate(); err == nil {
			t.Errorf("%s: エラーになるべき", c.name)
		}
	}
}

func TestLocalTaskSettings_KeyHelpers(t *testing.T) {
	l := LocalTaskSettings{Prefix: "T", Digits: 3, NextSeq: 7}
	if got := l.FormatKey(7); got != "T-007" {
		t.Errorf("FormatKey = %q", got)
	}
	if got := l.FormatKey(1234); got != "T-1234" {
		t.Errorf("桁数を超えても切らない: %q", got)
	}
	if got := l.NextKey(); got != "T-007" {
		t.Errorf("NextKey = %q", got)
	}
	for in, want := range map[string]int{"T-007": 7, "T-10": 10, "T-": -1, "PROJ-7": -1, "T-7a": -1, "TT-7": -1} {
		n, ok := l.SeqOfKey(in)
		if want < 0 {
			if ok {
				t.Errorf("SeqOfKey(%q) は不一致のはず", in)
			}
			continue
		}
		if !ok || n != want {
			t.Errorf("SeqOfKey(%q) = %d,%v want %d", in, n, ok, want)
		}
	}
}

func TestLocalTaskSettings_Status(t *testing.T) {
	l := DefaultLocalTaskSettings()
	if l.Status("inprogress").Name != "In Progress" {
		t.Error("ID で引ける")
	}
	if l.Status("gone").ID != "todo" {
		t.Error("無い ID は先頭にフォールバック")
	}
	if got := (LocalTaskSettings{}).Status("x"); got.Name != "—" || got.Category != CategoryNew {
		t.Errorf("ステータス未定義のときの救済: %+v", got)
	}
}

func TestNewLocalTask(t *testing.T) {
	in := LocalInput{Summary: "  Docker 化  ", Description: "desc", Priority: "High", Labels: []string{"infra", " ", "infra", "dx"}, Links: []string{"https://a", "", "https://a"}, StatusID: "todo"}
	tk, err := NewLocalTask("id1", "t-007", in, localNow)
	if err != nil {
		t.Fatal(err)
	}
	if tk.JiraKey != "T-007" || tk.Source != SourceLocal || !tk.IsLocal() || tk.LocalState != StateNone {
		t.Errorf("基本項目: %+v", tk)
	}
	if tk.Jira.Summary != "Docker 化" || tk.Jira.Description != "desc" || tk.Jira.Priority != "High" {
		t.Errorf("表示用スナップショット: %+v", tk.Jira)
	}
	if strings.Join(tk.Local.Labels, ",") != "infra,dx" || strings.Join(tk.Local.Links, ",") != "https://a" || tk.Local.StatusID != "todo" {
		t.Errorf("ラベル・リンクの正規化: %+v", tk.Local)
	}
	if _, err := NewLocalTask("id2", "T-008", LocalInput{Summary: "  "}, localNow); err == nil {
		t.Error("概要が空はエラー")
	}
	if _, err := NewLocalTask("id3", "007", LocalInput{Summary: "x"}, localNow); err == nil {
		t.Error("番号形式でないキーはエラー")
	}
}

func TestTask_Validate_LocalRequiresMeta(t *testing.T) {
	tk, _ := NewTask("id", "PROJ-1", JiraSnapshot{Summary: "s"}, localNow)
	if tk.IsLocal() {
		t.Error("NewTask は jira")
	}
	tk.Source = SourceLocal
	if err := tk.Validate(); err == nil {
		t.Error("local なのに Local が nil はエラー")
	}
	tk.Source = "github"
	if err := tk.Validate(); err == nil {
		t.Error("未知の source はエラー")
	}
}

func TestTask_UpdateLocal(t *testing.T) {
	tk, _ := NewLocalTask("id1", "T-001", LocalInput{Summary: "before", StatusID: "todo"}, localNow)
	later := localNow.Add(time.Minute)
	err := tk.UpdateLocal(LocalInput{Summary: "after", Priority: "Low", Labels: []string{"a"}, StatusID: "done"}, later)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Jira.Summary != "after" || tk.Jira.Priority != "Low" || tk.Local.StatusID != "done" || len(tk.Local.Labels) != 1 || !tk.UpdatedAt.Equal(later) {
		t.Errorf("更新結果: %+v %+v", tk.Jira, tk.Local)
	}
	if err := tk.UpdateLocal(LocalInput{Summary: ""}, later); err == nil {
		t.Error("空の概要はエラー")
	}
	jira, _ := NewTask("id2", "PROJ-1", JiraSnapshot{Summary: "s"}, localNow)
	if err := jira.UpdateLocal(LocalInput{Summary: "x"}, later); err == nil {
		t.Error("jira タスクは編集できない")
	}
}

func TestTask_LinkJira(t *testing.T) {
	tk, _ := NewLocalTask("id1", "T-001", LocalInput{Summary: "local", StatusID: "todo"}, localNow)
	env := Env{WorkDir: "~/work/T-001", Branch: "feature/T-001-local", Layout: "single", Panes: []PaneSpec{{Slot: 0}}}
	if err := tk.ApplyEnv(env, localNow); err != nil {
		t.Fatal(err)
	}
	snap := JiraSnapshot{Summary: "from jira", Status: "To Do", StatusCategory: "new", URL: "https://j/browse/PROJ-9"}
	if err := tk.LinkJira("proj-9", snap, localNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if tk.JiraKey != "PROJ-9" || tk.Source != SourceJira || tk.IsLocal() || tk.Local != nil {
		t.Errorf("切替結果: %+v", tk)
	}
	if tk.Jira.Summary != "from jira" || tk.Env.Branch != "feature/T-001-local" || tk.Env.WorkDir != "~/work/T-001" {
		t.Errorf("スナップショット差し替え・環境維持: %+v %+v", tk.Jira, tk.Env)
	}
	if err := tk.Validate(); err != nil {
		t.Errorf("切替後も valid: %v", err)
	}
	if err := tk.LinkJira("PROJ-10", snap, localNow); err == nil {
		t.Error("既に jira のタスクはエラー")
	}
	other, _ := NewLocalTask("id2", "T-002", LocalInput{Summary: "x"}, localNow)
	if err := other.LinkJira("bad key", snap, localNow); err == nil {
		t.Error("不正な番号はエラー")
	}
}
