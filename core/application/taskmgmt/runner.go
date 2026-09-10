package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// ErrRunNotActive は再開・中止の対象が実行中でないことを示す。
var ErrRunNotActive = errors.New("setup run is not active")

// runner はセットアップ実行を goroutine で回し、進捗を購読者へ配る。
// 1 タスクにつき同時 1 本。実行中の run はメモリ上の runState が正で、ステップ境界と
// ログ更新(間引き)のたびに永続化する。
type runner struct {
	s  *Service
	mu sync.Mutex
	// runs は実行中または購読者のいる run。終了後も購読者が居なくなるまで残す。
	runs map[string]*runState
}

// runState は 1 実行のメモリ上の状態。
type runState struct {
	mu     sync.Mutex
	run    *task.SetupRun
	cancel context.CancelFunc // 実行中のみ非 nil
	subs   map[chan *task.SetupRun]struct{}
	// lastSaved / lastPublished はログ更新の間引き用。
	lastSaved     time.Time
	lastPublished time.Time
}

func newRunner(s *Service) *runner {
	return &runner{s: s, runs: map[string]*runState{}}
}

// logThrottle はログ更新時の永続化・配信の最小間隔。
const logThrottle = 300 * time.Millisecond

// setupEnv はセットアップコマンドに渡す環境変数。
func setupEnv(t *task.Task, workDir string) []string {
	return []string{
		"TASK_KEY=" + t.JiraKey,
		"TASK_DIR=" + workDir,
		"TASK_BRANCH=" + t.Env.Branch,
	}
}

// snapshot は購読者に渡す独立したコピー。
func snapshot(r *task.SetupRun) *task.SetupRun {
	c := *r
	c.Steps = make([]task.Step, len(r.Steps))
	for i, s := range r.Steps {
		c.Steps[i] = s
		if s.Args != nil {
			c.Steps[i].Args = make(map[string]string, len(s.Args))
			for k, v := range s.Args {
				c.Steps[i].Args[k] = v
			}
		}
	}
	return &c
}

// publish は購読者へ現在の状態を配る。final=true は終端(必ず届くよう少し待つ)。
func (st *runState) publish(final bool) {
	snap := snapshot(st.run)
	for ch := range st.subs {
		if final {
			select {
			case ch <- snap:
			case <-time.After(time.Second):
			}
			continue
		}
		select {
		case ch <- snap:
		default: // 詰まっていれば落とす(次の更新で追いつく)
		}
	}
	st.lastPublished = time.Now()
}

// get は run をメモリ(実行中)またはストアから返す。
func (r *runner) get(ctx context.Context, id string) (*task.SetupRun, error) {
	r.mu.Lock()
	st, ok := r.runs[id]
	r.mu.Unlock()
	if ok {
		st.mu.Lock()
		defer st.mu.Unlock()
		return snapshot(st.run), nil
	}
	return r.s.d.Runs.FindByID(ctx, id)
}

// subscribe は現在の状態と更新チャネルを返す。
func (r *runner) subscribe(ctx context.Context, id string) (*task.SetupRun, <-chan *task.SetupRun, func(), error) {
	r.mu.Lock()
	st, ok := r.runs[id]
	if !ok {
		// 終了済み: ストアから読み、購読者用に載せる(更新は来ないが同じ経路にする)
		run, err := r.s.d.Runs.FindByID(ctx, id)
		if err != nil {
			r.mu.Unlock()
			return nil, nil, nil, err
		}
		st = &runState{run: run, subs: map[chan *task.SetupRun]struct{}{}}
		r.runs[id] = st
	}
	r.mu.Unlock()

	ch := make(chan *task.SetupRun, 16)
	st.mu.Lock()
	st.subs[ch] = struct{}{}
	snap := snapshot(st.run)
	st.mu.Unlock()
	cancel := func() {
		st.mu.Lock()
		delete(st.subs, ch)
		idle := len(st.subs) == 0 && st.cancel == nil
		st.mu.Unlock()
		if idle {
			r.mu.Lock()
			if cur, ok := r.runs[id]; ok && cur == st {
				delete(r.runs, id)
			}
			r.mu.Unlock()
		}
	}
	return snap, ch, cancel, nil
}

