package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// LocalCreateInput は手動作成の入力。Key を空にすると自動採番する。
type LocalCreateInput struct {
	task.LocalInput
	Key        string `json:"key"`
	TemplateID string `json:"templateId"`
	Setup      bool   `json:"setup"`
}

// LocalCreateResult は作成結果。Setup=true なら RunID を返す。
type LocalCreateResult struct {
	Task  TaskDTO `json:"task"`
	RunID string  `json:"runId,omitempty"`
}

// NextLocalKey は次に採番される番号を返す(採番は進めない)。作成フォームの表示用。
func (s *Service) NextLocalKey(ctx context.Context) (string, error) {
	st, err := s.d.Settings.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("load settings: %w", err)
	}
	return st.Normalized().LocalTask.NextKey(), nil
}

// allocateLocalKey は番号を決めて採番を進める。key が空なら Prefix-NextSeq、指定があれば
// 形式と一意性を検査し、Prefix-N 形式で N >= NextSeq なら NextSeq を N+1 に進める。
func (s *Service) allocateLocalKey(ctx context.Context, st *task.Settings, key string, existing map[string]*task.Task) (string, error) {
	lt := &st.LocalTask
	key = strings.ToUpper(strings.TrimSpace(key))
	if key == "" {
		// 既存と衝突しない番号まで進める(手入力で先の番号が使われていた場合)
		for {
			key = lt.NextKey()
			if _, ok := existing[key]; !ok {
				break
			}
			lt.NextSeq++
		}
		lt.NextSeq++
		return key, nil
	}
	if !task.IsJiraKey(key) {
		return "", apperr.Validation(fmt.Errorf("invalid key %q (expected PREFIX-123)", key))
	}
	if _, ok := existing[key]; ok {
		return "", apperr.Validation(fmt.Errorf("%w: %s", task.ErrDuplicateKey, key))
	}
	if n, ok := lt.SeqOfKey(key); ok && n >= lt.NextSeq {
		lt.NextSeq = n + 1
	}
	return key, nil
}

// CreateLocal は手動でタスクを作成する。setup=true ならテンプレート必須で、続けてセットアップを始める。
func (s *Service) CreateLocal(ctx context.Context, in LocalCreateInput) (LocalCreateResult, error) {
	st, err := s.d.Settings.Load(ctx)
	if err != nil {
		return LocalCreateResult{}, fmt.Errorf("load settings: %w", err)
	}
	st = st.Normalized()
	if in.StatusID == "" && len(st.LocalTask.Statuses) > 0 {
		in.StatusID = st.LocalTask.Statuses[0].ID
	}
	if st.LocalTask.Status(in.StatusID).ID != in.StatusID {
		return LocalCreateResult{}, apperr.Validation(fmt.Errorf("unknown status %q", in.StatusID))
	}
	var tpl *task.Template
	if in.TemplateID != "" {
		tpl, err = s.d.Templates.FindByID(ctx, in.TemplateID)
		if err != nil {
			return LocalCreateResult{}, fmt.Errorf("template %q: %w", in.TemplateID, err)
		}
	}
	if in.Setup && tpl == nil {
		return LocalCreateResult{}, apperr.Validation(errors.New("setup requires a template"))
	}
	existing, err := s.existingByKey(ctx)
	if err != nil {
		return LocalCreateResult{}, err
	}
	key, err := s.allocateLocalKey(ctx, &st, in.Key, existing)
	if err != nil {
		return LocalCreateResult{}, err
	}
	t, err := task.NewLocalTask(s.d.IDGen.NewID(), key, in.LocalInput, s.now())
	if err != nil {
		return LocalCreateResult{}, apperr.Validation(err)
	}
	if tpl != nil {
		if err := t.ApplyEnv(tpl.Expand(key, t.Jira.Summary), s.now()); err != nil {
			return LocalCreateResult{}, apperr.Validation(err)
		}
	}
	// 採番を先に確定させてからタスクを保存する(同じ番号の二重発行を避ける)
	if err := s.d.Settings.Save(ctx, st); err != nil {
		return LocalCreateResult{}, fmt.Errorf("save settings: %w", err)
	}
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return LocalCreateResult{}, fmt.Errorf("save task: %w", err)
	}
	res := LocalCreateResult{}
	if in.Setup {
		runID, err := s.StartSetup(ctx, t.ID)
		if err != nil {
			return res, err
		}
		res.RunID = runID
	}
	dto, err := s.Get(ctx, t.ID)
	if err != nil {
		return res, err
	}
	res.Task = dto
	return res, nil
}

// LinkJira はローカルタスクに Jira 番号を紐付け、Jira タスクへ切り替える。
func (s *Service) LinkJira(ctx context.Context, id, key string) (TaskDTO, error) {
	t, err := s.d.Tasks.FindByID(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	if !t.IsLocal() {
		return TaskDTO{}, apperr.Validation(errors.New("task is already a jira task"))
	}
	keys, err := normalizeKeys([]string{key})
	if err != nil {
		return TaskDTO{}, err
	}
	existing, err := s.existingByKey(ctx)
	if err != nil {
		return TaskDTO{}, err
	}
	if other, ok := existing[keys[0]]; ok && other.ID != t.ID {
		return TaskDTO{}, apperr.Validation(fmt.Errorf("%w: %s", task.ErrDuplicateKey, keys[0]))
	}
	cli, err := s.jira(ctx)
	if err != nil {
		return TaskDTO{}, err
	}
	issues, err := cli.FetchIssues(ctx, keys)
	if err != nil {
		return TaskDTO{}, wrapJiraErr("fetch issue", err)
	}
	if len(issues) == 0 {
		return TaskDTO{}, wrapJiraErr("fetch issue", port.ErrJiraNotFound)
	}
	if err := t.LinkJira(issues[0].Key, s.snapshotOf(issues[0]), s.now()); err != nil {
		return TaskDTO{}, apperr.Validation(err)
	}
	if err := s.d.Tasks.Save(ctx, t); err != nil {
		return TaskDTO{}, fmt.Errorf("save task: %w", err)
	}
	return s.toDTO(ctx, t, nil, true)
}

// resolveLocalStatus はローカルタスクの表示用ステータス名と分類を設定から埋める。
func resolveLocalStatus(t *task.Task, lt task.LocalTaskSettings) {
	if !t.IsLocal() || t.Local == nil {
		return
	}
	st := lt.Status(t.Local.StatusID)
	t.Jira.Status = st.Name
	t.Jira.StatusCategory = string(st.Category)
}

// validateLocalTaskSettings は設定保存時の検査と、新規ステータスの ID 付与を行う。
func (s *Service) validateLocalTaskSettings(lt *task.LocalTaskSettings) error {
	lt.Prefix = strings.ToUpper(strings.TrimSpace(lt.Prefix))
	if lt.NextSeq < 1 {
		lt.NextSeq = 1
	}
	for i := range lt.Statuses {
		lt.Statuses[i].Name = strings.TrimSpace(lt.Statuses[i].Name)
		lt.Statuses[i].ID = strings.TrimSpace(lt.Statuses[i].ID)
		if lt.Statuses[i].ID == "" {
			lt.Statuses[i].ID = s.d.IDGen.NewID()
		}
		if lt.Statuses[i].Category == "" {
			lt.Statuses[i].Category = task.CategoryNew
		}
	}
	if err := lt.Validate(); err != nil {
		return apperr.Validation(err)
	}
	return nil
}
