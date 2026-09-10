package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// jiraKeyInText は自由入力(カンマ区切り・URL 貼り付け)から番号を拾う。
var jiraKeyInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*-\d+`)

// ParseJiraKeys は入力文字列から Jira 番号を抽出し、大文字化・重複除去して返す。
// "PROJ-1301, PROJ-1302" や "https://x.atlassian.net/browse/PROJ-1301" を受け付ける。
func ParseJiraKeys(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range jiraKeyInText.FindAllString(text, -1) {
		k := strings.ToUpper(m)
		if !task.IsJiraKey(k) || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// normalizeKeys は API から受けた番号列を検証・正規化する。
func normalizeKeys(keys []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, k := range keys {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k == "" || seen[k] {
			continue
		}
		if !task.IsJiraKey(k) {
			return nil, apperr.Validation(fmt.Errorf("invalid jira key %q", k))
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, apperr.Validation(errors.New("jira keys are required"))
	}
	return out, nil
}

// snapshotOf は取得した issue をタスクの写しにする。
func (s *Service) snapshotOf(is port.JiraIssue) task.JiraSnapshot {
	return task.JiraSnapshot{
		Summary:        is.Summary,
		Status:         is.Status,
		StatusCategory: is.StatusCategory,
		Assignee:       is.Assignee,
		Priority:       is.Priority,
		Epic:           is.Epic,
		Sprint:         is.Sprint,
		Description:    is.Description,
		URL:            is.URL,
		FetchedAt:      s.now(),
	}
}

// existingByKey は取り込み済みタスクを番号で引く索引。
func (s *Service) existingByKey(ctx context.Context) (map[string]*task.Task, error) {
	all, err := s.d.Tasks.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	m := make(map[string]*task.Task, len(all))
	for _, t := range all {
		m[t.JiraKey] = t
	}
	return m, nil
}

// Preview は Jira から取得して保存せずに返す。templateID があれば展開した環境も付ける。
func (s *Service) Preview(ctx context.Context, keys []string, templateID string) ([]PreviewItem, error) {
	keys, err := normalizeKeys(keys)
	if err != nil {
		return nil, err
	}
	cli, err := s.jira(ctx)
	if err != nil {
		return nil, err
	}
	issues, err := cli.FetchIssues(ctx, keys)
	if err != nil {
		return nil, wrapJiraErr("fetch issues", err)
	}
	var tpl *task.Template
	if templateID != "" {
		tpl, err = s.d.Templates.FindByID(ctx, templateID)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", templateID, err)
		}
	}
	existing, err := s.existingByKey(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PreviewItem, 0, len(issues))
	for _, is := range issues {
		item := PreviewItem{Issue: is}
		if _, ok := existing[is.Key]; ok {
			item.Exists = true
			item.Warnings = append(item.Warnings, "既に取り込み済みです")
		}
		if tpl != nil {
			env := tpl.Expand(is.Key, is.Summary)
			item.Env = &env
		}
		out = append(out, item)
	}
	return out, nil
}

// Import は Jira から取得してタスクを保存する。setup=true なら続けてセットアップを開始する。
// 既に取り込み済みの番号があれば全体をエラーにする(部分取り込みはしない)。
func (s *Service) Import(ctx context.Context, keys []string, templateID string, setup bool) (ImportResult, error) {
	keys, err := normalizeKeys(keys)
	if err != nil {
		return ImportResult{}, err
	}
	existing, err := s.existingByKey(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	var dup []string
	for _, k := range keys {
		if _, ok := existing[k]; ok {
			dup = append(dup, k)
		}
	}
	if len(dup) > 0 {
		return ImportResult{}, apperr.Validation(fmt.Errorf("%w: %s", task.ErrDuplicateKey, strings.Join(dup, ", ")))
	}
	var tpl *task.Template
	if templateID != "" {
		tpl, err = s.d.Templates.FindByID(ctx, templateID)
		if err != nil {
			return ImportResult{}, fmt.Errorf("template %q: %w", templateID, err)
		}
	}
	if setup && tpl == nil {
		return ImportResult{}, apperr.Validation(errors.New("setup requires a template"))
	}
	cli, err := s.jira(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	issues, err := cli.FetchIssues(ctx, keys)
	if err != nil {
		return ImportResult{}, wrapJiraErr("fetch issues", err)
	}

	res := ImportResult{RunIDs: map[string]string{}}
	for _, is := range issues {
		t, err := task.NewTask(s.d.IDGen.NewID(), is.Key, s.snapshotOf(is), s.now())
		if err != nil {
			return res, apperr.Validation(fmt.Errorf("import %s: %w", is.Key, err))
		}
		if tpl != nil {
			if err := t.ApplyEnv(tpl.Expand(is.Key, is.Summary), s.now()); err != nil {
				return res, apperr.Validation(fmt.Errorf("import %s: %w", is.Key, err))
			}
		}
		if err := s.d.Tasks.Save(ctx, t); err != nil {
			return res, fmt.Errorf("save task %s: %w", is.Key, err)
		}
		dto, err := s.toDTO(ctx, t, nil, false)
		if err != nil {
			return res, err
		}
		res.Tasks = append(res.Tasks, dto)
		if setup {
			runID, err := s.StartSetup(ctx, t.ID)
			if err != nil {
				return res, err
			}
			res.RunIDs[is.Key] = runID
		}
	}
	return res, nil
}

// wsIndex はワークスペースの索引(ID → Workspace)。
func (s *Service) wsIndex(ctx context.Context) (map[string]*domain.Workspace, error) {
	all, err := s.d.Workspaces.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	m := make(map[string]*domain.Workspace, len(all))
	for _, w := range all {
		m[w.ID().String()] = w
	}
	return m, nil
}

// toDTO は導出状態を付けた DTO を作る。ws が nil なら索引を取り直す。
// checkDir=true なら作業フォルダの実在も検査する(詳細用)。
func (s *Service) toDTO(ctx context.Context, t *task.Task, ws map[string]*domain.Workspace, checkDir bool) (TaskDTO, error) {
	if ws == nil {
		var err error
		ws, err = s.wsIndex(ctx)
		if err != nil {
			return TaskDTO{}, err
		}
	}
	live := map[string]bool{}
	for _, id := range s.d.LivePaneIDs() {
		live[id] = true
	}
	lt, err := s.localSettings(ctx)
	if err != nil {
		return TaskDTO{}, err
	}
	return s.buildDTO(t, ws, live, lt, checkDir), nil
}

// localSettings はローカルタスクの採番・ステータス設定を返す。
func (s *Service) localSettings(ctx context.Context) (task.LocalTaskSettings, error) {
	st, err := s.d.Settings.Load(ctx)
	if err != nil {
		return task.LocalTaskSettings{}, fmt.Errorf("load settings: %w", err)
	}
	return st.Normalized().LocalTask, nil
}

func (s *Service) buildDTO(t *task.Task, ws map[string]*domain.Workspace, live map[string]bool, lt task.LocalTaskSettings, checkDir bool) TaskDTO {
	t.Source = t.Source.Normalize()
	resolveLocalStatus(t, lt)
	dto := TaskDTO{Task: t, EffectiveState: string(t.LocalState)}
	if t.WorkspaceID != "" {
		if w, ok := ws[t.WorkspaceID]; ok {
			dto.WorkspaceName = w.Name().String()
			for _, p := range w.Panes() {
				if live[p.ID().String()] {
					dto.Working = true
					break
				}
			}
		} else {
			dto.WorkspaceMissing = true
		}
	}
	if dto.Working && t.LocalState == task.StateReady {
		dto.EffectiveState = StateWorking
	}
	if checkDir && strings.TrimSpace(t.Env.WorkDir) != "" {
		if abs, err := s.absPath(t.Env.WorkDir); err == nil {
			if ok, err := s.d.Exec.Exists(abs); err == nil {
				dto.WorkDirExists = ok
			}
		}
	}
	return dto
}

// List は全タスクを、作業中 → 準備中 → 環境あり → 未準備 → 完了、同順位は更新日時の
// 新しい順で返す。
func (s *Service) List(ctx context.Context) ([]TaskDTO, error) {
	all, err := s.d.Tasks.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	ws, err := s.wsIndex(ctx)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, id := range s.d.LivePaneIDs() {
		live[id] = true
	}
	lt, err := s.localSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TaskDTO, 0, len(all))
	for _, t := range all {
		out = append(out, s.buildDTO(t, ws, live, lt, false))
	}
	rank := map[string]int{StateWorking: 0, string(task.StatePreparing): 1, string(task.StateReady): 2, string(task.StateNone): 3, string(task.StateDone): 4}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank[out[i].EffectiveState], rank[out[j].EffectiveState]
		if ri != rj {
			return ri < rj
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].JiraKey < out[j].JiraKey
	})
	return out, nil
}

// Get は 1 件を返す(作業フォルダの実在検査つき)。
func (s *Service) Get(ctx context.Context, id string) (TaskDTO, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	return s.toDTO(ctx, t, nil, true)
}

// FindByKey は Jira 番号で 1 件を返す。
func (s *Service) FindByKey(ctx context.Context, key string) (TaskDTO, error) {
	existing, err := s.existingByKey(ctx)
	if err != nil {
		return TaskDTO{}, err
	}
	t, ok := existing[strings.ToUpper(strings.TrimSpace(key))]
	if !ok {
		return TaskDTO{}, task.ErrNotFound
	}
	return s.toDTO(ctx, t, nil, true)
}

// UpdateInput は PATCH の入力。nil のフィールドは変更しない。
type UpdateInput struct {
	Note *string
	Env  *task.Env
	// LocalState は "done" か "ready"(完了の取り消し)のみ受け付ける。
	LocalState *string
	// Local はローカルタスクの概要・説明・優先度・ラベル・リンク・ステータス(local のみ)。
	Local *task.LocalInput
}

// Update はメモ・環境・完了状態を更新する。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (TaskDTO, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	now := s.now()
	if in.Note != nil {
		t.SetNote(*in.Note, now)
	}
	if in.Local != nil {
		lt, err := s.localSettings(ctx)
		if err != nil {
			return TaskDTO{}, err
		}
		if in.Local.StatusID != "" && lt.Status(in.Local.StatusID).ID != in.Local.StatusID {
			return TaskDTO{}, apperr.Validation(fmt.Errorf("unknown status %q", in.Local.StatusID))
		}
		if err := t.UpdateLocal(*in.Local, now); err != nil {
			return TaskDTO{}, apperr.Validation(err)
		}
	}
	if in.Env != nil {
		if t.LocalState == task.StatePreparing {
			return TaskDTO{}, apperr.Validation(errors.New("cannot edit env while setup is running"))
		}
		if err := s.validateEnvRefs(ctx, *in.Env); err != nil {
			return TaskDTO{}, err
		}
		if err := t.ApplyEnv(*in.Env, now); err != nil {
			return TaskDTO{}, apperr.Validation(err)
		}
	}
	if in.LocalState != nil {
		switch task.LocalState(*in.LocalState) {
		case task.StateDone:
			if err := t.MarkDone(now); err != nil {
				return TaskDTO{}, apperr.Validation(err)
			}
		case task.StateReady, task.StateNone:
			has := false
			if strings.TrimSpace(t.Env.WorkDir) != "" {
				if abs, err := s.absPath(t.Env.WorkDir); err == nil {
					has, _ = s.d.Exec.Exists(abs)
				}
			}
			t.Reopen(has, now)
		default:
			return TaskDTO{}, apperr.Validation(fmt.Errorf("localState must be done or ready, got %q", *in.LocalState))
		}
	}
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("save task: %w", err)
	}
	return s.toDTO(ctx, t, nil, true)
}

// validateEnvRefs は環境が参照するベースクローン・レイアウトの存在を検査する。
func (s *Service) validateEnvRefs(ctx context.Context, env task.Env) error {
	if env.IsEmpty() {
		return nil
	}
	if env.Layout != "" && !domain.LayoutPreset(env.Layout).IsValid() {
		return apperr.Validation(fmt.Errorf("invalid layout %q", env.Layout))
	}
	if env.Layout != "" && len(env.Panes) > domain.LayoutPreset(env.Layout).Capacity() {
		return apperr.Validation(fmt.Errorf("layout %q holds %d panes, got %d", env.Layout, domain.LayoutPreset(env.Layout).Capacity(), len(env.Panes)))
	}
	for _, r := range env.Repos {
		if r.Source.Kind == task.SourceBaseCopy {
			if _, err := s.d.BaseClones.FindByID(ctx, r.Source.BaseCloneID); err != nil {
				if errors.Is(err, task.ErrNotFound) {
					return apperr.Validation(fmt.Errorf("repo %q: base clone %q not found", r.Name, r.Source.BaseCloneID))
				}
				return err
			}
		}
	}
	return nil
}

// Delete はタスクを削除する。作業フォルダとワークスペースは残す(ワークスペースの紐付けは外す)。
func (s *Service) Delete(ctx context.Context, id string) error {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if t.LocalState == task.StatePreparing {
		return apperr.Validation(errors.New("cannot delete while setup is running"))
	}
	if t.WorkspaceID != "" {
		if w, err := s.d.Workspaces.FindByID(ctx, mustWsID(t.WorkspaceID)); err == nil {
			w.LinkTask("")
			_ = s.d.Workspaces.Save(ctx, w)
		}
	}
	if t.SetupRunID != "" {
		_ = s.d.Runs.Delete(ctx, t.SetupRunID)
	}
	return s.d.Tasks.Delete(ctx, id)
}

// Sync は 1 件を Jira から再取得する。ローカルタスクは対象外。
func (s *Service) Sync(ctx context.Context, id string) (TaskDTO, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	if t.IsLocal() {
		return TaskDTO{}, apperr.Validation(errors.New("local task has nothing to sync"))
	}
	cli, err := s.jira(ctx)
	if err != nil {
		return TaskDTO{}, err
	}
	issues, err := cli.FetchIssues(ctx, []string{t.JiraKey})
	if err != nil {
		return TaskDTO{}, wrapJiraErr("fetch issue", err)
	}
	if len(issues) == 0 {
		return TaskDTO{}, wrapJiraErr("fetch issue", port.ErrJiraNotFound)
	}
	t.UpdateJira(s.snapshotOf(issues[0]), s.now())
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("save task: %w", err)
	}
	return s.toDTO(ctx, t, nil, true)
}

// jqlSearchMax は自動取り込み JQL の上限件数。
const jqlSearchMax = 200

// SyncAll は全タスクを再取得し、JQL が設定されていれば該当 issue を未準備で追加する。
// 1 件の失敗で全体を止めず、Errors に積む(設定不備は即エラー)。
func (s *Service) SyncAll(ctx context.Context) (SyncResult, error) {
	cli, err := s.jira(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	all, err := s.d.Tasks.List(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list tasks: %w", err)
	}
	res := SyncResult{Added: []string{}}
	byKey := map[string]*task.Task{}
	var keys []string
	for _, t := range all {
		byKey[t.JiraKey] = t
		if t.IsLocal() {
			continue // ローカルタスクは Jira に無い
		}
		keys = append(keys, t.JiraKey)
	}
	// 既存分はまとめて取得。無くなった issue があれば 1 件ずつに落として特定する。
	if len(keys) > 0 {
		issues, err := cli.FetchIssues(ctx, keys)
		if err != nil && errors.Is(err, port.ErrJiraNotFound) {
			issues = nil
			for _, k := range keys {
				one, e := cli.FetchIssues(ctx, []string{k})
				if e != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", k, e))
					continue
				}
				issues = append(issues, one...)
			}
		} else if err != nil {
			return res, wrapJiraErr("fetch issues", err)
		}
		for _, is := range issues {
			t, ok := byKey[is.Key]
			if !ok {
				continue
			}
			t.UpdateJira(s.snapshotOf(is), s.now())
			if err := s.d.Tasks.Save(ctx, t); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: save: %v", is.Key, err))
				continue
			}
			res.Updated++
		}
	}
	cfg, _, err := s.d.JiraStore.Load(ctx)
	if err != nil {
		return res, fmt.Errorf("load jira config: %w", err)
	}
	if strings.TrimSpace(cfg.JQL) != "" {
		found, err := cli.Search(ctx, cfg.JQL, jqlSearchMax)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("JQL: %v", err))
			return res, nil
		}
		for _, is := range found {
			if _, ok := byKey[is.Key]; ok {
				continue
			}
			t, err := task.NewTask(s.d.IDGen.NewID(), is.Key, s.snapshotOf(is), s.now())
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", is.Key, err))
				continue
			}
			if err := s.d.Tasks.Save(ctx, t); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: save: %v", is.Key, err))
				continue
			}
			byKey[is.Key] = t
			res.Added = append(res.Added, is.Key)
		}
	}
	return res, nil
}

// DeleteWorkDir は作業フォルダを削除し、紐付くワークスペースも削除する(明示操作専用)。
func (s *Service) DeleteWorkDir(ctx context.Context, id string) (TaskDTO, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	if t.LocalState == task.StatePreparing {
		return TaskDTO{}, apperr.Validation(errors.New("cannot delete work dir while setup is running"))
	}
	if strings.TrimSpace(t.Env.WorkDir) != "" {
		abs, err := s.absPath(t.Env.WorkDir)
		if err != nil {
			return TaskDTO{}, apperr.Validation(fmt.Errorf("work dir: %w", err))
		}
		if err := s.d.Exec.RemoveAll(abs); err != nil {
			return TaskDTO{}, fmt.Errorf("remove work dir: %w", err)
		}
	}
	if t.WorkspaceID != "" {
		if s.d.DeleteWorkspace != nil {
			if err := s.d.DeleteWorkspace(ctx, t.WorkspaceID); err != nil && !errors.Is(err, domain.ErrWorkspaceNotFound) {
				return TaskDTO{}, fmt.Errorf("delete workspace: %w", err)
			}
		}
		t.UnlinkWorkspace(s.now())
	}
	t.ClearWorkDir(s.now())
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("save task: %w", err)
	}
	return s.toDTO(ctx, t, nil, true)
}

// UnlinkWorkspace はワークスペースが削除されたときに紐付けを外す(HTTP 層の削除処理から呼ぶ)。
// 該当タスクが無ければ何もしない。
func (s *Service) UnlinkWorkspace(ctx context.Context, workspaceID string) error {
	all, err := s.d.Tasks.List(ctx)
	if err != nil {
		return err
	}
	for _, t := range all {
		if t.WorkspaceID != workspaceID {
			continue
		}
		t.UnlinkWorkspace(s.now())
		if err := s.d.Tasks.Save(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// mustWsID は文字列から WorkspaceId を作る(空でないことは呼び出し側が保証)。
func mustWsID(id string) domain.WorkspaceId {
	wid, _ := domain.NewWorkspaceId(id)
	return wid
}