// updateTask は最新のタスクを読み直して変更を適用し保存する(ノート編集との競合を避ける)。
func (s *Service) updateTask(ctx context.Context, id string, fn func(t *task.Task) error) (*task.Task, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := fn(t); err != nil {
		return nil, err
	}
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return nil, fmt.Errorf("save task: %w", err)
	}
	return t, nil
}

// StartSetup はタスクのセットアップを開始し、run ID を返す。実行はバックグラウンド。
func (s *Service) StartSetup(ctx context.Context, taskID string) (string, error) {
	t, err := s.d.Tasks.FindByID(ctx, taskID)
	if err != nil {
		return "", err
	}
	if t.LocalState == task.StatePreparing {
		return "", apperr.Validation(errors.New("setup is already running"))
	}
	if t.Env.IsEmpty() {
		return "", apperr.Validation(errors.New("env is not configured (choose a template or edit the task)"))
	}
	if err := s.validateEnvRefs(ctx, t.Env); err != nil {
		return "", err
	}
	workDir, err := s.absPath(t.Env.WorkDir)
	if err != nil {
		return "", apperr.Validation(fmt.Errorf("work dir: %w", err))
	}
	bcs, err := s.d.BaseClones.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list base clones: %w", err)
	}
	bcMap := map[string]*task.BaseClone{}
	for _, b := range bcs {
		bcMap[b.ID] = b
	}
	settings, err := s.d.Settings.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("load settings: %w", err)
	}
	hasWS := false
	if t.WorkspaceID != "" {
		if _, err := s.d.Workspaces.FindByID(ctx, mustWsID(t.WorkspaceID)); err == nil {
			hasWS = true
		}
	}
	steps, err := PlanSteps(PlanInput{Task: t, WorkDir: workDir, BaseClones: bcMap, Settings: settings, HasWorkspace: hasWS})
	if err != nil {
		return "", apperr.Validation(err)
	}
	run, err := task.NewSetupRun(s.d.IDGen.NewID(), t.ID, steps, s.now())
	if err != nil {
		return "", apperr.Validation(err)
	}
	// 直近の実行だけ残す
	if t.SetupRunID != "" {
		_ = s.d.Runs.Delete(ctx, t.SetupRunID)
		s.runner.forget(t.SetupRunID)
	}
	run.Start(s.now())
	if err := s.d.Runs.Save(ctx, run); err != nil {
		return "", fmt.Errorf("save setup run: %w", err)
	}
	if _, err := s.updateTask(ctx, t.ID, func(t *task.Task) error {
		return t.BeginSetup(run.ID, s.now())
	}); err != nil {
		return "", apperr.Validation(err)
	}
	runID := run.ID
	s.runner.launch(run)
	return runID, nil
}

// forget は終了済み run のメモリ上の状態を捨てる(購読者が居ても新しい run に移る)。
func (r *runner) forget(id string) {
	r.mu.Lock()
	st, ok := r.runs[id]
	if ok && st.cancel == nil {
		delete(r.runs, id)
	}
	r.mu.Unlock()
}

// GetSetupRun は run を返す(実行中はメモリ上の最新)。
func (s *Service) GetSetupRun(ctx context.Context, id string) (*task.SetupRun, error) {
	return s.runner.get(ctx, id)
}

// SubscribeSetupRun は SSE 用に現在値と更新チャネルを返す。
func (s *Service) SubscribeSetupRun(ctx context.Context, id string) (*task.SetupRun, <-chan *task.SetupRun, func(), error) {
	return s.runner.subscribe(ctx, id)
}

// RetrySetupRun は失敗したステップから再実行する。
func (s *Service) RetrySetupRun(ctx context.Context, id string) (*task.SetupRun, error) {
	return s.resume(ctx, id, false)
}

// SkipSetupStep は失敗したステップを飛ばして続行する。
func (s *Service) SkipSetupStep(ctx context.Context, id string) (*task.SetupRun, error) {
	return s.resume(ctx, id, true)
}

