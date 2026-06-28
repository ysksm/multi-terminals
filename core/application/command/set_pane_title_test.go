package command_test

import (
	"context"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestSetPaneTitleHandler(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// create workspace
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// add pane
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// set pane title
	h := command.NewSetPaneTitleHandler(repo)
	err = h.Handle(ctx, command.SetPaneTitleCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Title:       "web server",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// verify via repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got := w.Panes()[0].Title().String(); got != "web server" {
		t.Fatalf("title not persisted: got %q", got)
	}

	// 不正なワークスペース id は検証エラー
	if err := h.Handle(ctx, command.SetPaneTitleCommand{WorkspaceID: "", PaneID: paneResult.PaneID, Title: "x"}); err == nil {
		t.Fatal("invalid workspace id: expected error")
	}
}
