package taskmgmt

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

func TestCreateBaseClone(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	e.git.DefaultBranches["/data/base/frontend"] = "develop"
	e.exec.Sizes["/data/base/frontend"] = 1234

	bc, err := e.svc.CreateBaseClone(ctx, BaseCloneInput{Name: " frontend ", URL: " git@x:fe.git "})
	if err != nil {
		t.Fatal(err)
	}
	if bc.Path != "/data/base/frontend" || bc.Name != "frontend" || bc.URL != "git@x:fe.git" {
		t.Errorf("base clone: %+v", bc)
	}
	if bc.DefaultBranch != "develop" || bc.SizeBytes != 1234 || !bc.LastFetchedAt.Equal(testNow) {
		t.Errorf("refresh: %+v", bc)
	}
	if !reflect.DeepEqual(e.git.Clones, []apptest.CloneCall{{URL: "git@x:fe.git", Dest: "/data/base/frontend"}}) {
		t.Errorf("clones: %v", e.git.Clones)
	}
	if !reflect.DeepEqual(e.exec.Mkdirs, []string{"/data/base"}) {
		t.Errorf("mkdirs: %v", e.exec.Mkdirs)
	}
	if saved, err := e.baseClones.FindByID(ctx, bc.ID); err != nil || saved.Path != bc.Path {
		t.Errorf("not saved: %v", err)
	}
	// 重複名
	if _, err := e.svc.CreateBaseClone(ctx, BaseCloneInput{Name: "frontend", URL: "u"}); !isValidation(err) {
		t.Errorf("duplicate: %v", err)
	}
	// 明示パス(~ 展開)
	bc2, err := e.svc.CreateBaseClone(ctx, BaseCloneInput{Name: "backend", URL: "u", Path: "~/bases/be"})
	if err != nil || bc2.Path != "/home/test/bases/be" {
		t.Errorf("explicit path: %v %+v", err, bc2)
	}
	// 検証エラー
	for name, in := range map[string]BaseCloneInput{
		"bad name": {Name: "a/b", URL: "u"},
		"no url":   {Name: "ok", URL: ""},
	} {
		if _, err := e.svc.CreateBaseClone(ctx, in); !isValidation(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// clone 失敗
	e.git.CloneErr = errors.New("auth")
	if _, err := e.svc.CreateBaseClone(ctx, BaseCloneInput{Name: "third", URL: "u"}); !isValidation(err) {
		t.Errorf("clone error: %v", err)
	}
	all, _ := e.baseClones.List(ctx)
	if len(all) != 2 {
		t.Errorf("saved after clone error: %d", len(all))
	}
}

func TestCreateBaseClone_NoBaseDir(t *testing.T) {
	e := newTestService(t)
	e.svc.d.BaseDir = ""
	if _, err := e.svc.CreateBaseClone(context.Background(), BaseCloneInput{Name: "x", URL: "u"}); !isValidation(err) {
		t.Errorf("path required: %v", err)
	}
}

func TestListBaseClones_Order(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "1", Name: "zeta", URL: "u", Path: "/z"})
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "2", Name: "alpha", URL: "u", Path: "/a"})
	list, err := e.svc.ListBaseClones(ctx)
	if err != nil || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Errorf("order: %v %+v", err, list)
	}
}

func TestUpdateBaseClone(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	bc := &task.BaseClone{ID: "b1", Name: "fe", URL: "u", Path: "/base/fe", DefaultBranch: "main", SizeBytes: 1}
	_ = e.baseClones.Save(ctx, bc)
	e.git.DefaultBranches["/base/fe"] = "main"
	e.exec.Sizes["/base/fe"] = 99
	e.now = testNow.Add(time.Hour)

	got, err := e.svc.UpdateBaseClone(ctx, "b1")
	if err != nil {
		t.Fatal(err)
	}
	wantOps := []apptest.GitOpCall{{Dir: "/base/fe", Op: "fetch"}, {Dir: "/base/fe", Op: "pull"}}
	if !reflect.DeepEqual(e.git.GitOps, wantOps) {
		t.Errorf("ops: %v", e.git.GitOps)
	}
	if !reflect.DeepEqual(e.git.Checkouts, []apptest.CheckoutCall{{Dir: "/base/fe", Branch: "main"}}) {
		t.Errorf("checkouts: %v", e.git.Checkouts)
	}
	if got.SizeBytes != 99 || !got.LastFetchedAt.Equal(e.now) {
		t.Errorf("record: %+v", got)
	}
	e.git.OpErr = errors.New("network")
	if _, err := e.svc.UpdateBaseClone(ctx, "b1"); !isValidation(err) {
		t.Errorf("fetch error: %v", err)
	}
	if _, err := e.svc.UpdateBaseClone(ctx, "nope"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func TestUpdateAllBaseClones(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "1", Name: "a", URL: "u", Path: "/a"})
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "2", Name: "b", URL: "u", Path: "/b"})
	e.git.DefaultBranchErr = errors.New("no head")
	all, errs, err := e.svc.UpdateAllBaseClones(ctx)
	if err != nil || len(all) != 2 || len(errs) != 2 {
		t.Errorf("all=%d errs=%v err=%v", len(all), errs, err)
	}
	e.git.DefaultBranchErr = nil
	_, errs, err = e.svc.UpdateAllBaseClones(ctx)
	if err != nil || len(errs) != 0 {
		t.Errorf("errs=%v err=%v", errs, err)
	}
}

func TestDeleteBaseClone(t *testing.T) {
	e := newTestService(t)
	ctx := context.Background()
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "b1", Name: "fe", URL: "u", Path: "/base/fe"})
	_ = e.templates.Save(ctx, &task.Template{ID: "t1", Name: "web", WorkDirPattern: "{KEY}", BranchPattern: "b", Layout: "single",
		Repos: []task.Repo{{Name: "fe", Source: task.RepoSource{Kind: task.SourceBaseCopy, BaseCloneID: "b1"}}}})

	if err := e.svc.DeleteBaseClone(ctx, "b1", true); !isValidation(err) {
		t.Errorf("in use: %v", err)
	}
	if len(e.exec.Removed) != 0 {
		t.Error("must not remove files when refused")
	}
	_ = e.templates.Delete(ctx, "t1")
	if err := e.svc.DeleteBaseClone(ctx, "b1", false); err != nil {
		t.Fatal(err)
	}
	if len(e.exec.Removed) != 0 {
		t.Error("removeFiles=false must keep files")
	}
	if _, err := e.baseClones.FindByID(ctx, "b1"); !errors.Is(err, task.ErrNotFound) {
		t.Error("not deleted")
	}
	_ = e.baseClones.Save(ctx, &task.BaseClone{ID: "b2", Name: "be", URL: "u", Path: "/base/be"})
	if err := e.svc.DeleteBaseClone(ctx, "b2", true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.exec.Removed, []string{"/base/be"}) {
		t.Errorf("removed: %v", e.exec.Removed)
	}
	if err := e.svc.DeleteBaseClone(ctx, "nope", false); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}
