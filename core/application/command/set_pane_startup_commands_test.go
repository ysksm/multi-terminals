package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestSetPaneStartupCommandsHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// create workspace
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// add pane with no commands
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// set startup commands
	handler := command.NewSetPaneStartupCommandsHandler(repo)
	err = handler.Handle(ctx, command.SetPaneStartupCommandsCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Commands: []command.StartupCommandInput{
			{Command: "echo hello", AutoRun: true},
			{Command: "ls -la", AutoRun: false},
		},
	})
	if err != nil {
		t.Fatalf("set pane startup commands: %v", err)
	}

	// verify via repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	panes := w.Panes()
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
	cmds := panes[0].Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0].Command() != "echo hello" || !cmds[0].AutoRun() {
		t.Errorf("command[0] mismatch: got %q autoRun=%v", cmds[0].Command(), cmds[0].AutoRun())
	}
	if cmds[1].Command() != "ls -la" || cmds[1].AutoRun() {
		t.Errorf("command[1] mismatch: got %q autoRun=%v", cmds[1].Command(), cmds[1].AutoRun())
	}
}

func TestSetPaneStartupCommandsHandler_Handle_EmptyCommands(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// create workspace
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// add pane with a command
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
		Commands:    []command.StartupCommandInput{{Command: "echo hi", AutoRun: true}},
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// clear commands with empty slice
	handler := command.NewSetPaneStartupCommandsHandler(repo)
	err = handler.Handle(ctx, command.SetPaneStartupCommandsCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Commands:    []command.StartupCommandInput{},
	})
	if err != nil {
		t.Fatalf("set pane startup commands (empty): %v", err)
	}

	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	cmds := w.Panes()[0].Commands()
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands after clear, got %d", len(cmds))
	}
}

func TestSetPaneStartupCommandsHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewSetPaneStartupCommandsHandler(repo)
	err := handler.Handle(ctx, command.SetPaneStartupCommandsCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
		Commands:    []command.StartupCommandInput{{Command: "echo hi", AutoRun: false}},
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestSetPaneStartupCommandsHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	handler := command.NewSetPaneStartupCommandsHandler(repo)
	err = handler.Handle(ctx, command.SetPaneStartupCommandsCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      "no-such-pane",
		Commands:    []command.StartupCommandInput{{Command: "echo hi", AutoRun: false}},
	})
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}

func TestSetPaneStartupCommandsHandler_Handle_InvalidCommand(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	handler := command.NewSetPaneStartupCommandsHandler(repo)
	err = handler.Handle(ctx, command.SetPaneStartupCommandsCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Commands:    []command.StartupCommandInput{{Command: "", AutoRun: false}}, // empty command
	})
	if err == nil {
		t.Fatal("expected error for empty command string, got nil")
	}
}
