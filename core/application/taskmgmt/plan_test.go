package taskmgmt

import (
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

var planNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func planTask() *task.Task {
	t, _ := task.NewTask("t1", "PROJ-1", task.JiraSnapshot{Summary: "Jira 設定"}, planNow)
	t.Env = task.Env{
		WorkDir: "~/work/PROJ-1", Branch: "feature/PROJ-1-jira", Layout: "split_vertical",
		Repos: []task.Repo{
			{Name: "frontend", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}, SetupCommands: []string{"npm ci", " ", "npm run build"}},
			{Name: "backend", Source: task.RepoSource{Kind: task.SourceClone, URL: "git@x:be.git"}},
		},
		Panes: []task.PaneSpec{{Slot: 0, RepoName: "frontend"}, {Slot: 1, RepoName: "backend"}},
	}
	return t
}

func planBases() map[string]*task.BaseClone {
	return map[string]*task.BaseClone{"b1": {ID: "b1", Name: "frontend", Path: "/base/frontend", URL: "u"}}
}

func kinds(steps []task.Step) []task.StepKind {
	out := make([]task.StepKind, len(steps))
	for i, s := range steps {
		out[i] = s.Kind
	}
	return out
}

func TestPlanSteps_Order(t *testing.T) {
	steps, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: "/home/test/work/PROJ-1", BaseClones: planBases(), Settings: task.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	want := []task.StepKind{
		task.StepMkdir, task.StepBaseFetch,
		task.StepRepoCopy, task.StepRepoBranch, task.StepRepoSetup,
		task.StepRepoClone, task.StepRepoBranch,
		task.StepWorkspace,
	}
	got := kinds(steps)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
	if steps[0].Dir != "/home/test/work/PROJ-1" {
		t.Errorf("mkdir dir: %q", steps[0].Dir)
	}
	if steps[1].Args[argBaseClone] != "b1" || steps[1].Dir != "/base/frontend" {
		t.Errorf("base fetch: %+v", steps[1])
	}
	if steps[2].Args[argRepo] != "frontend" || steps[2].Args[argBaseClone] != "b1" || !strings.Contains(steps[2].Command, "/base/frontend") {
		t.Errorf("copy: %+v", steps[2])
	}
	if strings.Contains(steps[2].Command, "node_modules") {
		t.Error("IncludeNodeModules=true must not mention exclusion")
	}
	if steps[3].Args[argBranch] != "feature/PROJ-1-jira" || steps[3].Dir != "/home/test/work/PROJ-1/frontend" {
		t.Errorf("branch: %+v", steps[3])
	}
	if steps[4].Args[argCommands] != "npm ci\nnpm run build" || steps[4].Command != "npm ci && npm run build" {
		t.Errorf("setup: %+v", steps[4])
	}
	if steps[5].Args[argURL] != "git@x:be.git" || steps[5].Args[argRepo] != "backend" {
		t.Errorf("clone: %+v", steps[5])
	}
	if steps[7].Label != "ワークスペースを作成" || !strings.Contains(steps[7].Command, "PROJ-1 Jira 設定") || !strings.Contains(steps[7].Command, "2 ペイン") {
		t.Errorf("workspace: %+v", steps[7])
	}
	// Index / State は NewSetupRun が付けるので PlanSteps では未設定でよい
}

func TestPlanSteps_NoAutoFetchAndNoNodeModules(t *testing.T) {
	st := task.Settings{AutoFetchBeforeSetup: false, IncludeNodeModules: false}
	steps, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: "/w", BaseClones: planBases(), Settings: st})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if s.Kind == task.StepBaseFetch {
			t.Fatal("base fetch must be omitted when AutoFetchBeforeSetup=false")
		}
		if s.Kind == task.StepRepoCopy && !strings.Contains(s.Command, "node_modules") {
			t.Error("copy command should mention node_modules exclusion")
		}
	}
}

func TestPlanSteps_BaseFetchOncePerClone(t *testing.T) {
	tk := planTask()
	tk.Env.Repos = append(tk.Env.Repos, task.Repo{Name: "shared", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}})
	steps, err := PlanSteps(PlanInput{Task: tk, WorkDir: "/w", BaseClones: planBases(), Settings: task.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range steps {
		if s.Kind == task.StepBaseFetch {
			n++
		}
	}
	if n != 1 {
		t.Errorf("base fetch steps = %d, want 1", n)
	}
}

func TestPlanSteps_NoBranchNoSetup(t *testing.T) {
	tk := planTask()
	tk.Env.Branch = ""
	tk.Env.Repos = []task.Repo{{Name: "backend", Source: task.RepoSource{Kind: task.SourceClone, URL: "u"}}}
	tk.Env.Panes = nil
	steps, err := PlanSteps(PlanInput{Task: tk, WorkDir: "/w", Settings: task.Settings{}})
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(steps)
	want := []task.StepKind{task.StepMkdir, task.StepRepoClone, task.StepWorkspace}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestPlanSteps_HasWorkspace(t *testing.T) {
	steps, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: "/w", BaseClones: planBases(), Settings: task.Settings{}, HasWorkspace: true})
	if err != nil {
		t.Fatal(err)
	}
	last := steps[len(steps)-1]
	if last.Kind != task.StepWorkspace || last.Label != "ワークスペースを確認" || !strings.Contains(last.Command, "既存") {
		t.Errorf("last step: %+v", last)
	}
}

func TestPlanSteps_Errors(t *testing.T) {
	if _, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: "/w", BaseClones: nil, Settings: task.Settings{}}); err == nil {
		t.Error("unknown base clone should fail")
	}
	if _, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: "/w", BaseClones: nil, Settings: task.DefaultSettings()}); err == nil {
		t.Error("unknown base clone (auto fetch) should fail")
	}
	empty, _ := task.NewTask("t", "PROJ-2", task.JiraSnapshot{}, planNow)
	if _, err := PlanSteps(PlanInput{Task: empty, WorkDir: "/w"}); err == nil {
		t.Error("empty env should fail")
	}
	if _, err := PlanSteps(PlanInput{Task: planTask(), WorkDir: " ", BaseClones: planBases()}); err == nil {
		t.Error("empty workDir should fail")
	}
	if _, err := PlanSteps(PlanInput{}); err == nil {
		t.Error("nil task should fail")
	}
	bad := planTask()
	bad.Env.Repos[0].Source.Kind = "weird"
	if _, err := PlanSteps(PlanInput{Task: bad, WorkDir: "/w", BaseClones: planBases()}); err == nil {
		t.Error("bad source kind should fail")
	}
}

func TestWorkspaceName(t *testing.T) {
	tk := planTask()
	if got := workspaceName(tk); got != "PROJ-1 Jira 設定" {
		t.Errorf("got %q", got)
	}
	tk.Jira.Summary = "  "
	if got := workspaceName(tk); got != "PROJ-1" {
		t.Errorf("got %q", got)
	}
}
