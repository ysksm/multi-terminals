package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// TemplateInput はテンプレートの作成/更新の入力。
type TemplateInput struct {
	Name           string          `json:"name"`
	IsDefault      bool            `json:"isDefault"`
	WorkDirPattern string          `json:"workDirPattern"`
	BranchPattern  string          `json:"branchPattern"`
	Layout         string          `json:"layout"`
	Repos          []task.Repo     `json:"repos"`
	Panes          []task.PaneSpec `json:"panes"`
}

// ListTemplates は既定を先頭に、名前順で返す。1 件も無ければ既定テンプレートを作って返す。
func (s *Service) ListTemplates(ctx context.Context) ([]*task.Template, error) {
	all, err := s.d.Templates.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	if len(all) == 0 {
		t := task.DefaultTemplate(s.d.IDGen.NewID(), s.now())
		if err := s.d.Templates.Save(ctx, t); err != nil {
			return nil, fmt.Errorf("save default template: %w", err)
		}
		all = []*task.Template{t}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].IsDefault != all[j].IsDefault {
			return all[i].IsDefault
		}
		return all[i].Name < all[j].Name
	})
	return all, nil
}

// GetTemplate は 1 件を返す。
func (s *Service) GetTemplate(ctx context.Context, id string) (*task.Template, error) {
	return s.d.Templates.FindByID(ctx, id)
}

// applyTemplateInput は入力を検証してテンプレートへ反映する。
func (s *Service) applyTemplateInput(ctx context.Context, t *task.Template, in TemplateInput) error {
	t.Name = strings.TrimSpace(in.Name)
	t.IsDefault = in.IsDefault
	t.WorkDirPattern = strings.TrimSpace(in.WorkDirPattern)
	t.BranchPattern = strings.TrimSpace(in.BranchPattern)
	t.Layout = strings.TrimSpace(in.Layout)
	t.Repos = in.Repos
	if t.Repos == nil {
		t.Repos = []task.Repo{}
	}
	t.Panes = in.Panes
	if t.Panes == nil {
		t.Panes = []task.PaneSpec{}
	}
	t.UpdatedAt = s.now()
	if err := t.Validate(); err != nil {
		return apperr.Validation(err)
	}
	lp := domain.LayoutPreset(t.Layout)
	if !lp.IsValid() {
		return apperr.Validation(fmt.Errorf("invalid layout %q", t.Layout))
	}
	if len(t.Panes) > lp.Capacity() {
		return apperr.Validation(fmt.Errorf("layout %q holds %d panes, got %d", t.Layout, lp.Capacity(), len(t.Panes)))
	}
	for _, p := range t.Panes {
		if p.Slot >= lp.Capacity() {
			return apperr.Validation(fmt.Errorf("pane slot %d out of range for layout %q", p.Slot, t.Layout))
		}
	}
	for _, r := range t.Repos {
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

// clearOtherDefaults は t 以外の既定フラグを落とす(既定は 1 件だけ)。
func (s *Service) clearOtherDefaults(ctx context.Context, keepID string) error {
	all, err := s.d.Templates.List(ctx)
	if err != nil {
		return err
	}
	for _, o := range all {
		if o.ID != keepID && o.IsDefault {
			o.IsDefault = false
			o.UpdatedAt = s.now()
			if err := s.d.Templates.Save(ctx, o); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateTemplate は新規テンプレートを保存する。
func (s *Service) CreateTemplate(ctx context.Context, in TemplateInput) (*task.Template, error) {
	t := &task.Template{ID: s.d.IDGen.NewID(), CreatedAt: s.now()}
	if err := s.applyTemplateInput(ctx, t, in); err != nil {
		return nil, err
	}
	if err := s.d.Templates.Save(ctx, t); err != nil {
		return nil, fmt.Errorf("save template: %w", err)
	}
	if t.IsDefault {
		if err := s.clearOtherDefaults(ctx, t.ID); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// UpdateTemplate は既存テンプレートを更新する。
func (s *Service) UpdateTemplate(ctx context.Context, id string, in TemplateInput) (*task.Template, error) {
	t, err := s.d.Templates.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.applyTemplateInput(ctx, t, in); err != nil {
		return nil, err
	}
	if err := s.d.Templates.Save(ctx, t); err != nil {
		return nil, fmt.Errorf("save template: %w", err)
	}
	if t.IsDefault {
		if err := s.clearOtherDefaults(ctx, t.ID); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// DeleteTemplate はテンプレートを削除する。タスク側は展開済みの環境を持つので影響しない。
func (s *Service) DeleteTemplate(ctx context.Context, id string) error {
	return s.d.Templates.Delete(ctx, id)
}
