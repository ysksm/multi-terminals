package taskmgmt

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

func validTemplateInput() TemplateInput {
	return TemplateInput{
		Name: "backend only", WorkDirPattern: "~/work/{KEY}", BranchPattern: "feature/{KEY}-{slug}", Layout: "split_vertical",
		Repos: []task.Repo{{Name: "backend", Source: task.RepoSource{Kind: task.SourceClone, URL: "git@x:be.git"}}},
		Panes: []task.PaneSpec{{Slot: 0, RepoName: "backend"}, {Slot: 1}},
	}
}

func TestListTemplates_CreatesDefault(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	list, err := e.svc.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].IsDefault || list[0].Name != "標準" {
		t.Fatalf("default: %+v", list)
	}
	again, _ := e.svc.ListTemplates(ctx)
	if len(again) != 1 || again[0].ID != list[0].ID {
		t.Error("default should be created only once")
	}
}

func TestListTemplates_Order(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	in := validTemplateInput()
	in.Name = "zzz"
	if _, err := e.svc.CreateTemplate(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Name = "aaa"
	if _, err := e.svc.CreateTemplate(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Name = "mmm"
	in.IsDefault = true
	if _, err := e.svc.CreateTemplate(ctx, in); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.ListTemplates(ctx)
	if len(list) != 3 || list[0].Name != "mmm" || list[1].Name != "aaa" || list[2].Name != "zzz" {
		t.Errorf("order: %v", names(list))
	}
}

func names(list []*task.Template) []string {
	out := make([]string, len(list))
	for i, t := range list {
		out[i] = t.Name
	}
	return out
}

func TestCreateUpdateTemplate_Default(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	in := validTemplateInput()
	in.IsDefault = true
	a, err := e.svc.CreateTemplate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Name = "second"
	b, err := e.svc.CreateTemplate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = e.svc.GetTemplate(ctx, a.ID)
	if a.IsDefault || !b.IsDefault {
		t.Errorf("default should move: a=%v b=%v", a.IsDefault, b.IsDefault)
	}
	// Update で既定を戻す
	in.Name = "renamed"
	upd, err := e.svc.UpdateTemplate(ctx, a.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = e.svc.GetTemplate(ctx, b.ID)
	if upd.Name != "renamed" || !upd.IsDefault || b.IsDefault || upd.CreatedAt != a.CreatedAt {
		t.Errorf("update: %+v b=%v", upd, b.IsDefault)
	}
	if upd.Repos == nil || upd.Panes == nil {
		t.Error("nil slices should be normalized to empty")
	}
	if _, err := e.svc.UpdateTemplate(ctx, "nope", in); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := e.svc.DeleteTemplate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetTemplate(ctx, a.ID); !errors.Is(err, task.ErrNotFound) {
		t.Error("not deleted")
	}
	if err := e.svc.DeleteTemplate(ctx, a.ID); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("double delete: %v", err)
	}
}

func TestCreateTemplate_NilSlices(t *testing.T) {
	e := newTestService(t)
	in := TemplateInput{Name: "min", WorkDirPattern: "~/w/{KEY}", BranchPattern: "b/{KEY}", Layout: "single"}
	tp, err := e.svc.CreateTemplate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if tp.Repos == nil || tp.Panes == nil || len(tp.Repos) != 0 {
		t.Errorf("slices: %+v", tp)
	}
}

func TestCreateTemplate_Validation(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	mut := func(f func(*TemplateInput)) TemplateInput { in := validTemplateInput(); f(&in); return in }
	cases := map[string]TemplateInput{
		"empty name": mut(func(in *TemplateInput) { in.Name = "" }),
		"bad layout": mut(func(in *TemplateInput) { in.Layout = "triple" }),
		"too many":   mut(func(in *TemplateInput) { in.Layout = "single" }),
		"slot range": mut(func(in *TemplateInput) { in.Panes = []task.PaneSpec{{Slot: 5}} }),
		"unknown base": mut(func(in *TemplateInput) {
			in.Repos = []task.Repo{{Name: "fe", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "nope"}}}
			in.Panes = nil
		}),
		"workdir noKEY": mut(func(in *TemplateInput) { in.WorkDirPattern = "~/fixed" }),
		"unknown repo":  mut(func(in *TemplateInput) { in.Panes = []task.PaneSpec{{Slot: 0, RepoName: "x"}} }),
	}
	for name, in := range cases {
		if _, err := e.svc.CreateTemplate(ctx, in); !isValidation(err) {
			t.Errorf("%s: expected validation error, got %v", name, err)
		}
	}
	all, _ := e.templates.List(ctx)
	if len(all) != 0 {
		t.Errorf("invalid templates saved: %d", len(all))
	}
	// 登録済みベースクローンなら OK
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "b1", Name: "fe", URL: "u", Path: "/p"})
	in := mut(func(in *TemplateInput) {
		in.Repos = []task.Repo{{Name: "fe", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}}}
		in.Panes = []task.PaneSpec{{Slot: 0, RepoName: "fe"}}
	})
	if _, err := e.svc.CreateTemplate(ctx, in); err != nil {
		t.Errorf("valid base clone ref: %v", err)
	}
}
