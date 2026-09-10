package taskmgmt

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

func localIn(summary string) LocalCreateInput {
	return LocalCreateInput{LocalInput: task.LocalInput{Summary: summary, Priority: "Medium"}}
}

func TestNextLocalKey_Default(t *testing.T) {
	e := newTestService(t)
	key, err := e.svc.NextLocalKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if key != "T-001" {
		t.Errorf("NextLocalKey = %q", key)
	}
}

func TestCreateLocal_AutoKeyAllocation(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	r1, err := e.svc.CreateLocal(ctx, localIn("first"))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := e.svc.CreateLocal(ctx, localIn("second"))
	if err != nil {
		t.Fatal(err)
	}
	if r1.Task.JiraKey != "T-001" || r2.Task.JiraKey != "T-002" {
		t.Errorf("採番: %s %s", r1.Task.JiraKey, r2.Task.JiraKey)
	}
	if r1.Task.Source != task.SourceLocal || r1.Task.Local == nil || r1.Task.Local.StatusID != "todo" {
		t.Errorf("既定ステータスが先頭になる: %+v", r1.Task.Local)
	}
	if r1.Task.Jira.Status != "To Do" || r1.Task.Jira.StatusCategory != "new" {
		t.Errorf("DTO でステータス名が解決される: %+v", r1.Task.Jira)
	}
	if r1.RunID != "" {
		t.Error("setup=false なら run は無い")
	}
	key, _ := e.svc.NextLocalKey(ctx)
	if key != "T-003" {
		t.Errorf("採番が進む: %q", key)
	}
}

func TestCreateLocal_SkipsExistingKey(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	// 先に手入力で T-001 を使っておく(採番は 2 に進む)。T-002 も別途占有しておき、次の自動採番が T-003 になることを見る
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "manual"}, Key: "t-001"}); err != nil {
		t.Fatal(err)
	}
	occupied, _ := task.NewLocalTask("x", "T-002", task.LocalInput{Summary: "occupied"}, testNow)
	if err := e.tasks.Save(ctx, occupied); err != nil {
		t.Fatal(err)
	}
	r, err := e.svc.CreateLocal(ctx, localIn("auto"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Task.JiraKey != "T-003" {
		t.Errorf("既存の番号を飛ばす: %q", r.Task.JiraKey)
	}
}

func TestCreateLocal_ExplicitKeyBumpsSeq(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	r, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "x"}, Key: "T-010"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Task.JiraKey != "T-010" {
		t.Errorf("指定した番号: %q", r.Task.JiraKey)
	}
	st, _ := e.svc.GetSettings(ctx)
	if st.LocalTask.NextSeq != 11 {
		t.Errorf("NextSeq = %d want 11", st.LocalTask.NextSeq)
	}
	// 別接頭辞は採番に影響しない
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "y"}, Key: "OTHER-99"}); err != nil {
		t.Fatal(err)
	}
	st, _ = e.svc.GetSettings(ctx)
	if st.LocalTask.NextSeq != 11 {
		t.Errorf("別接頭辞で NextSeq が動いた: %d", st.LocalTask.NextSeq)
	}
	// 重複・不正
	_, err = e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "z"}, Key: "T-010"})
	if !isValidation(err) || !errors.Is(err, task.ErrDuplicateKey) {
		t.Errorf("重複は ErrDuplicateKey の validation: %v", err)
	}
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "z"}, Key: "10"}); !isValidation(err) {
		t.Errorf("不正な番号は validation: %v", err)
	}
	if _, err := e.svc.CreateLocal(ctx, localIn("  ")); !isValidation(err) {
		t.Errorf("概要が空は validation: %v", err)
	}
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "s", StatusID: "nope"}}); !isValidation(err) {
		t.Errorf("未知のステータスは validation: %v", err)
	}
}

