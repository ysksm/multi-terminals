package jsonstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

func TestTaskCollection_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, err := NewTaskRepository(t.TempDir())
	if err != nil {
		t.Fatalf("NewTaskRepository: %v", err)
	}

	now := time.Date(2026, 9, 10, 12, 41, 0, 0, time.FixedZone("JST", 9*3600))
	tk, err := task.NewTask("t1", "PROJ-1301", task.JiraSnapshot{Summary: "Jira 連携", FetchedAt: now}, now)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	tk.Note = "メモ"
	tk.Env = task.Env{
		WorkDir: "~/work/PROJ-1301",
		Branch:  "feature/PROJ-1301-jira",
		Layout:  "grid_2x2",
		Repos:   []task.Repo{{Name: "frontend", Source: task.RepoSource{Kind: task.SourceClone, URL: "git@x:y.git"}, SetupCommands: []string{"npm ci"}}},
		Panes:   []task.PaneSpec{{Slot: 0, RepoName: "frontend", Commands: []task.Command{{Command: "npm run dev", AutoRun: true}}}},
	}

	if err := repo.Save(ctx, tk); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "t1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.JiraKey != "PROJ-1301" || got.Note != "メモ" || got.LocalState != task.StateNone {
		t.Errorf("unexpected record: %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.Jira.FetchedAt.Equal(now) {
		t.Errorf("time fields not preserved: %v / %v", got.CreatedAt, got.Jira.FetchedAt)
	}
	if len(got.Env.Repos) != 1 || got.Env.Repos[0].SetupCommands[0] != "npm ci" {
		t.Errorf("env not preserved: %+v", got.Env)
	}
	if len(got.Env.Panes) != 1 || !got.Env.Panes[0].Commands[0].AutoRun {
		t.Errorf("panes not preserved: %+v", got.Env.Panes)
	}

	// overwrite
	got.Note = "更新"
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	again, _ := repo.FindByID(ctx, "t1")
	if again.Note != "更新" {
		t.Errorf("overwrite not applied: %q", again.Note)
	}

	if err := repo.Delete(ctx, "t1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, "t1"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
	if err := repo.Delete(ctx, "t1"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestTaskCollection_ListSorted(t *testing.T) {
	ctx := context.Background()
	repo, err := NewTaskRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, id := range []string{"c", "a", "b"} {
		tk, _ := task.NewTask(id, "PROJ-1", task.JiraSnapshot{}, now)
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != "a" || list[1].ID != "b" || list[2].ID != "c" {
		t.Errorf("expected sorted [a b c], got %+v", list)
	}

	empty, err := NewTemplateRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := empty.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("expected empty non-nil slice, got %#v", got)
	}
}

func TestTaskCollection_InvalidID(t *testing.T) {
	ctx := context.Background()
	repo, err := NewTaskRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "..", "a/b", "../x", "a..b", string(os.PathSeparator) + "abs"} {
		if _, err := repo.FindByID(ctx, id); err == nil || errors.Is(err, task.ErrNotFound) {
			t.Errorf("FindByID(%q): expected invalid-id error, got %v", id, err)
		}
		if err := repo.Delete(ctx, id); err == nil || errors.Is(err, task.ErrNotFound) {
			t.Errorf("Delete(%q): expected invalid-id error, got %v", id, err)
		}
		tk := &task.Task{ID: id, JiraKey: "PROJ-1", LocalState: task.StateNone}
		if err := repo.Save(ctx, tk); err == nil {
			t.Errorf("Save(%q): expected invalid-id error", id)
		}
	}
}

func TestTaskCollection_RejectsFutureVersion(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	repo, err := NewTaskRepository(base)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "tasks", "future.json")
	if err := os.WriteFile(path, []byte(`{"version": 99, "record": {"id": "future", "jiraKey": "PROJ-1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, "future"); err == nil {
		t.Error("FindByID: expected error for version 99")
	}
	if _, err := repo.List(ctx); err == nil {
		t.Error("List: expected error for version 99")
	}

	// 壊れたファイルもファイル名付きで全体エラー
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(ctx); err == nil {
		t.Error("List: expected error for corrupt file")
	}
}

func TestSetupRunCollection_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, err := NewSetupRunRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 13, 0, 0, 0, time.UTC)
	run, err := task.NewSetupRun("r1", "t1", []task.Step{
		{Kind: task.StepMkdir, Label: "作業フォルダを作成", Command: "mkdir -p ~/work/PROJ-1", Args: map[string]string{"path": "~/work/PROJ-1"}},
		{Kind: task.StepWorkspace, Label: "ワークスペースを作成"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	run.Start(now)
	if err := run.BeginStep(0, now); err != nil {
		t.Fatal(err)
	}
	run.AppendLog(0, "ok\n")
	run.FinishStep(0, nil, now.Add(1500*time.Millisecond))

	if err := repo.Save(ctx, run); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "r1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.TaskID != "t1" || got.State != task.RunRunning || len(got.Steps) != 2 {
		t.Errorf("unexpected run: %+v", got)
	}
	if got.Steps[0].State != task.StepDone || got.Steps[0].DurationMs != 1500 || got.Steps[0].Log != "ok\n" {
		t.Errorf("step 0 not preserved: %+v", got.Steps[0])
	}
	if !got.Steps[0].StartedAt.Equal(now) || !got.StartedAt.Equal(now) {
		t.Errorf("time fields not preserved: %+v", got.Steps[0])
	}
	if got.Steps[0].Args["path"] != "~/work/PROJ-1" {
		t.Errorf("args not preserved: %+v", got.Steps[0].Args)
	}
	if got.Steps[1].State != task.StepPending || got.Steps[1].Index != 1 {
		t.Errorf("step 1 not preserved: %+v", got.Steps[1])
	}
}

func TestBaseCloneCollection_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, err := NewBaseCloneRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	bc := &task.BaseClone{ID: "b1", Name: "frontend", URL: "git@x:y.git", Path: "/tmp/base/frontend", CreatedAt: now}
	bc.MarkFetched("main", 4096, now)
	if err := repo.Save(ctx, bc); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(ctx, "b1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultBranch != "main" || got.SizeBytes != 4096 || !got.LastFetchedAt.Equal(now) {
		t.Errorf("unexpected base clone: %+v", got)
	}
}
