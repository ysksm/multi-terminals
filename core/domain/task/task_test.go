package task

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func TestIsJiraKey(t *testing.T) {
	for k, want := range map[string]bool{
		"PROJ-1301": true, "A-1": true, "AB_C-42": true,
		"proj-1301": false, "PROJ1301": false, "PROJ-": false, "1PROJ-1": false, "": false,
	} {
		if got := IsJiraKey(k); got != want {
			t.Errorf("IsJiraKey(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestNewTask(t *testing.T) {
	tk, err := NewTask("t1", " proj-1 ", JiraSnapshot{Summary: "s"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if tk.JiraKey != "PROJ-1" || tk.LocalState != StateNone || !tk.CreatedAt.Equal(t0) {
		t.Errorf("unexpected task: %+v", tk)
	}
	if _, err := NewTask("", "PROJ-1", JiraSnapshot{}, t0); err == nil {
		t.Error("empty id should fail")
	}
	if _, err := NewTask("t", "bad", JiraSnapshot{}, t0); err == nil {
		t.Error("invalid key should fail")
	}
}

func TestTask_SetupTransitions(t *testing.T) {
	tk, _ := NewTask("t1", "PROJ-1", JiraSnapshot{}, t0)
	if err := tk.BeginSetup("r1", t0); err == nil {
		t.Fatal("BeginSetup with empty env should fail")
	}
	tk.Env = Env{WorkDir: "~/w/PROJ-1", Layout: "single"}
	if err := tk.BeginSetup("r1", t0); err != nil {
		t.Fatal(err)
	}
	if tk.LocalState != StatePreparing || tk.SetupRunID != "r1" {
		t.Errorf("state=%s run=%s", tk.LocalState, tk.SetupRunID)
	}
	if err := tk.BeginSetup("r2", t0); err == nil {
		t.Error("second BeginSetup should fail")
	}
	if err := tk.MarkDone(t0); err == nil {
		t.Error("MarkDone while preparing should fail")
	}
	tk.FailSetup(t0.Add(time.Minute))
	if tk.LocalState != StateNone {
		t.Errorf("FailSetup: state=%s", tk.LocalState)
	}
	_ = tk.BeginSetup("r3", t0)
	tk.FinishSetup("ws1", t0.Add(2*time.Minute))
	if tk.LocalState != StateReady || tk.WorkspaceID != "ws1" {
		t.Errorf("FinishSetup: %+v", tk)
	}
	// FailSetup は preparing 以外を変えない
	tk.FailSetup(t0)
	if tk.LocalState != StateReady {
		t.Errorf("FailSetup on ready changed state: %s", tk.LocalState)
	}
	if err := tk.MarkDone(t0); err != nil {
		t.Fatal(err)
	}
	if tk.LocalState != StateDone {
		t.Errorf("MarkDone: %s", tk.LocalState)
	}
	tk.Reopen(true, t0)
	if tk.LocalState != StateReady {
		t.Errorf("Reopen(true): %s", tk.LocalState)
	}
	_ = tk.MarkDone(t0)
	tk.Reopen(false, t0)
	if tk.LocalState != StateNone {
		t.Errorf("Reopen(false): %s", tk.LocalState)
	}
	// Reopen は done 以外には効かない
	tk.LocalState = StateReady
	tk.Reopen(false, t0)
	if tk.LocalState != StateReady {
		t.Errorf("Reopen on ready changed state: %s", tk.LocalState)
	}
}

func TestTask_WorkspaceAndWorkDir(t *testing.T) {
	tk, _ := NewTask("t1", "PROJ-1", JiraSnapshot{}, t0)
	tk.LinkWorkspace("ws1", t0)
	if tk.WorkspaceID != "ws1" {
		t.Fatal("LinkWorkspace")
	}
	tk.UnlinkWorkspace(t0)
	if tk.WorkspaceID != "" {
		t.Fatal("UnlinkWorkspace")
	}
	tk.LocalState = StateReady
	tk.ClearWorkDir(t0)
	if tk.LocalState != StateNone {
		t.Errorf("ClearWorkDir on ready: %s", tk.LocalState)
	}
	tk.LocalState = StateDone
	tk.ClearWorkDir(t0)
	if tk.LocalState != StateDone {
		t.Errorf("ClearWorkDir on done: %s", tk.LocalState)
	}
}

func TestTask_ApplyEnvAndNote(t *testing.T) {
	tk, _ := NewTask("t1", "PROJ-1", JiraSnapshot{}, t0)
	later := t0.Add(time.Hour)
	tk.SetNote("memo", later)
	if tk.Note != "memo" || !tk.UpdatedAt.Equal(later) {
		t.Errorf("SetNote: %+v", tk)
	}
	bad := Env{WorkDir: "~/w", Repos: []Repo{{Name: "a/b", Source: RepoSource{Kind: SourceClone, URL: "u"}}}}
	if err := tk.ApplyEnv(bad, t0); err == nil {
		t.Error("ApplyEnv with invalid repo name should fail")
	}
	good := Env{WorkDir: "~/w", Layout: "single", Repos: []Repo{{Name: "a", Source: RepoSource{Kind: SourceClone, URL: "u"}}}}
	if err := tk.ApplyEnv(good, t0); err != nil {
		t.Fatal(err)
	}
	if tk.Env.WorkDir != "~/w" {
		t.Error("ApplyEnv not applied")
	}
	tk.UpdateJira(JiraSnapshot{Summary: "new"}, later)
	if tk.Jira.Summary != "new" {
		t.Error("UpdateJira")
	}
}

func TestEnv_Validate(t *testing.T) {
	if err := (Env{}).Validate(); err != nil {
		t.Errorf("empty env should be valid: %v", err)
	}
	if (Env{}).IsEmpty() != true {
		t.Error("IsEmpty")
	}
	cases := map[string]Env{
		"missing workdir": {Repos: []Repo{{Name: "a", Source: RepoSource{Kind: SourceClone, URL: "u"}}}},
		"dup repo":        {WorkDir: "w", Repos: []Repo{{Name: "a", Source: RepoSource{Kind: SourceClone, URL: "u"}}, {Name: "a", Source: RepoSource{Kind: SourceClone, URL: "u"}}}},
		"dup slot":        {WorkDir: "w", Layout: "single", Panes: []PaneSpec{{Slot: 0}, {Slot: 0}}},
		"unknown repo":    {WorkDir: "w", Layout: "single", Panes: []PaneSpec{{Slot: 0, RepoName: "x"}}},
		"neg slot":        {WorkDir: "w", Layout: "single", Panes: []PaneSpec{{Slot: -1}}},
		"empty command":   {WorkDir: "w", Layout: "single", Panes: []PaneSpec{{Slot: 0, Commands: []Command{{Command: " "}}}}},
		"panes no layout": {WorkDir: "w", Panes: []PaneSpec{{Slot: 0}}},
		"clone no url":    {WorkDir: "w", Repos: []Repo{{Name: "a", Source: RepoSource{Kind: SourceClone}}}},
		"basecopy no id":  {WorkDir: "w", Repos: []Repo{{Name: "a", Source: RepoSource{Kind: SourceBaseCopy}}}},
		"bad kind":        {WorkDir: "w", Repos: []Repo{{Name: "a", Source: RepoSource{Kind: "x"}}}},
	}
	for name, env := range cases {
		if err := env.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	ok := Env{WorkDir: "w", Layout: "grid_2x2",
		Repos: []Repo{{Name: "fe", Source: RepoSource{Kind: SourceBaseCopy, BaseCloneID: "b1"}}, {Name: "be", Source: RepoSource{Kind: SourceClone, URL: "u"}}},
		Panes: []PaneSpec{{Slot: 0, RepoName: "fe", Commands: []Command{{Command: "npm run dev", AutoRun: true}}}, {Slot: 1, RepoName: "be"}, {Slot: 3}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid env rejected: %v", err)
	}
}

func TestLocalState_IsValid(t *testing.T) {
	for _, s := range []LocalState{StateNone, StatePreparing, StateReady, StateDone} {
		if !s.IsValid() {
			t.Errorf("%s should be valid", s)
		}
	}
	if LocalState("working").IsValid() {
		t.Error("working is derived, not a stored state")
	}
}
