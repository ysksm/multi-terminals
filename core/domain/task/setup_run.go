package task

import (
	"errors"
	"fmt"
	"time"
)

// RunState はセットアップ実行全体の状態。
type RunState string

const (
	RunPending RunState = "pending"
	RunRunning RunState = "running"
	RunFailed  RunState = "failed"
	RunDone    RunState = "done"
	RunAborted RunState = "aborted"
)

// StepState は 1 ステップの状態。
type StepState string

const (
	StepPending StepState = "pending"
	StepRunning StepState = "running"
	StepDone    StepState = "done"
	StepFailed  StepState = "failed"
	StepSkipped StepState = "skipped"
)

// StepKind はステップの種類。ランナーが実行方法を選ぶキー。
type StepKind string

const (
	StepMkdir      StepKind = "mkdir"       // 作業フォルダ作成
	StepBaseFetch  StepKind = "base:fetch"  // ベースクローンの更新
	StepRepoCopy   StepKind = "repo:copy"   // ベースクローンからコピー
	StepRepoClone  StepKind = "repo:clone"  // git clone
	StepRepoBranch StepKind = "repo:branch" // fetch + ブランチ作成/切替
	StepRepoSetup  StepKind = "repo:setup"  // セットアップコマンド
	StepWorkspace  StepKind = "workspace"   // ワークスペース作成
)

// Step はセットアップの 1 手順。Command は表示用(実行方法は Kind と Args で決まる)。
type Step struct {
	Index      int               `json:"index"`
	Kind       StepKind          `json:"kind"`
	Label      string            `json:"label"`
	Command    string            `json:"command"`
	Dir        string            `json:"dir"`
	Args       map[string]string `json:"args,omitempty"`
	State      StepState         `json:"state"`
	StartedAt  time.Time         `json:"startedAt,omitempty"`
	DurationMs int64             `json:"durationMs"`
	Log        string            `json:"log"`
}

// SetupRun はタスク 1 件のセットアップ実行(集約ルート)。Task ごとに直近 1 件を保持。
type SetupRun struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"taskId"`
	State      RunState  `json:"state"`
	Steps      []Step    `json:"steps"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	// WorkspaceID は workspace ステップで作成したワークスペース。
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// GetID は Repository[T] 用の識別子。
func (r *SetupRun) GetID() string { return r.ID }

// NewSetupRun は pending 状態の実行を作る。
func NewSetupRun(id, taskID string, steps []Step, now time.Time) (*SetupRun, error) {
	if id == "" || taskID == "" {
		return nil, errors.New("setup run id and task id must not be empty")
	}
	if len(steps) == 0 {
		return nil, errors.New("setup run needs at least one step")
	}
	for i := range steps {
		steps[i].Index = i
		steps[i].State = StepPending
	}
	return &SetupRun{ID: id, TaskID: taskID, State: RunPending, Steps: steps, StartedAt: now}, nil
}

// NextStep は次に実行すべきステップの index を返す。無ければ -1。
// failed のステップは「そこから再実行」の対象なので次として返す。
func (r *SetupRun) NextStep() int {
	for i, s := range r.Steps {
		if s.State == StepPending || s.State == StepFailed {
			return i
		}
	}
	return -1
}

// Progress は完了(done/skipped)したステップ数を返す。
func (r *SetupRun) Progress() int {
	n := 0
	for _, s := range r.Steps {
		if s.State == StepDone || s.State == StepSkipped {
			n++
		}
	}
	return n
}

// FailedStep は失敗中のステップ index。無ければ -1。
func (r *SetupRun) FailedStep() int {
	for i, s := range r.Steps {
		if s.State == StepFailed {
			return i
		}
	}
	return -1
}

// Start は実行中にする(再実行時も呼ぶ)。
func (r *SetupRun) Start(now time.Time) {
	r.State = RunRunning
	r.Error = ""
	r.FinishedAt = time.Time{}
	if r.StartedAt.IsZero() {
		r.StartedAt = now
	}
}

// BeginStep はステップを実行中にする。
func (r *SetupRun) BeginStep(i int, now time.Time) error {
	if i < 0 || i >= len(r.Steps) {
		return fmt.Errorf("step %d out of range", i)
	}
	r.Steps[i].State = StepRunning
	r.Steps[i].StartedAt = now
	r.Steps[i].Log = ""
	r.Steps[i].DurationMs = 0
	return nil
}

// AppendLog はステップのログを追記する。
func (r *SetupRun) AppendLog(i int, chunk string) {
	if i < 0 || i >= len(r.Steps) {
		return
	}
	r.Steps[i].Log += chunk
}

// FinishStep はステップの成否を記録する。
func (r *SetupRun) FinishStep(i int, err error, now time.Time) {
	if i < 0 || i >= len(r.Steps) {
		return
	}
	s := &r.Steps[i]
	s.DurationMs = now.Sub(s.StartedAt).Milliseconds()
	if err != nil {
		s.State = StepFailed
		if s.Log != "" && s.Log[len(s.Log)-1] != '\n' {
			s.Log += "\n"
		}
		s.Log += "error: " + err.Error() + "\n"
		r.State = RunFailed
		r.Error = fmt.Sprintf("step %d (%s): %v", i+1, s.Label, err)
		r.FinishedAt = now
		return
	}
	s.State = StepDone
}

// SkipStep は失敗中のステップをスキップ済みにする。失敗中でなければエラー。
func (r *SetupRun) SkipStep(i int) error {
	if i < 0 || i >= len(r.Steps) {
		return fmt.Errorf("step %d out of range", i)
	}
	if r.Steps[i].State != StepFailed {
		return fmt.Errorf("step %d is not failed", i)
	}
	r.Steps[i].State = StepSkipped
	r.State = RunPending
	r.Error = ""
	return nil
}

// Complete は全ステップ完了を記録する。
func (r *SetupRun) Complete(now time.Time) {
	r.State = RunDone
	r.Error = ""
	r.FinishedAt = now
}

// Abort は中止を記録する。実行中のステップは失敗扱いにする。
func (r *SetupRun) Abort(now time.Time) {
	for i := range r.Steps {
		if r.Steps[i].State == StepRunning {
			r.Steps[i].State = StepFailed
			r.Steps[i].DurationMs = now.Sub(r.Steps[i].StartedAt).Milliseconds()
			r.Steps[i].Log += "aborted\n"
		}
	}
	r.State = RunAborted
	r.FinishedAt = now
}

// IsActive は実行中か(再開・中止の対象)。
func (r *SetupRun) IsActive() bool { return r.State == RunRunning }
