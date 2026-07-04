package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
)

func TestCloneRepositoryHandler_Handle_Success(t *testing.T) {
	git := apptest.NewFakeGitService()
	h := command.NewCloneRepositoryHandler(git)

	result, err := h.Handle(context.Background(), command.CloneRepositoryCommand{
		URL:  "https://github.com/user/repo.git",
		Dest: "~/src/github/repo",
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Path != "~/src/github/repo" {
		t.Errorf("Path = %q", result.Path)
	}
	if len(git.Clones) != 1 || git.Clones[0].URL != "https://github.com/user/repo.git" {
		t.Errorf("Clones = %+v", git.Clones)
	}
}

func TestCloneRepositoryHandler_Handle_EmptyURL(t *testing.T) {
	h := command.NewCloneRepositoryHandler(apptest.NewFakeGitService())
	if _, err := h.Handle(context.Background(), command.CloneRepositoryCommand{URL: "  ", Dest: "/tmp/x"}); err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}

func TestCloneRepositoryHandler_Handle_EmptyDest(t *testing.T) {
	h := command.NewCloneRepositoryHandler(apptest.NewFakeGitService())
	if _, err := h.Handle(context.Background(), command.CloneRepositoryCommand{URL: "https://x/y.git", Dest: ""}); err == nil {
		t.Fatal("expected error for empty dest, got nil")
	}
}

func TestCloneRepositoryHandler_Handle_CloneError(t *testing.T) {
	git := apptest.NewFakeGitService()
	git.CloneErr = errors.New("dest exists")
	h := command.NewCloneRepositoryHandler(git)
	if _, err := h.Handle(context.Background(), command.CloneRepositoryCommand{URL: "https://x/y.git", Dest: "/tmp/x"}); err == nil {
		t.Fatal("expected clone error to propagate, got nil")
	}
}
