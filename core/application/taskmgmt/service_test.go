package taskmgmt

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// testEnv はテスト用 Service と全ての偽物をまとめたもの。
type testEnv struct {
	svc        *Service
	tasks      *apptest.MemRepo[*task.Task]
	templates  *apptest.MemRepo[*task.Template]
	baseClones *apptest.MemRepo[*task.BaseClone]
	runs       *apptest.MemRepo[*task.SetupRun]
	settings   *apptest.FakeSettingsStore
	jiraStore  *apptest.FakeJiraStore
	jira       *apptest.FakeJiraClient
	git        *apptest.FakeGitService
	exec       *apptest.FakeExecutor
	ws         *apptest.FakeRepo
	idgen      *apptest.FakeIDGen
	live       []string
	deletedWS  []string
	now        time.Time
}

var testNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		tasks:      apptest.NewMemRepo[*task.Task](),
		templates:  apptest.NewMemRepo[*task.Template](),
		baseClones: apptest.NewMemRepo[*task.BaseClone](),
		runs:       apptest.NewMemRepo[*task.SetupRun](),
		settings:   apptest.NewFakeSettingsStore(),
		jiraStore:  apptest.NewFakeJiraStore(),
		jira:       apptest.NewFakeJiraClient(),
		git:        apptest.NewFakeGitService(),
		exec:       apptest.NewFakeExecutor(),
		ws:         apptest.NewFakeRepo(),
		idgen:      apptest.NewFakeIDGen(),
		now:        testNow,
	}
	// NewFakeGitService は DefaultBranches を初期化しないので、テスト側で用意する
	e.git.DefaultBranches = map[string]string{}
	svc, err := New(Deps{
		Tasks: e.tasks, Templates: e.templates, BaseClones: e.baseClones, Runs: e.runs,
		Settings: e.settings, JiraStore: e.jiraStore, JiraFactory: e.jira.Factory,
		Git: e.git, Exec: e.exec, Workspaces: e.ws, IDGen: e.idgen,
		LivePaneIDs: func() []string { return e.live },
		DeleteWorkspace: func(_ context.Context, id string) error {
			e.deletedWS = append(e.deletedWS, id)
			wid, _ := domain.NewWorkspaceId(id)
			return e.ws.Delete(context.Background(), wid)
		},
		BaseDir: "/data",
		Now:     func() time.Time { return e.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func issue(key, summary string) port.JiraIssue {
	return port.JiraIssue{Key: key, Summary: summary, Status: "To Do", StatusCategory: "new", Assignee: "me", URL: "https://example.atlassian.net/browse/" + key}
}

func (e *testEnv) addIssue(key, summary string) {
	e.jira.Issues[key] = issue(key, summary)
}

// addTemplate はベースクローン b1 とそれを使うテンプレートを登録する。
func (e *testEnv) addTemplate(t *testing.T) *task.Template {
	t.Helper()
	ctx := context.Background()
	bc := &task.BaseClone{ID: "b1", Name: "frontend", URL: "git@x:fe.git", Path: "/base/frontend", DefaultBranch: "main", CreatedAt: e.now}
	if err := e.baseClones.Save(ctx, bc); err != nil {
		t.Fatal(err)
	}
	tpl := &task.Template{
		ID: "tpl1", Name: "web", IsDefault: true, WorkDirPattern: "~/work/{KEY}", BranchPattern: "feature/{KEY}-{slug}", Layout: "split_vertical",
		Repos: []task.Repo{
			{Name: "frontend", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}, SetupCommands: []string{"npm ci"}},
			{Name: "backend", Source: task.RepoSource{Kind: task.SourceClone, URL: "git@x:be.git"}, SetupCommands: []string{"go mod download"}},
		},
		Panes: []task.PaneSpec{
			{Slot: 0, RepoName: "frontend", Commands: []task.Command{{Command: "npm run dev", AutoRun: true}}},
			{Slot: 1, RepoName: "backend", Commands: []task.Command{{Command: "claude", AutoRun: false}}},
		},
		CreatedAt: e.now, UpdatedAt: e.now,
	}
	if err := e.templates.Save(ctx, tpl); err != nil {
		t.Fatal(err)
	}
	return tpl
}

func (e *testEnv) importOne(t *testing.T, key string, templateID string) *task.Task {
	t.Helper()
	e.addIssue(key, "Summary "+key)
	res, err := e.svc.Import(context.Background(), []string{key}, templateID, false)
	if err != nil {
		t.Fatalf("import %s: %v", key, err)
	}
	return res.Tasks[0].Task
}

func isValidation(err error) bool {
	var ve *apperr.ValidationError
	return errors.As(err, &ve)
}

// waitResumed は再開直後の run が prev(failed/aborted)から動き出すまで待ってから、
// 終端状態まで待つ。resume は Start を goroutine 側で呼ぶため、直後の GetSetupRun は
// 前回の終端状態を返しうる。
func (e *testEnv) waitResumed(t *testing.T, runID string, prev task.RunState) *task.SetupRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r, err := e.svc.GetSetupRun(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if r.State != prev {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return e.waitRun(t, runID)
}

// waitRun は run が終端状態になるまで待つ。
func (e *testEnv) waitRun(t *testing.T, runID string) *task.SetupRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r, err := e.svc.GetSetupRun(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		switch r.State {
		case task.RunDone, task.RunFailed, task.RunAborted:
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", runID)
	return nil
}

func TestNew_RequiresDeps(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Error("missing deps should fail")
	}
}

func TestParseJiraKeys(t *testing.T) {
	got := ParseJiraKeys("PROJ-1301, proj-1302 https://x.atlassian.net/browse/PROJ-1303?x=1 PROJ-1301 nope-x 12-34")
	want := []string{"PROJ-1301", "PROJ-1302", "PROJ-1303"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := ParseJiraKeys(""); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

func TestPreview(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	e.addIssue("PROJ-1", "Jira settings")
	e.addIssue("PROJ-2", "Other")
	e.importOne(t, "PROJ-2", "")

	items, err := e.svc.Preview(ctx, []string{"proj-1", "PROJ-2"}, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items: %+v", items)
	}
	if items[0].Env == nil || items[0].Env.Branch != "feature/PROJ-1-jira-settings" || items[0].Env.WorkDir != "~/work/PROJ-1" {
		t.Errorf("env: %+v", items[0].Env)
	}
	if items[0].Exists || !items[1].Exists || len(items[1].Warnings) == 0 {
		t.Errorf("exists flags: %+v", items)
	}
	// テンプレートなし
	items, err = e.svc.Preview(ctx, []string{"PROJ-1"}, "")
	if err != nil || items[0].Env != nil {
		t.Errorf("no template: %v %+v", err, items)
	}
	if _, err := e.svc.Preview(ctx, []string{"PROJ-999"}, ""); err == nil || !errors.Is(err, port.ErrJiraNotFound) || !isValidation(err) {
		t.Errorf("missing issue: %v", err)
	}
	if _, err := e.svc.Preview(ctx, []string{"bad key"}, ""); !isValidation(err) {
		t.Errorf("bad key: %v", err)
	}
	if _, err := e.svc.Preview(ctx, nil, ""); !isValidation(err) {
		t.Errorf("no keys: %v", err)
	}
	if _, err := e.svc.Preview(ctx, []string{"PROJ-1"}, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("unknown template: %v", err)
	}
}

func TestPreview_JiraNotConfigured(t *testing.T) {
	e := newTestService(t)
	e.jiraStore.Tok = ""
	_, err := e.svc.Preview(context.Background(), []string{"PROJ-1"}, "")
	if !errors.Is(err, port.ErrJiraNotConfigured) || !isValidation(err) {
		t.Errorf("got %v", err)
	}
}

func TestImport(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	e.addIssue("PROJ-1", "Jira settings")
	e.addIssue("PROJ-2", "Logging")

	res, err := e.svc.Import(ctx, []string{"PROJ-1", "PROJ-2"}, tpl.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tasks) != 2 || len(res.RunIDs) != 0 {
		t.Fatalf("result: %+v", res)
	}
	tk := res.Tasks[0]
	if tk.JiraKey != "PROJ-1" || tk.LocalState != task.StateNone || tk.EffectiveState != "none" {
		t.Errorf("task: %+v", tk)
	}
	if tk.Env.TemplateID != tpl.ID || tk.Env.Branch != "feature/PROJ-1-jira-settings" || len(tk.Env.Repos) != 2 {
		t.Errorf("env: %+v", tk.Env)
	}
	if tk.Jira.Summary != "Jira settings" || !tk.Jira.FetchedAt.Equal(testNow) {
		t.Errorf("snapshot: %+v", tk.Jira)
	}
	all, _ := e.tasks.List(ctx)
	if len(all) != 2 {
		t.Errorf("saved %d", len(all))
	}

	// 重複は全体エラーで何も保存しない
	e.addIssue("PROJ-3", "New")
	_, err = e.svc.Import(ctx, []string{"PROJ-3", "PROJ-1"}, "", false)
	if !errors.Is(err, task.ErrDuplicateKey) || !isValidation(err) {
		t.Fatalf("duplicate: %v", err)
	}
	all, _ = e.tasks.List(ctx)
	if len(all) != 2 {
		t.Errorf("duplicate import saved tasks: %d", len(all))
	}
	// setup にはテンプレートが必要
	if _, err := e.svc.Import(ctx, []string{"PROJ-3"}, "", true); !isValidation(err) {
		t.Errorf("setup without template: %v", err)
	}
	// テンプレートなしの取り込みは env 空
	res, err = e.svc.Import(ctx, []string{"PROJ-3"}, "", false)
	if err != nil || !res.Tasks[0].Env.IsEmpty() {
		t.Errorf("no template import: %v %+v", err, res)
	}
}

func TestImport_WithSetup(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	e.addIssue("PROJ-1", "Jira settings")
	res, err := e.svc.Import(ctx, []string{"PROJ-1"}, tpl.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	runID, ok := res.RunIDs["PROJ-1"]
	if !ok || runID == "" {
		t.Fatalf("run id missing: %+v", res)
	}
	r := e.waitRun(t, runID)
	if r.State != task.RunDone {
		t.Fatalf("run: %+v", r)
	}
	tk, err := e.tasks.FindByID(ctx, res.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if tk.LocalState != task.StateReady || tk.WorkspaceID == "" || tk.SetupRunID != runID {
		t.Errorf("task after setup: %+v", tk)
	}
}

func TestList_OrderAndWorking(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tDone := e.importOne(t, "PROJ-1", "")
	tNone := e.importOne(t, "PROJ-2", "")
	tReady := e.importOne(t, "PROJ-3", "")
	tWorking := e.importOne(t, "PROJ-4", "")
	tPrep := e.importOne(t, "PROJ-5", "")

	_ = tDone.MarkDone(e.now)
	_ = e.tasks.Save(ctx, tDone)
	tReady.LocalState = task.StateReady
	_ = e.tasks.Save(ctx, tReady)
	tPrep.LocalState = task.StatePreparing
	_ = e.tasks.Save(ctx, tPrep)

	// 作業中: ライブセッションを持つ pane のあるワークスペースに紐付ける
	ws := newWorkspace(t, "ws1", "PROJ-4 work", "single", []paneSpec{{id: "p1", dir: "/w"}})
	_ = e.ws.Save(ctx, ws)
	tWorking.LocalState = task.StateReady
	tWorking.LinkWorkspace("ws1", e.now)
	_ = e.tasks.Save(ctx, tWorking)
	e.live = []string{"p1"}

	// 実体のないワークスペースに紐付いたもの
	tNone.LinkWorkspace("missing", e.now)
	_ = e.tasks.Save(ctx, tNone)

	list, err := e.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var keys, states []string
	for _, d := range list {
		keys = append(keys, d.JiraKey)
		states = append(states, d.EffectiveState)
	}
	wantKeys := []string{"PROJ-4", "PROJ-5", "PROJ-3", "PROJ-2", "PROJ-1"}
	wantStates := []string{"working", "preparing", "ready", "none", "done"}
	if !reflect.DeepEqual(keys, wantKeys) || !reflect.DeepEqual(states, wantStates) {
		t.Errorf("keys=%v states=%v", keys, states)
	}
	if !list[0].Working || list[0].WorkspaceName != "PROJ-4 work" {
		t.Errorf("working dto: %+v", list[0])
	}
	if !list[3].WorkspaceMissing {
		t.Errorf("missing ws flag: %+v", list[3])
	}
	// ライブセッションが無くなれば ready に戻る
	e.live = nil
	list, _ = e.svc.List(ctx)
	if list[0].JiraKey != "PROJ-5" {
		t.Errorf("without live: first=%s", list[0].JiraKey)
	}
}

func TestList_SameStateOrdersByUpdatedAtDesc(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	old := e.importOne(t, "PROJ-1", "")
	e.now = testNow.Add(time.Hour)
	_ = e.importOne(t, "PROJ-2", "")
	_ = e.tasks.Save(ctx, old)
	list, _ := e.svc.List(ctx)
	if list[0].JiraKey != "PROJ-2" {
		t.Errorf("newest first: %s", list[0].JiraKey)
	}
}

func TestGetAndFindByKey(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.importOne(t, "PROJ-1", "")
	tk.Env = task.Env{WorkDir: "~/work/PROJ-1", Layout: "single"}
	_ = e.tasks.Save(ctx, tk)
	e.exec.ExistsPaths["/home/test/work/PROJ-1"] = true

	got, err := e.svc.Get(ctx, tk.ID)
	if err != nil || got.JiraKey != "PROJ-1" || !got.WorkDirExists {
		t.Errorf("Get: %v %+v", err, got)
	}
	byKey, err := e.svc.FindByKey(ctx, "proj-1")
	if err != nil || byKey.ID != tk.ID {
		t.Errorf("FindByKey: %v %+v", err, byKey)
	}
	if _, err := e.svc.FindByKey(ctx, "PROJ-9"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("FindByKey missing: %v", err)
	}
	if _, err := e.svc.Get(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("Get missing: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	e.addTemplate(t)
	tk := e.importOne(t, "PROJ-1", "")
	note := "memo"
	got, err := e.svc.Update(ctx, tk.ID, UpdateInput{Note: &note})
	if err != nil || got.Note != "memo" {
		t.Fatalf("note: %v %+v", err, got)
	}

	env := task.Env{WorkDir: "~/work/PROJ-1", Branch: "feature/PROJ-1", Layout: "single",
		Repos: []task.Repo{{Name: "frontend", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}}},
		Panes: []task.PaneSpec{{Slot: 0, RepoName: "frontend"}}}
	got, err = e.svc.Update(ctx, tk.ID, UpdateInput{Env: &env})
	if err != nil || got.Env.Branch != "feature/PROJ-1" {
		t.Fatalf("env: %v %+v", err, got)
	}
	badLayout := env
	badLayout.Layout = "weird"
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{Env: &badLayout}); !isValidation(err) {
		t.Errorf("bad layout: %v", err)
	}
	tooMany := env
	tooMany.Panes = []task.PaneSpec{{Slot: 0}, {Slot: 1}}
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{Env: &tooMany}); !isValidation(err) {
		t.Errorf("too many panes: %v", err)
	}
	badBase := env
	badBase.Repos = []task.Repo{{Name: "x", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "nope"}}}
	badBase.Panes = nil
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{Env: &badBase}); !isValidation(err) {
		t.Errorf("unknown base clone: %v", err)
	}

	// 完了 → 取り消し(フォルダあり → ready)
	done := "done"
	got, err = e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &done})
	if err != nil || got.LocalState != task.StateDone {
		t.Fatalf("done: %v %+v", err, got)
	}
	e.exec.ExistsPaths["/home/test/work/PROJ-1"] = true
	ready := "ready"
	got, err = e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &ready})
	if err != nil || got.LocalState != task.StateReady {
		t.Fatalf("reopen: %v %+v", err, got)
	}
	// フォルダなし → none
	_, _ = e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &done})
	delete(e.exec.ExistsPaths, "/home/test/work/PROJ-1")
	got, _ = e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &ready})
	if got.LocalState != task.StateNone {
		t.Errorf("reopen without dir: %s", got.LocalState)
	}
	bogus := "preparing"
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &bogus}); !isValidation(err) {
		t.Errorf("bogus state: %v", err)
	}
	// 準備中は env 編集不可
	tk, _ = e.tasks.FindByID(ctx, tk.ID)
	tk.LocalState = task.StatePreparing
	_ = e.tasks.Save(ctx, tk)
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{Env: &env}); !isValidation(err) {
		t.Errorf("env while preparing: %v", err)
	}
	if _, err := e.svc.Update(ctx, tk.ID, UpdateInput{LocalState: &done}); !isValidation(err) {
		t.Errorf("done while preparing: %v", err)
	}
	if _, err := e.svc.Update(ctx, "nope", UpdateInput{Note: &note}); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func TestDelete(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.importOne(t, "PROJ-1", "")
	ws := newWorkspace(t, "ws1", "w", "single", nil)
	ws.LinkTask(tk.ID)
	_ = e.ws.Save(ctx, ws)
	run := &task.SetupRun{ID: "r1", TaskID: tk.ID, State: task.RunDone, Steps: []task.Step{{}}}
	_ = e.runs.Save(ctx, run)
	tk.LinkWorkspace("ws1", e.now)
	tk.SetupRunID = "r1"
	_ = e.tasks.Save(ctx, tk)

	if err := e.svc.Delete(ctx, tk.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.tasks.FindByID(ctx, tk.ID); !errors.Is(err, task.ErrNotFound) {
		t.Error("task not deleted")
	}
	w, err := e.ws.FindByID(ctx, mustWsID("ws1"))
	if err != nil || w.TaskID() != "" {
		t.Errorf("workspace should remain and be unlinked: %v %+v", err, w)
	}
	if _, err := e.runs.FindByID(ctx, "r1"); !errors.Is(err, task.ErrNotFound) {
		t.Error("run should be deleted")
	}
	if _, err := e.ws.FindByID(ctx, mustWsID("ws1")); err != nil {
		t.Error("workspace must not be deleted")
	}
	tk2 := e.importOne(t, "PROJ-2", "")
	tk2.LocalState = task.StatePreparing
	_ = e.tasks.Save(ctx, tk2)
	if err := e.svc.Delete(ctx, tk2.ID); !isValidation(err) {
		t.Errorf("delete while preparing: %v", err)
	}
	if err := e.svc.Delete(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func TestSync(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.importOne(t, "PROJ-1", "")
	is := issue("PROJ-1", "Renamed")
	is.Status = "In Progress"
	e.jira.Issues["PROJ-1"] = is
	e.now = testNow.Add(time.Hour)
	got, err := e.svc.Sync(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Jira.Summary != "Renamed" || got.Jira.Status != "In Progress" || !got.Jira.FetchedAt.Equal(e.now) {
		t.Errorf("sync: %+v", got.Jira)
	}
	delete(e.jira.Issues, "PROJ-1")
	if _, err := e.svc.Sync(ctx, tk.ID); !errors.Is(err, port.ErrJiraNotFound) {
		t.Errorf("gone: %v", err)
	}
}

func TestSyncAll(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	t1 := e.importOne(t, "PROJ-1", "")
	e.importOne(t, "PROJ-2", "")
	e.jira.Issues["PROJ-1"] = issue("PROJ-1", "Updated 1")
	e.jira.SearchResults = []port.JiraIssue{issue("PROJ-2", "dup"), issue("PROJ-7", "From JQL")}
	e.jiraStore.Cfg.JQL = "assignee = currentUser()"

	res, err := e.svc.SyncAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 2 || !reflect.DeepEqual(res.Added, []string{"PROJ-7"}) || len(res.Errors) != 0 {
		t.Errorf("result: %+v", res)
	}
	got, _ := e.tasks.FindByID(ctx, t1.ID)
	if got.Jira.Summary != "Updated 1" {
		t.Errorf("not updated: %+v", got.Jira)
	}
	all, _ := e.tasks.List(ctx)
	if len(all) != 3 {
		t.Errorf("tasks after sync: %d", len(all))
	}
	if len(e.jira.SearchCalls) != 1 || e.jira.SearchCalls[0] != "assignee = currentUser()" {
		t.Errorf("search calls: %v", e.jira.SearchCalls)
	}
	// 一括取得で 1 件消えていれば 1 件ずつに落として他は更新する
	e.jira.Issues["PROJ-7"] = issue("PROJ-7", "From JQL")
	delete(e.jira.Issues, "PROJ-2")
	e.jira.SearchResults = nil
	res, err = e.svc.SyncAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 2 || len(res.Errors) != 1 {
		t.Errorf("partial: %+v", res)
	}
	// JQL 無し・タスク無しでも動く
	e2 := newTestService(t)
	if res, err := e2.svc.SyncAll(ctx); err != nil || res.Updated != 0 || len(res.Added) != 0 {
		t.Errorf("empty: %v %+v", err, res)
	}
}

func TestDeleteWorkDir(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.importOne(t, "PROJ-1", "")
	ws := newWorkspace(t, "ws1", "w", "single", nil)
	_ = e.ws.Save(ctx, ws)
	tk.Env = task.Env{WorkDir: "~/work/PROJ-1", Layout: "single"}
	tk.LocalState = task.StateReady
	tk.LinkWorkspace("ws1", e.now)
	_ = e.tasks.Save(ctx, tk)

	got, err := e.svc.DeleteWorkDir(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.exec.Removed, []string{"/home/test/work/PROJ-1"}) {
		t.Errorf("removed: %v", e.exec.Removed)
	}
	if !reflect.DeepEqual(e.deletedWS, []string{"ws1"}) {
		t.Errorf("deleted ws: %v", e.deletedWS)
	}
	if got.LocalState != task.StateNone || got.WorkspaceID != "" {
		t.Errorf("after delete: %+v", got.Task)
	}
	// 完了タスクは完了のまま
	tk2 := e.importOne(t, "PROJ-2", "")
	tk2.Env = task.Env{WorkDir: "~/work/PROJ-2", Layout: "single"}
	_ = tk2.MarkDone(e.now)
	_ = e.tasks.Save(ctx, tk2)
	got, err = e.svc.DeleteWorkDir(ctx, tk2.ID)
	if err != nil || got.LocalState != task.StateDone {
		t.Errorf("done stays done: %v %+v", err, got.Task)
	}
	tk3 := e.importOne(t, "PROJ-3", "")
	tk3.LocalState = task.StatePreparing
	_ = e.tasks.Save(ctx, tk3)
	if _, err := e.svc.DeleteWorkDir(ctx, tk3.ID); !isValidation(err) {
		t.Errorf("preparing: %v", err)
	}
}

func TestUnlinkWorkspace(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	a := e.importOne(t, "PROJ-1", "")
	b := e.importOne(t, "PROJ-2", "")
	a.LinkWorkspace("ws1", e.now)
	b.LinkWorkspace("ws2", e.now)
	_ = e.tasks.Save(ctx, a)
	_ = e.tasks.Save(ctx, b)
	if err := e.svc.UnlinkWorkspace(ctx, "ws1"); err != nil {
		t.Fatal(err)
	}
	a, _ = e.tasks.FindByID(ctx, a.ID)
	b, _ = e.tasks.FindByID(ctx, b.ID)
	if a.WorkspaceID != "" || b.WorkspaceID != "ws2" {
		t.Errorf("a=%q b=%q", a.WorkspaceID, b.WorkspaceID)
	}
}

func TestSettingsAndJiraConfig(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	st, _ := e.svc.GetSettings(ctx)
	if !st.AutoFetchBeforeSetup || !st.IncludeNodeModules {
		t.Errorf("defaults: %+v", st)
	}
	st.IncludeNodeModules = false
	if got, err := e.svc.PutSettings(ctx, st); err != nil || got.IncludeNodeModules {
		t.Errorf("put: %v %+v", err, got)
	}

	e.jiraStore.Cfg.FieldIDs = map[string]string{"Sprint": "customfield_1"}
	cfg, _ := e.svc.GetJiraConfig(ctx)
	if !cfg.HasToken || cfg.FieldIDs != nil {
		t.Errorf("get: %+v", cfg)
	}
	// 同じサイトならフィールド ID キャッシュを引き継ぎ、トークン空なら維持
	got, err := e.svc.PutJiraConfig(ctx, JiraConfigInput{Kind: "cloud", BaseURL: "https://example.atlassian.net/", Email: "me@example.com", JQL: " x "})
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != "https://example.atlassian.net" || got.JQL != "x" || !got.HasToken {
		t.Errorf("put: %+v", got)
	}
	if e.jiraStore.Cfg.FieldIDs["Sprint"] != "customfield_1" {
		t.Error("field ids should carry over for the same site")
	}
	// サイト変更でキャッシュ破棄、トークン更新、ファクトリ再生成
	_, _ = e.svc.TestJira(ctx), e.jira.FactoryCalls
	before := e.jira.FactoryCalls
	got, err = e.svc.PutJiraConfig(ctx, JiraConfigInput{Kind: "server", BaseURL: "https://jira.internal", Token: "pat"})
	if err != nil || e.jiraStore.Cfg.FieldIDs != nil || e.jiraStore.Tok != "pat" {
		t.Errorf("site change: %v cfg=%+v tok=%q", err, e.jiraStore.Cfg, e.jiraStore.Tok)
	}
	e.jira.User = port.JiraUser{DisplayName: "Me", Email: "me@x"}
	res := e.svc.TestJira(ctx)
	if !res.OK || res.DisplayName != "Me" || e.jira.FactoryCalls != before+1 {
		t.Errorf("test: %+v factory=%d", res, e.jira.FactoryCalls)
	}
	// トークン削除
	got, _ = e.svc.PutJiraConfig(ctx, JiraConfigInput{Kind: "server", BaseURL: "https://jira.internal", ClearToken: true})
	if got.HasToken {
		t.Error("token should be cleared")
	}
	if res := e.svc.TestJira(ctx); res.OK || res.Error == "" {
		t.Errorf("test without token: %+v", res)
	}
	if _, err := e.svc.PutJiraConfig(ctx, JiraConfigInput{Kind: "cloud", BaseURL: "https://a"}); !isValidation(err) {
		t.Errorf("cloud without email: %v", err)
	}
	e.jira.Err = port.ErrJiraAuth
	e.jiraStore.Tok = "t"
	e.svc.invalidateJira()
	if res := e.svc.TestJira(ctx); res.OK {
		t.Error("auth error should fail test")
	}
}

// ---- ヘルパ

type paneSpec struct {
	id, dir string
}

func newWorkspace(t *testing.T, id, name, layout string, panes []paneSpec) *domain.Workspace {
	t.Helper()
	wid, _ := domain.NewWorkspaceId(id)
	wn, _ := domain.NewWorkspaceName(name)
	w, err := domain.NewWorkspace(wid, wn, domain.LayoutPreset(layout))
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range panes {
		pid, _ := domain.NewPaneId(p.id)
		dp, _ := domain.NewDirectoryPath(p.dir)
		slot, _ := domain.NewSlotIndex(i)
		title, _ := domain.NewPaneTitle("")
		host, _ := domain.NewRemoteHost("")
		pane, _ := domain.NewPane(pid, dp, slot, title, host, nil)
		if err := w.AddPane(pane); err != nil {
			t.Fatal(err)
		}
	}
	return w
}