func TestCreateLocal_WithTemplateExpandsEnv(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	in := localIn("Playwright で E2E テスト")
	in.TemplateID = tpl.ID
	r, err := e.svc.CreateLocal(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	env := r.Task.Env
	if env.TemplateID != tpl.ID || !strings.Contains(env.WorkDir, "T-001") || env.Branch != "feature/T-001-playwright-e2e" {
		t.Errorf("テンプレート展開: %+v", env)
	}
	if len(env.Repos) != 2 || len(env.Panes) != 2 {
		t.Errorf("リポジトリ・ペインがコピーされる: %+v", env)
	}
	// setup=true はテンプレート必須
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "s"}, Setup: true}); !isValidation(err) {
		t.Errorf("テンプレート無しの setup は validation: %v", err)
	}
}

func TestCreateLocal_WithSetupRunsToDone(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	in := localIn("setup me")
	in.TemplateID = tpl.ID
	in.Setup = true
	r, err := e.svc.CreateLocal(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.RunID == "" {
		t.Fatal("run が始まらない")
	}
	var run *task.SetupRun
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err = e.svc.GetSetupRun(ctx, r.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if run.State == task.RunDone || run.State == task.RunFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if run.State != task.RunDone {
		t.Fatalf("run state = %s (%s)", run.State, run.Error)
	}
	got, err := e.svc.Get(ctx, r.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocalState != task.StateReady || got.WorkspaceID == "" || !got.IsLocal() {
		t.Errorf("完了後: state=%s ws=%q local=%v", got.LocalState, got.WorkspaceID, got.IsLocal())
	}
	if got.WorkspaceName != "T-001 setup me" {
		t.Errorf("ワークスペース名は番号 + 概要: %q", got.WorkspaceName)
	}
}

func TestLocalStatusResolution(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	in := localIn("s")
	in.StatusID = "inprogress"
	r, err := e.svc.CreateLocal(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Jira.Status != "In Progress" || list[0].Jira.StatusCategory != "indeterminate" {
		t.Errorf("List で解決: %+v", list[0].Jira)
	}
	// ステータス定義から消えた ID は先頭にフォールバック
	stored, _ := e.tasks.FindByID(ctx, r.Task.ID)
	stored.Local.StatusID = "deleted"
	_ = e.tasks.Save(ctx, stored)
	got, _ := e.svc.Get(ctx, r.Task.ID)
	if got.Jira.Status != "To Do" || got.Jira.StatusCategory != "new" {
		t.Errorf("フォールバック: %+v", got.Jira)
	}
	// jira タスクには影響しない
	e.importOne(t, "PROJ-1", "")
	list, _ = e.svc.List(ctx)
	for _, d := range list {
		if d.JiraKey == "PROJ-1" && (d.Jira.Status != "To Do" || d.Source != task.SourceJira) {
			t.Errorf("jira タスクの状態が壊れた: %+v", d.Jira)
		}
	}
}

func TestUpdate_LocalFields(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	r, err := e.svc.CreateLocal(ctx, localIn("before"))
	if err != nil {
		t.Fatal(err)
	}
	upd, err := e.svc.Update(ctx, r.Task.ID, UpdateInput{Local: &task.LocalInput{Summary: "after", Priority: "High", Labels: []string{"x", "y"}, Links: []string{"https://l"}, StatusID: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Jira.Summary != "after" || upd.Jira.Priority != "High" || upd.Local.StatusID != "done" || upd.Jira.Status != "Done" || len(upd.Local.Labels) != 2 || len(upd.Local.Links) != 1 {
		t.Errorf("更新結果: %+v %+v", upd.Jira, upd.Local)
	}
	if _, err := e.svc.Update(ctx, r.Task.ID, UpdateInput{Local: &task.LocalInput{Summary: "s", StatusID: "nope"}}); !isValidation(err) {
		t.Errorf("未知のステータス: %v", err)
	}
	if _, err := e.svc.Update(ctx, r.Task.ID, UpdateInput{Local: &task.LocalInput{Summary: ""}}); !isValidation(err) {
		t.Errorf("空の概要: %v", err)
	}
	jira := e.importOne(t, "PROJ-1", "")
	if _, err := e.svc.Update(ctx, jira.ID, UpdateInput{Local: &task.LocalInput{Summary: "x"}}); !isValidation(err) {
		t.Errorf("jira タスクは編集不可: %v", err)
	}
}

func TestSync_RejectsLocalAndSyncAllSkipsLocal(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	r, err := e.svc.CreateLocal(ctx, localIn("local"))
	if err != nil {
		t.Fatal(err)
	}
	e.importOne(t, "PROJ-1", "")
	if _, err := e.svc.Sync(ctx, r.Task.ID); !isValidation(err) {
		t.Errorf("Sync(local) は validation: %v", err)
	}
	e.jira.FetchCalls = nil
	res, err := e.svc.SyncAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Errorf("jira 1 件だけ更新: %+v", res)
	}
	for _, call := range e.jira.FetchCalls {
		for _, k := range call {
			if k == r.Task.JiraKey {
				t.Errorf("ローカルの番号を Jira に問い合わせた: %v", e.jira.FetchCalls)
			}
		}
	}
}

func TestLinkJira(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tpl := e.addTemplate(t)
	in := localIn("local task")
	in.TemplateID = tpl.ID
	r, err := e.svc.CreateLocal(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	e.addIssue("PROJ-9", "from jira")
	got, err := e.svc.LinkJira(ctx, r.Task.ID, "proj-9")
	if err != nil {
		t.Fatal(err)
	}
	if got.JiraKey != "PROJ-9" || got.Source != task.SourceJira || got.Local != nil || got.Jira.Summary != "from jira" {
		t.Errorf("切替結果: key=%s source=%s local=%v jira=%+v", got.JiraKey, got.Source, got.Local, got.Jira)
	}
	if got.Env.Branch != r.Task.Env.Branch || got.Env.WorkDir != r.Task.Env.WorkDir {
		t.Errorf("環境は維持: %+v vs %+v", got.Env, r.Task.Env)
	}
	stored, _ := e.tasks.FindByID(ctx, r.Task.ID)
	if stored.JiraKey != "PROJ-9" {
		t.Error("保存されている")
	}
	// 既に jira → エラー
	if _, err := e.svc.LinkJira(ctx, r.Task.ID, "PROJ-10"); !isValidation(err) {
		t.Errorf("二重紐付け: %v", err)
	}
	// 既存の Jira 番号に紐付けようとするとエラー
	r2, _ := e.svc.CreateLocal(ctx, localIn("another"))
	_, err = e.svc.LinkJira(ctx, r2.Task.ID, "PROJ-9")
	if !isValidation(err) || !errors.Is(err, task.ErrDuplicateKey) {
		t.Errorf("重複番号: %v", err)
	}
	// Jira に無い番号
	if _, err := e.svc.LinkJira(ctx, r2.Task.ID, "PROJ-404"); !isValidation(err) {
		t.Errorf("無い番号: %v", err)
	}
}

func TestPutSettings_LocalTask(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	if _, err := e.svc.CreateLocal(ctx, LocalCreateInput{LocalInput: task.LocalInput{Summary: "s"}, Key: "T-005"}); err != nil {
		t.Fatal(err)
	}
	st, _ := e.svc.GetSettings(ctx)
	st.LocalTask.Prefix = "todo"
	st.LocalTask.NextSeq = 1 // クライアントが巻き戻しても無視される
	st.LocalTask.Statuses = append(st.LocalTask.Statuses, task.LocalStatus{Name: "Review"})
	saved, err := e.svc.PutSettings(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LocalTask.Prefix != "TODO" {
		t.Errorf("接頭辞は大文字化: %q", saved.LocalTask.Prefix)
	}
	if saved.LocalTask.NextSeq != 6 {
		t.Errorf("NextSeq は保存済みの値を保つ: %d", saved.LocalTask.NextSeq)
	}
	last := saved.LocalTask.Statuses[len(saved.LocalTask.Statuses)-1]
	if last.ID == "" || last.Category != task.CategoryNew {
		t.Errorf("新規ステータスに ID と既定カテゴリ: %+v", last)
	}
	st.LocalTask.Prefix = "bad-prefix"
	if _, err := e.svc.PutSettings(ctx, st); !isValidation(err) {
		t.Errorf("不正な接頭辞: %v", err)
	}
	st.LocalTask.Prefix = "T"
	st.LocalTask.Statuses = nil
	if _, err := e.svc.PutSettings(ctx, st); !isValidation(err) {
		t.Errorf("ステータス無し: %v", err)
	}
}
