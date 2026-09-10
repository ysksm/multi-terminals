package task

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func threeSteps() []Step {
	return []Step{
		{Kind: StepMkdir, Label: "mkdir"},
		{Kind: StepRepoClone, Label: "clone"},
		{Kind: StepWorkspace, Label: "ws"},
	}
}

func TestNewSetupRun(t *testing.T) {
	r, err := NewSetupRun("r1", "t1", threeSteps(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != RunPending || len(r.Steps) != 3 {
		t.Fatalf("run: %+v", r)
	}
	for i, s := range r.Steps {
		if s.Index != i || s.State != StepPending {
			t.Errorf("step %d: %+v", i, s)
		}
	}
	if _, err := NewSetupRun("", "t1", threeSteps(), t0); err == nil {
		t.Error("empty id")
	}
	if _, err := NewSetupRun("r", "t1", nil, t0); err == nil {
		t.Error("no steps")
	}
}

func TestSetupRun_StepFlow(t *testing.T) {
	r, _ := NewSetupRun("r1", "t1", threeSteps(), t0)
	if r.NextStep() != 0 || r.Progress() != 0 || r.FailedStep() != -1 {
		t.Fatal("initial")
	}
	r.Start(t0)
	if r.State != RunRunning {
		t.Fatal("Start")
	}
	if err := r.BeginStep(0, t0); err != nil {
		t.Fatal(err)
	}
	if r.Steps[0].State != StepRunning {
		t.Fatal("BeginStep")
	}
	r.AppendLog(0, "hello")
	r.AppendLog(99, "ignored")
	r.FinishStep(0, nil, t0.Add(1500*time.Millisecond))
	if r.Steps[0].State != StepDone || r.Steps[0].DurationMs != 1500 || r.Steps[0].Log != "hello" {
		t.Errorf("FinishStep ok: %+v", r.Steps[0])
	}
	if r.NextStep() != 1 || r.Progress() != 1 {
		t.Errorf("next=%d progress=%d", r.NextStep(), r.Progress())
	}
	_ = r.BeginStep(1, t0)
	r.AppendLog(1, "out")
	r.FinishStep(1, errors.New("boom"), t0.Add(time.Second))
	if r.Steps[1].State != StepFailed || r.State != RunFailed || r.FailedStep() != 1 {
		t.Errorf("FinishStep fail: %+v state=%s", r.Steps[1], r.State)
	}
	if !strings.Contains(r.Steps[1].Log, "out\nerror: boom") || !strings.Contains(r.Error, "clone") {
		t.Errorf("log=%q err=%q", r.Steps[1].Log, r.Error)
	}
	// failed は再実行対象として NextStep が返す
	if r.NextStep() != 1 {
		t.Errorf("failed step should be next, got %d", r.NextStep())
	}
	if err := r.SkipStep(0); err == nil {
		t.Error("skipping non-failed step should fail")
	}
	if err := r.SkipStep(1); err != nil {
		t.Fatal(err)
	}
	if r.Steps[1].State != StepSkipped || r.State != RunPending || r.Error != "" || r.NextStep() != 2 || r.Progress() != 2 {
		t.Errorf("SkipStep: %+v", r)
	}
	r.Start(t0.Add(time.Hour))
	if !r.StartedAt.Equal(t0) {
		t.Error("Start must keep original StartedAt")
	}
	_ = r.BeginStep(2, t0)
	r.FinishStep(2, nil, t0)
	if r.NextStep() != -1 {
		t.Error("all done")
	}
	r.Complete(t0)
	if r.State != RunDone || r.FinishedAt.IsZero() || r.IsActive() {
		t.Errorf("Complete: %+v", r)
	}
	if err := r.BeginStep(5, t0); err == nil {
		t.Error("out of range")
	}
}

func TestSetupRun_Abort(t *testing.T) {
	r, _ := NewSetupRun("r1", "t1", threeSteps(), t0)
	r.Start(t0)
	_ = r.BeginStep(0, t0)
	if !r.IsActive() {
		t.Fatal("running should be active")
	}
	r.Abort(t0.Add(2 * time.Second))
	if r.State != RunAborted || r.Steps[0].State != StepFailed || r.Steps[0].DurationMs != 2000 {
		t.Errorf("Abort: %+v", r)
	}
	if !strings.Contains(r.Steps[0].Log, "aborted") {
		t.Error("abort log")
	}
	if r.Steps[1].State != StepPending {
		t.Error("pending steps untouched")
	}
	// 中止で失敗したステップは再実行の起点になる
	if r.NextStep() != 0 {
		t.Errorf("NextStep after abort = %d", r.NextStep())
	}
}
