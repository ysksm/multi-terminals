package taskmgmt

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// setupTask はテンプレート付きで取り込んだタスクを返す(未準備・環境あり)。
func (e *testEnv) setupTask(t *testing.T) *task.Task {
	t.Helper()
	tpl := e.addTemplate(t)
	return e.importOne(t, "PROJ-1", tpl.ID)
}

func stepStates(r *task.SetupRun) []task.StepState {
	out := make([]task.StepState, len(r.Steps))
	for i, s := range r.Steps {
		out[i] = s.State
	}
	return out
}

func TestRunner_HappyPath(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	e.exec.CommandOutput = "ok\n"
	e.git.DefaultBranches["/base/frontend"] = "main"
	e.git.DefaultBranches["/home/test/work/PROJ-1/frontend"] = "main"
	e.git.DefaultBranches["/home/test/work/PROJ-1/backend"] = "develop"

	runID, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 開始直後は preparing
	if got, _ := e.tasks.FindByID(ctx, tk.ID); got.LocalState != task.StatePreparing || got.SetupRunID != runID {
		t.Errorf("task after start: %+v", got)
	}
	r := e.waitRun(t, runID)
	if r.State != task.RunDone || r.Error != "" || r.FinishedAt.IsZero() {
		t.Fatalf("run: state=%s err=%q", r.State, r.Error)
	}
	for _, s := range r.Steps {
		if s.State != task.StepDone {
			t.Errorf("step %d %s: %s log=%q", s.Index, s.Label, s.State, s.Log)
		}
	}
	wd := "/home/test/work/PROJ-1"
	if !reflect.DeepEqual(e.exec.Mkdirs, []string{wd}) {
		t.Errorf("mkdirs: %v", e.exec.Mkdirs)
	}
	// ベース更新: fetch → checkout main → pull
	if !reflect.DeepEqual(e.git.GitOps[:2], []apptest.GitOpCall{{Dir: "/base/frontend", Op: "fetch"}, {Dir: "/base/frontend", Op: "pull"}}) {
		t.Errorf("base git ops: %v", e.git.GitOps)
	}
	if len(e.git.Checkouts) == 0 || e.git.Checkouts[0] != (apptest.CheckoutCall{Dir: "/base/frontend", Branch: "main"}) {
		t.Errorf("base checkout: %v", e.git.Checkouts)
	}
	if !reflect.DeepEqual(e.exec.Copies, []apptest.CopyCall{{Src: "/base/frontend", Dst: wd + "/frontend", Exclude: nil}}) {
		t.Errorf("copies: %+v", e.exec.Copies)
	}
	if !reflect.DeepEqual(e.git.Clones, []apptest.CloneCall{{URL: "git@x:be.git", Dest: wd + "/backend"}}) {
		t.Errorf("clones: %+v", e.git.Clones)
	}
	wantBranches := []apptest.CreateBranchCall{
		{Dir: wd + "/frontend", Branch: "feature/PROJ-1-summary-proj-1", StartPoint: "origin/main"},
		{Dir: wd + "/backend", Branch: "feature/PROJ-1-summary-proj-1", StartPoint: "origin/develop"},
	}
	if !reflect.DeepEqual(e.git.CreateBranches, wantBranches) {
		t.Errorf("branches: %+v", e.git.CreateBranches)
	}
	if !reflect.DeepEqual(e.exec.CommandList(), []string{"npm ci", "go mod download"}) {
		t.Errorf("commands: %v", e.exec.CommandList())
	}
	cmd := e.exec.Commands[0]
	if cmd.Dir != wd+"/frontend" || !contains(cmd.Env, "TASK_KEY=PROJ-1") || !contains(cmd.Env, "TASK_DIR="+wd) || !contains(cmd.Env, "TASK_BRANCH=feature/PROJ-1-summary-proj-1") {
		t.Errorf("command env/dir: %+v", cmd)
	}
	if !strings.Contains(r.Steps[4].Log, "$ npm ci\nok\n") {
		t.Errorf("setup log: %q", r.Steps[4].Log)
	}

	// ワークスペース
	got, _ := e.tasks.FindByID(ctx, tk.ID)
	if got.LocalState != task.StateReady || got.WorkspaceID == "" || got.WorkspaceID != r.WorkspaceID {
		t.Fatalf("task: %+v run ws=%s", got, r.WorkspaceID)
	}
	w, err := e.ws.FindByID(ctx, mustWsID(got.WorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	if w.Name().String() != "PROJ-1 Summary PROJ-1" || string(w.Layout()) != "split_vertical" || w.TaskID() != tk.ID {
		t.Errorf("workspace: name=%q layout=%s task=%q", w.Name(), w.Layout(), w.TaskID())
	}
	panes := w.Panes()
	if len(panes) != 2 {
		t.Fatalf("panes: %d", len(panes))
	}
	if panes[0].Directory().String() != wd+"/frontend" || panes[0].Title().String() != "frontend" || panes[0].Slot().Int() != 0 {
		t.Errorf("pane0: %+v", panes[0])
	}
	cmds := panes[0].Commands()
	if len(cmds) != 1 || cmds[0].Command() != "npm run dev" || !cmds[0].AutoRun() {
		t.Errorf("pane0 cmds: %+v", cmds)
	}
	if panes[1].Directory().String() != wd+"/backend" || panes[1].Commands()[0].AutoRun() {
		t.Errorf("pane1: %+v", panes[1])
	}
	// 永続化された run も終端状態
	saved, _ := e.runs.FindByID(ctx, runID)
	if saved.State != task.RunDone {
		t.Errorf("persisted run: %s", saved.State)
	}

	// 再実行: 既存ワークスペースを使う
	runID2, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runID2 == runID {
		t.Error("new run id expected")
	}
	if _, err := e.runs.FindByID(ctx, runID); !errors.Is(err, task.ErrNotFound) {
		t.Error("previous run should be deleted")
	}
	r2 := e.waitRun(t, runID2)
	last := r2.Steps[len(r2.Steps)-1]
	if r2.State != task.RunDone || last.Label != "ワークスペースを確認" || !strings.Contains(last.Log, "existing") || r2.WorkspaceID != got.WorkspaceID {
		t.Errorf("rerun: state=%s last=%+v", r2.State, last)
	}
	all, _ := e.ws.List(ctx)
	if len(all) != 1 {
		t.Errorf("workspace duplicated: %d", len(all))
	}
}

func TestRunner_FailRetrySkip(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	e.exec.CommandErr = func(c string) error {
		if c == "go mod download" {
			return errors.New("exit 1")
		}
		return nil
	}
	runID, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := e.waitRun(t, runID)
	if r.State != task.RunFailed || r.FailedStep() < 0 || !strings.Contains(r.Error, "backend: セットアップコマンド") {
		t.Fatalf("run: state=%s err=%q states=%v", r.State, r.Error, stepStates(r))
	}
	failed := r.FailedStep()
	if !strings.Contains(r.Steps[failed].Log, "error: exit 1") {
		t.Errorf("failed log: %q", r.Steps[failed].Log)
	}
	if r.Steps[len(r.Steps)-1].State != task.StepPending {
		t.Error("workspace step must remain pending")
	}
	got, _ := e.tasks.FindByID(ctx, tk.ID)
	if got.LocalState != task.StateNone || got.WorkspaceID != "" {
		t.Errorf("task after fail: %+v", got)
	}
	// 実行中でないので abort は不可
	if _, err := e.svc.AbortSetupRun(ctx, runID); !isValidation(err) || !errors.Is(err, ErrRunNotActive) {
		t.Errorf("abort inactive: %v", err)
	}
	// skip は失敗ステップにしか効かない(done 状態では拒否) — ここでは retry を先に試す
	e.exec.CommandErr = nil
	before := len(e.exec.Commands)
	snap, err := e.svc.RetrySetupRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != runID {
		t.Errorf("retry snapshot: %+v", snap)
	}
	r = e.waitResumed(t, runID, task.RunFailed)
	if r.State != task.RunDone {
		t.Fatalf("after retry: %s %q", r.State, r.Error)
	}
	// 失敗したステップだけ再実行される(前のステップは再実行しない)
	if len(e.exec.Commands)-before != 1 || e.exec.Commands[len(e.exec.Commands)-1].Command != "go mod download" {
		t.Errorf("retry commands: %v", e.exec.CommandList()[before:])
	}
	got, _ = e.tasks.FindByID(ctx, tk.ID)
	if got.LocalState != task.StateReady || got.WorkspaceID == "" {
		t.Errorf("task after retry: %+v", got)
	}
	if _, err := e.svc.RetrySetupRun(ctx, runID); !isValidation(err) {
		t.Errorf("retry done run: %v", err)
	}
	if _, err := e.svc.SkipSetupStep(ctx, runID); !isValidation(err) {
		t.Errorf("skip done run: %v", err)
	}
}

func TestRunner_Skip(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	e.exec.CopyErr = errors.New("disk full")
	runID, _ := e.svc.StartSetup(ctx, tk.ID)
	r := e.waitRun(t, runID)
	if r.State != task.RunFailed || r.Steps[r.FailedStep()].Kind != task.StepRepoCopy {
		t.Fatalf("run: %s %v", r.State, stepStates(r))
	}
	skipped := r.FailedStep()
	e.exec.CopyErr = nil
	if _, err := e.svc.SkipSetupStep(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r = e.waitResumed(t, runID, task.RunFailed)
	if r.State != task.RunDone || r.Steps[skipped].State != task.StepSkipped {
		t.Errorf("after skip: %s %v", r.State, stepStates(r))
	}
	if len(e.exec.Copies) != 0 {
		t.Error("skipped copy must not run")
	}
	if r.Progress() != len(r.Steps) {
		t.Errorf("progress %d/%d", r.Progress(), len(r.Steps))
	}
}

func TestRunner_Abort(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	e.exec.Block = make(chan struct{})
	runID, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	// コマンドがブロックされるまで待つ
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(e.exec.CommandList()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if len(e.exec.CommandList()) == 0 {
		t.Fatal("command never started")
	}
	// 実行中は二重開始不可
	if _, err := e.svc.StartSetup(ctx, tk.ID); !isValidation(err) {
		t.Errorf("start while preparing: %v", err)
	}
	if _, err := e.svc.RetrySetupRun(ctx, runID); !isValidation(err) {
		t.Errorf("retry while running: %v", err)
	}
	r, err := e.svc.AbortSetupRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != task.RunAborted {
		r = e.waitRun(t, runID)
	}
	if r.State != task.RunAborted || r.FinishedAt.IsZero() {
		t.Fatalf("run: state=%s finished=%v", r.State, r.FinishedAt)
	}
	setupIdx := -1
	for i, s := range r.Steps {
		if s.Kind == task.StepRepoSetup {
			setupIdx = i
			break
		}
	}
	if setupIdx < 0 || r.Steps[setupIdx].State != task.StepFailed || !strings.Contains(r.Steps[setupIdx].Log, "aborted") {
		t.Errorf("aborted step: %+v", r.Steps)
	}
	got, _ := e.tasks.FindByID(ctx, tk.ID)
	if got.LocalState != task.StateNone {
		t.Errorf("task after abort: %s", got.LocalState)
	}
	saved, _ := e.runs.FindByID(ctx, runID)
	if saved.State != task.RunAborted {
		t.Errorf("persisted: %s", saved.State)
	}
	// 中止後は再開できる
	close(e.exec.Block)
	e.exec.Block = nil
	if _, err := e.svc.RetrySetupRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r = e.waitResumed(t, runID, task.RunAborted)
	if r.State != task.RunDone {
		t.Errorf("resume after abort: %s %q", r.State, r.Error)
	}
}

func TestRunner_Subscribe(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	e.exec.Block = make(chan struct{})
	runID, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	snap, ch, cancel, err := e.svc.SubscribeSetupRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if snap.ID != runID || len(snap.Steps) == 0 {
		t.Fatalf("snapshot: %+v", snap)
	}
	close(e.exec.Block)
	var last *task.SetupRun
	timeout := time.After(2 * time.Second)
	got := 0
loop:
	for {
		select {
		case r := <-ch:
			got++
			last = r
			if r.State == task.RunDone {
				break loop
			}
		case <-timeout:
			t.Fatalf("no terminal snapshot (got %d updates, last=%+v)", got, last)
		}
	}
	if got < 2 {
		t.Errorf("expected several updates, got %d", got)
	}
	// スナップショットは独立したコピー(後から変えても購読者側に影響しない)
	last.Steps[0].Log = "tampered"
	again, _ := e.svc.GetSetupRun(ctx, runID)
	if again.Steps[0].Log == "tampered" {
		t.Error("snapshot must be a copy")
	}
	// 終了済み run の購読は現在値を返す
	snap2, _, cancel2, err := e.svc.SubscribeSetupRun(ctx, runID)
	if err != nil || snap2.State != task.RunDone {
		t.Errorf("subscribe finished: %v %+v", err, snap2)
	}
	cancel2()
	if _, _, _, err := e.svc.SubscribeSetupRun(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("subscribe missing: %v", err)
	}
	if _, err := e.svc.GetSetupRun(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
}

func TestStartSetup_Validation(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.importOne(t, "PROJ-1", "")
	if _, err := e.svc.StartSetup(ctx, tk.ID); !isValidation(err) {
		t.Errorf("empty env: %v", err)
	}
	tk.Env = task.Env{WorkDir: "~/w/PROJ-1", Layout: "single", Repos: []task.Repo{{Name: "x", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "nope"}}}}
	_ = e.tasks.Save(ctx, tk)
	if _, err := e.svc.StartSetup(ctx, tk.ID); !isValidation(err) {
		t.Errorf("unknown base clone: %v", err)
	}
	if _, err := e.svc.StartSetup(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing task: %v", err)
	}
	// 最小構成(リポジトリなし)でもワークスペースができる
	tk.Env = task.Env{WorkDir: "~/w/PROJ-1", Layout: "single"}
	_ = e.tasks.Save(ctx, tk)
	runID, err := e.svc.StartSetup(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := e.waitRun(t, runID)
	if r.State != task.RunDone || len(r.Steps) != 2 {
		t.Fatalf("minimal run: %s %v", r.State, stepStates(r))
	}
	got, _ := e.tasks.FindByID(ctx, tk.ID)
	w, err := e.ws.FindByID(ctx, mustWsID(got.WorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	panes := w.Panes()
	if len(panes) != 1 || panes[0].Directory().String() != "/home/test/w/PROJ-1" || panes[0].Title().String() != "" {
		t.Errorf("default pane: %+v", panes)
	}
}

func TestRunner_ExistingBranchIsCheckedOut(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	tk := e.setupTask(t)
	wd := "/home/test/work/PROJ-1"
	e.git.BranchLists[wd+"/frontend"] = []port.BranchInfo{{Name: "feature/PROJ-1-summary-proj-1", IsRemote: true}}
	runID, _ := e.svc.StartSetup(ctx, tk.ID)
	r := e.waitRun(t, runID)
	if r.State != task.RunDone {
		t.Fatalf("run: %s %q", r.State, r.Error)
	}
	if !contains2(e.git.Checkouts, apptest.CheckoutCall{Dir: wd + "/frontend", Branch: "feature/PROJ-1-summary-proj-1"}) {
		t.Errorf("existing branch should be checked out: %v", e.git.Checkouts)
	}
	for _, c := range e.git.CreateBranches {
		if c.Dir == wd+"/frontend" {
			t.Errorf("must not create branch when it exists: %v", c)
		}
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func contains2(list []apptest.CheckoutCall, v apptest.CheckoutCall) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