// resume は失敗/中止した run を再開する。skip=true なら失敗ステップをスキップ済みにする。
func (s *Service) resume(ctx context.Context, id string, skip bool) (*task.SetupRun, error) {
	if s.runner.isActive(id) {
		return nil, apperr.Validation(errors.New("setup run is still running"))
	}
	run, err := s.d.Runs.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch run.State {
	case task.RunFailed, task.RunAborted, task.RunPending:
	default:
		return nil, apperr.Validation(fmt.Errorf("setup run is %s; nothing to resume", run.State))
	}
	if skip {
		i := run.FailedStep()
		if i < 0 {
			return nil, apperr.Validation(errors.New("no failed step to skip"))
		}
		if err := run.SkipStep(i); err != nil {
			return nil, apperr.Validation(err)
		}
	} else if run.State == task.RunAborted {
		// 中止で failed になったステップは再実行対象(FailedStep のまま NextStep が拾う)
		run.State = task.RunPending
	}
	// 呼び出し直後の GetSetupRun が failed のままにならないよう、ここで running にしてから保存する
	run.Start(s.now())
	if err := s.d.Runs.Save(ctx, run); err != nil {
		return nil, fmt.Errorf("save setup run: %w", err)
	}
	if _, err := s.updateTask(ctx, run.TaskID, func(t *task.Task) error {
		return t.BeginSetup(run.ID, s.now())
	}); err != nil {
		return nil, apperr.Validation(err)
	}
	snap := snapshot(run) // launch 後は goroutine が run を書き換えるので先に取る
	s.runner.launch(run)
	return snap, nil
}

// AbortSetupRun は実行中の run を中止する。
func (s *Service) AbortSetupRun(ctx context.Context, id string) (*task.SetupRun, error) {
	if !s.runner.abort(id) {
		return nil, apperr.Validation(ErrRunNotActive)
	}
	// 中止処理は goroutine 側で完了するので、少し待ってから現在値を返す
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && s.runner.isActive(id) {
		time.Sleep(20 * time.Millisecond)
	}
	return s.runner.get(ctx, id)
}

