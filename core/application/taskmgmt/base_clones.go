package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// BaseCloneInput はベースクローン登録の入力。Path が空なら BaseDir/base/<name>。
type BaseCloneInput struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Path string `json:"path"`
}

// ListBaseClones は名前順で返す。
func (s *Service) ListBaseClones(ctx context.Context) ([]*task.BaseClone, error) {
	all, err := s.d.BaseClones.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list base clones: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all, nil
}

// CreateBaseClone は clone して登録する。clone は同期で行う(大きなリポジトリは時間がかかる)。
func (s *Service) CreateBaseClone(ctx context.Context, in BaseCloneInput) (*task.BaseClone, error) {
	name := strings.TrimSpace(in.Name)
	path := strings.TrimSpace(in.Path)
	if path == "" {
		if s.d.BaseDir == "" {
			return nil, apperr.Validation(errors.New("path is required"))
		}
		path = filepath.Join(s.d.BaseDir, "base", name)
	}
	abs, err := s.absPath(path)
	if err != nil {
		return nil, apperr.Validation(fmt.Errorf("path: %w", err))
	}
	bc := &task.BaseClone{
		ID:        s.d.IDGen.NewID(),
		Name:      name,
		URL:       strings.TrimSpace(in.URL),
		Path:      abs,
		CreatedAt: s.now(),
	}
	if err := bc.Validate(); err != nil {
		return nil, apperr.Validation(err)
	}
	all, err := s.d.BaseClones.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range all {
		if o.Name == bc.Name {
			return nil, apperr.Validation(fmt.Errorf("base clone %q already exists", bc.Name))
		}
	}
	if err := s.d.Exec.MkdirAll(filepath.Dir(abs)); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	if _, err := s.d.Git.Clone(bc.URL, abs); err != nil {
		return nil, apperr.Validation(fmt.Errorf("clone: %w", err))
	}
	s.refreshBaseClone(bc)
	if err := s.d.BaseClones.Save(ctx, bc); err != nil {
		return nil, fmt.Errorf("save base clone: %w", err)
	}
	return bc, nil
}

// refreshBaseClone は既定ブランチとサイズを取り直して記録する。
func (s *Service) refreshBaseClone(bc *task.BaseClone) {
	def, err := s.d.Git.DefaultBranch(bc.Path)
	if err != nil {
		def = ""
	}
	size, err := s.d.Exec.DirSize(bc.Path)
	if err != nil {
		size = bc.SizeBytes
	}
	bc.MarkFetched(def, size, s.now())
}

// updateBaseClone は fetch → 既定ブランチへ切替 → pull し、記録を更新する。
func (s *Service) updateBaseClone(ctx context.Context, bc *task.BaseClone) error {
	if err := s.d.Git.Fetch(bc.Path); err != nil {
		return err
	}
	def, err := s.d.Git.DefaultBranch(bc.Path)
	if err != nil {
		return err
	}
	if err := s.d.Git.Checkout(bc.Path, def); err != nil {
		return err
	}
	if err := s.d.Git.Pull(bc.Path); err != nil {
		return err
	}
	s.refreshBaseClone(bc)
	return s.d.BaseClones.Save(ctx, bc)
}

// UpdateBaseClone は 1 件を最新化する。
func (s *Service) UpdateBaseClone(ctx context.Context, id string) (*task.BaseClone, error) {
	bc, err := s.d.BaseClones.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.updateBaseClone(ctx, bc); err != nil {
		return nil, apperr.Validation(fmt.Errorf("update %s: %w", bc.Name, err))
	}
	return bc, nil
}

// UpdateAllBaseClones は全件を最新化する。失敗は名前ごとに集めて返す(他は続行)。
func (s *Service) UpdateAllBaseClones(ctx context.Context) ([]*task.BaseClone, []string, error) {
	all, err := s.ListBaseClones(ctx)
	if err != nil {
		return nil, nil, err
	}
	var errs []string
	for _, bc := range all {
		if err := s.updateBaseClone(ctx, bc); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", bc.Name, err))
		}
	}
	return all, errs, nil
}

// DeleteBaseClone は登録を削除する。removeFiles=true なら clone 本体も消す。
// テンプレートやタスクから参照されていれば削除しない。
func (s *Service) DeleteBaseClone(ctx context.Context, id string, removeFiles bool) error {
	bc, err := s.d.BaseClones.FindByID(ctx, id)
	if err != nil {
		return err
	}
	tpls, err := s.d.Templates.List(ctx)
	if err != nil {
		return err
	}
	for _, t := range tpls {
		for _, r := range t.Repos {
			if r.Source.Kind == task.SourceBaseCopy && r.Source.BaseCloneID == id {
				return apperr.Validation(fmt.Errorf("base clone %q is used by template %q", bc.Name, t.Name))
			}
		}
	}
	if removeFiles {
		if err := s.d.Exec.RemoveAll(bc.Path); err != nil {
			return fmt.Errorf("remove %s: %w", bc.Path, err)
		}
	}
	return s.d.BaseClones.Delete(ctx, id)
}
