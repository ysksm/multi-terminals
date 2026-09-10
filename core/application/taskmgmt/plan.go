package taskmgmt

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

// PlanInput はステップ列生成の入力。pure にするため必要な情報を全て持ち込む。
type PlanInput struct {
	Task       *task.Task
	WorkDir    string // 展開済み絶対パス
	BaseClones map[string]*task.BaseClone
	Settings   task.Settings
	// HasWorkspace はタスクに既にワークスペースが紐付いているか(再実行時は作り直さない)。
	HasWorkspace bool
}

// argKeys: Step.Args のキー。ランナーとの取り決め。
const (
	argRepo      = "repo"      // リポジトリ名
	argBaseClone = "baseClone" // ベースクローン ID
	argURL       = "url"
	argBranch    = "branch"
	argCommands  = "commands" // "\n" 区切り
)

// PlanSteps はタスクの環境定義からセットアップ手順を決定的に生成する。
// 順序: mkdir → (ベース更新) → 各リポジトリ: 取得 → ブランチ → セットアップ → ワークスペース。
func PlanSteps(in PlanInput) ([]task.Step, error) {
	t := in.Task
	if t == nil {
		return nil, fmt.Errorf("plan: task is nil")
	}
	if t.Env.IsEmpty() {
		return nil, fmt.Errorf("plan: env is not configured")
	}
	if strings.TrimSpace(in.WorkDir) == "" {
		return nil, fmt.Errorf("plan: workDir is empty")
	}
	var steps []task.Step
	steps = append(steps, task.Step{
		Kind:    task.StepMkdir,
		Label:   "作業フォルダを作成",
		Command: "mkdir -p " + in.WorkDir,
		Dir:     in.WorkDir,
	})

	// ベースクローンの更新(使うものだけ、1 回ずつ)
	if in.Settings.AutoFetchBeforeSetup {
		seen := map[string]bool{}
		for _, r := range t.Env.Repos {
			if r.Source.Kind != task.SourceBaseCopy || seen[r.Source.BaseCloneID] {
				continue
			}
			seen[r.Source.BaseCloneID] = true
			bc, ok := in.BaseClones[r.Source.BaseCloneID]
			if !ok {
				return nil, fmt.Errorf("plan: repo %q refers to unknown base clone %q", r.Name, r.Source.BaseCloneID)
			}
			steps = append(steps, task.Step{
				Kind:    task.StepBaseFetch,
				Label:   fmt.Sprintf("ベース %s を最新化", bc.Name),
				Command: "git fetch --prune && git pull --ff-only",
				Dir:     bc.Path,
				Args:    map[string]string{argBaseClone: bc.ID},
			})
		}
	}

	for _, r := range t.Env.Repos {
		dir := filepath.Join(in.WorkDir, r.Name)
		switch r.Source.Kind {
		case task.SourceBaseCopy:
			bc, ok := in.BaseClones[r.Source.BaseCloneID]
			if !ok {
				return nil, fmt.Errorf("plan: repo %q refers to unknown base clone %q", r.Name, r.Source.BaseCloneID)
			}
			cmd := fmt.Sprintf("cp -R %s %s", bc.Path, dir)
			if !in.Settings.IncludeNodeModules {
				cmd += "  (node_modules を除く)"
			}
			steps = append(steps, task.Step{
				Kind:    task.StepRepoCopy,
				Label:   fmt.Sprintf("%s: ベースクローンからコピー", r.Name),
				Command: cmd,
				Dir:     in.WorkDir,
				Args:    map[string]string{argRepo: r.Name, argBaseClone: bc.ID},
			})
		case task.SourceClone:
			steps = append(steps, task.Step{
				Kind:    task.StepRepoClone,
				Label:   fmt.Sprintf("%s: git clone", r.Name),
				Command: fmt.Sprintf("git clone %s %s", r.Source.URL, dir),
				Dir:     in.WorkDir,
				Args:    map[string]string{argRepo: r.Name, argURL: r.Source.URL},
			})
		default:
			return nil, fmt.Errorf("plan: repo %q has invalid source kind %q", r.Name, r.Source.Kind)
		}
		if strings.TrimSpace(t.Env.Branch) != "" {
			steps = append(steps, task.Step{
				Kind:    task.StepRepoBranch,
				Label:   fmt.Sprintf("%s: 最新化してブランチを作成", r.Name),
				Command: fmt.Sprintf("git fetch --prune && git switch -c %s origin/<default>", t.Env.Branch),
				Dir:     dir,
				Args:    map[string]string{argRepo: r.Name, argBranch: t.Env.Branch},
			})
		}
		if cmds := nonEmpty(r.SetupCommands); len(cmds) > 0 {
			steps = append(steps, task.Step{
				Kind:    task.StepRepoSetup,
				Label:   fmt.Sprintf("%s: セットアップコマンド", r.Name),
				Command: strings.Join(cmds, " && "),
				Dir:     dir,
				Args:    map[string]string{argRepo: r.Name, argCommands: strings.Join(cmds, "\n")},
			})
		}
	}

	label := "ワークスペースを作成"
	cmd := fmt.Sprintf("「%s」 %s · %d ペイン", workspaceName(t), t.Env.Layout, len(t.Env.Panes))
	if in.HasWorkspace {
		label = "ワークスペースを確認"
		cmd = "既存のワークスペースをそのまま使う"
	}
	steps = append(steps, task.Step{
		Kind:    task.StepWorkspace,
		Label:   label,
		Command: cmd,
		Dir:     in.WorkDir,
	})
	return steps, nil
}

// workspaceName はタスクから作るワークスペース名("PROJ-1301 概要")。
func workspaceName(t *task.Task) string {
	if strings.TrimSpace(t.Jira.Summary) == "" {
		return t.JiraKey
	}
	return t.JiraKey + " " + strings.TrimSpace(t.Jira.Summary)
}

func nonEmpty(cmds []string) []string {
	out := cmds[:0:0]
	for _, c := range cmds {
		if strings.TrimSpace(c) != "" {
			out = append(out, strings.TrimSpace(c))
		}
	}
	return out
}