func (r *runner) isActive(id string) bool {
	r.mu.Lock()
	st, ok := r.runs[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cancel != nil
}

func (r *runner) abort(id string) bool {
	r.mu.Lock()
	st, ok := r.runs[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	st.mu.Lock()
	cancel := st.cancel
	st.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// launch は run を実行する goroutine を起動する。
func (r *runner) launch(run *task.SetupRun) {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	st, ok := r.runs[run.ID]
	if !ok {
		st = &runState{subs: map[chan *task.SetupRun]struct{}{}}
		r.runs[run.ID] = st
	}
	r.mu.Unlock()
	st.mu.Lock()
	st.run = run
	st.cancel = cancel
	st.mu.Unlock()
	go r.execute(ctx, st)
}

// stepLog は実行中ステップへのログ書き込み。永続化と配信は間引く。
type stepLog struct {
	r   *runner
	st  *runState
	idx int
}

func (l *stepLog) Write(p []byte) (int, error) {
	l.st.mu.Lock()
	l.st.run.AppendLog(l.idx, string(p))
	needSave := time.Since(l.st.lastSaved) > logThrottle
	needPub := time.Since(l.st.lastPublished) > logThrottle
	if needPub {
		l.st.publish(false)
	}
	var snap *task.SetupRun
	if needSave {
		snap = snapshot(l.st.run)
		l.st.lastSaved = time.Now()
	}
	l.st.mu.Unlock()
	if snap != nil {
		_ = l.r.s.d.Runs.Save(context.Background(), snap)
	}
	return len(p), nil
}

// execute はステップを順に実行する本体。
func (r *runner) execute(ctx context.Context, st *runState) {
	s := r.s
	bg := context.Background()

	st.mu.Lock()
	run := st.run
	run.Start(s.now())
	st.publish(false)
	st.mu.Unlock()
	_ = s.d.Runs.Save(bg, snapshot(run))

	t, err := s.d.Tasks.FindByID(bg, run.TaskID)
	if err != nil {
		r.finish(st, fmt.Errorf("load task: %w", err))
		return
	}
	workDir, err := s.absPath(t.Env.WorkDir)
	if err != nil {
		r.finish(st, fmt.Errorf("work dir: %w", err))
		return
	}

	for {
		if ctx.Err() != nil {
			r.finish(st, ctx.Err())
			return
		}
		st.mu.Lock()
		i := run.NextStep()
		if i < 0 {
			st.mu.Unlock()
			break
		}
		_ = run.BeginStep(i, s.now())
		step := run.Steps[i]
		st.publish(false)
		st.mu.Unlock()
		_ = s.d.Runs.Save(bg, snapshot(run))

		log := &stepLog{r: r, st: st, idx: i}
		stepErr := r.execStep(ctx, t, workDir, step, log, st)
		if stepErr != nil && ctx.Err() != nil {
			r.finish(st, ctx.Err())
			return
		}
		st.mu.Lock()
		run.FinishStep(i, stepErr, s.now())
		st.publish(stepErr != nil)
		st.mu.Unlock()
		_ = s.d.Runs.Save(bg, snapshot(run))
		if stepErr != nil {
			r.finish(st, nil)
			return
		}
	}

	st.mu.Lock()
	run.Complete(s.now())
	wsID := run.WorkspaceID
	st.mu.Unlock()
	_ = s.d.Runs.Save(bg, snapshot(run))
	_, _ = s.updateTask(bg, run.TaskID, func(t *task.Task) error {
		t.FinishSetup(wsID, s.now())
		return nil
	})
	r.finish(st, nil)
}

// finish は run を終端状態にして購読者へ最終通知し、実行中フラグを落とす。
// abortErr が非 nil なら中止として記録する。
func (r *runner) finish(st *runState, abortErr error) {
	s := r.s
	bg := context.Background()
	st.mu.Lock()
	run := st.run
	if abortErr != nil {
		if errors.Is(abortErr, context.Canceled) {
			run.Abort(s.now())
		} else {
			run.State = task.RunFailed
			run.Error = abortErr.Error()
			run.FinishedAt = s.now()
		}
	}
	state := run.State
	snap := snapshot(run)
	st.mu.Unlock()
	_ = s.d.Runs.Save(bg, snap)
	if state != task.RunDone {
		_, _ = s.updateTask(bg, run.TaskID, func(t *task.Task) error {
			t.FailSetup(s.now())
			return nil
		})
	}
	st.mu.Lock()
	st.publish(true)
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	idle := len(st.subs) == 0
	st.mu.Unlock()
	if idle {
		r.mu.Lock()
		if cur, ok := r.runs[run.ID]; ok && cur == st {
			delete(r.runs, run.ID)
		}
		r.mu.Unlock()
	}
}

// execStep は 1 ステップを実行する。
func (r *runner) execStep(ctx context.Context, t *task.Task, workDir string, step task.Step, log io.Writer, st *runState) error {
	s := r.s
	switch step.Kind {
	case task.StepMkdir:
		if err := s.d.Exec.MkdirAll(workDir); err != nil {
			return err
		}
		fmt.Fprintf(log, "created %s\n", workDir)
		return nil

	case task.StepBaseFetch:
		bc, err := s.d.BaseClones.FindByID(ctx, step.Args[argBaseClone])
		if err != nil {
			return fmt.Errorf("base clone: %w", err)
		}
		if err := s.updateBaseClone(ctx, bc); err != nil {
			return err
		}
		fmt.Fprintf(log, "fetched %s (default branch %s)\n", bc.Name, bc.DefaultBranch)
		return nil

	case task.StepRepoCopy:
		bc, err := s.d.BaseClones.FindByID(ctx, step.Args[argBaseClone])
		if err != nil {
			return fmt.Errorf("base clone: %w", err)
		}
		dst := repoDir(workDir, step.Args[argRepo])
		settings, err := s.d.Settings.Load(ctx)
		if err != nil {
			return err
		}
		var exclude []string
		if !settings.IncludeNodeModules {
			exclude = []string{"node_modules"}
		}
		fmt.Fprintf(log, "copy %s -> %s\n", bc.Path, dst)
		return s.d.Exec.CopyTree(ctx, bc.Path, dst, exclude, log)

	case task.StepRepoClone:
		dst := repoDir(workDir, step.Args[argRepo])
		fmt.Fprintf(log, "git clone %s %s\n", step.Args[argURL], dst)
		path, err := s.d.Git.Clone(step.Args[argURL], dst)
		if err != nil {
			return err
		}
		fmt.Fprintf(log, "cloned to %s\n", path)
		return nil

	case task.StepRepoBranch:
		dir := repoDir(workDir, step.Args[argRepo])
		branch := step.Args[argBranch]
		fmt.Fprintf(log, "git fetch --prune\n")
		if err := s.d.Git.Fetch(dir); err != nil {
			return err
		}
		branches, err := s.d.Git.Branches(dir)
		if err != nil {
			return err
		}
		for _, b := range branches {
			if b.Name == branch {
				fmt.Fprintf(log, "branch %s exists; git switch %s\n", branch, branch)
				return s.d.Git.Checkout(dir, branch)
			}
		}
		def, err := s.d.Git.DefaultBranch(dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(log, "git switch -c %s origin/%s\n", branch, def)
		return s.d.Git.CreateBranch(dir, branch, "origin/"+def)

	case task.StepRepoSetup:
		dir := repoDir(workDir, step.Args[argRepo])
		env := setupEnv(t, workDir)
		for _, c := range strings.Split(step.Args[argCommands], "\n") {
			if strings.TrimSpace(c) == "" {
				continue
			}
			fmt.Fprintf(log, "$ %s\n", c)
			if err := s.d.Exec.RunCommand(ctx, dir, c, env, log); err != nil {
				return err
			}
		}
		return nil

	case task.StepWorkspace:
		return r.execWorkspaceStep(ctx, t, workDir, log, st)
	}
	return fmt.Errorf("unknown step kind %q", step.Kind)
}

// execWorkspaceStep はワークスペースを作成してタスクと相互に紐付ける。
func (r *runner) execWorkspaceStep(ctx context.Context, t *task.Task, workDir string, log io.Writer, st *runState) error {
	s := r.s
	// 既存があればそのまま使う
	if t.WorkspaceID != "" {
		if w, err := s.d.Workspaces.FindByID(ctx, mustWsID(t.WorkspaceID)); err == nil {
			if w.TaskID() != t.ID {
				w.LinkTask(t.ID)
				if err := s.d.Workspaces.Save(ctx, w); err != nil {
					return err
				}
			}
			fmt.Fprintf(log, "using existing workspace %q\n", w.Name().String())
			st.mu.Lock()
			st.run.WorkspaceID = t.WorkspaceID
			st.mu.Unlock()
			return nil
		}
	}
	id, err := domain.NewWorkspaceId(s.d.IDGen.NewID())
	if err != nil {
		return err
	}
	name, err := domain.NewWorkspaceName(workspaceName(t))
	if err != nil {
		return err
	}
	layout := domain.LayoutPreset(t.Env.Layout)
	if !layout.IsValid() {
		layout = domain.LayoutSingle
	}
	w, err := domain.NewWorkspace(id, name, layout)
	if err != nil {
		return err
	}
	specs := t.Env.Panes
	if len(specs) == 0 {
		specs = []task.PaneSpec{{Slot: 0}}
	}
	for _, sp := range specs {
		dir := workDir
		if sp.RepoName != "" {
			dir = repoDir(workDir, sp.RepoName)
		}
		pid, err := domain.NewPaneId(s.d.IDGen.NewID())
		if err != nil {
			return err
		}
		dp, err := domain.NewDirectoryPath(dir)
		if err != nil {
			return err
		}
		slot, err := domain.NewSlotIndex(sp.Slot)
		if err != nil {
			return err
		}
		title, err := domain.NewPaneTitle(sp.RepoName)
		if err != nil {
			return err
		}
		host, _ := domain.NewRemoteHost("")
		var cmds []domain.StartupCommand
		for _, c := range sp.Commands {
			sc, err := domain.NewStartupCommand(c.Command, c.AutoRun)
			if err != nil {
				return err
			}
			cmds = append(cmds, sc)
		}
		p, err := domain.NewPane(pid, dp, slot, title, host, cmds)
		if err != nil {
			return err
		}
		if err := w.AddPane(p); err != nil {
			return err
		}
	}
	w.LinkTask(t.ID)
	if err := s.d.Workspaces.Save(ctx, w); err != nil {
		return err
	}
	if _, err := s.updateTask(ctx, t.ID, func(t *task.Task) error {
		t.LinkWorkspace(id.String(), s.now())
		return nil
	}); err != nil {
		return err
	}
	t.WorkspaceID = id.String()
	fmt.Fprintf(log, "created workspace %q (%s, %d panes)\n", name.String(), layout, len(specs))
	st.mu.Lock()
	st.run.WorkspaceID = id.String()
	st.mu.Unlock()
	return nil
}
