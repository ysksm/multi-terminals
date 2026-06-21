package command_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/domain"
)

// createWorkspaceWithPanes saves a workspace with the given panes to the repo.
func createWorkspaceWithPanes(t *testing.T, repo *apptest.FakeRepo, wsID string, panes []*domain.Pane) {
	t.Helper()
	wid, err := domain.NewWorkspaceId(wsID)
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	name, err := domain.NewWorkspaceName("Test")
	if err != nil {
		t.Fatalf("NewWorkspaceName: %v", err)
	}
	w, err := domain.NewWorkspace(wid, name, "split_vertical")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	for _, p := range panes {
		if err := w.AddPane(p); err != nil {
			t.Fatalf("AddPane: %v", err)
		}
	}
	if err := repo.Save(context.Background(), w); err != nil {
		t.Fatalf("repo.Save: %v", err)
	}
}

func makePane(t *testing.T, id string, dir string, slot int, cmds []domain.StartupCommand) *domain.Pane {
	t.Helper()
	pid, err := domain.NewPaneId(id)
	if err != nil {
		t.Fatalf("NewPaneId: %v", err)
	}
	d, err := domain.NewDirectoryPath(dir)
	if err != nil {
		t.Fatalf("NewDirectoryPath: %v", err)
	}
	si, err := domain.NewSlotIndex(slot)
	if err != nil {
		t.Fatalf("NewSlotIndex: %v", err)
	}
	p, err := domain.NewPane(pid, d, si, cmds)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	return p
}

func makeStartupCmd(t *testing.T, cmd string, autoRun bool) domain.StartupCommand {
	t.Helper()
	c, err := domain.NewStartupCommand(cmd, autoRun)
	if err != nil {
		t.Fatalf("NewStartupCommand: %v", err)
	}
	return c
}

func TestOpenWorkspaceHandler_StartsSessionsForEachPane(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	// Create workspace with 2 panes (slots 0 and 1)
	pane0 := makePane(t, "pane-0", "/tmp", 0, nil)
	pane1 := makePane(t, "pane-1", "/tmp", 1, nil)
	createWorkspaceWithPanes(t, repo, "ws-1", []*domain.Pane{pane0, pane1})

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	result, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Both panes should be started and returned
	if len(result.Panes) != 2 {
		t.Errorf("expected 2 panes in result, got %d", len(result.Panes))
	}

	// Both sessions should be in the registry
	ids := reg.IDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 sessions in registry, got %d", len(ids))
	}

	// Runner should have been called twice (single-goroutine access after Handle returns).
	if len(runner.Started) != 2 {
		t.Errorf("expected 2 Start calls, got %d", len(runner.Started))
	}
}

func TestOpenWorkspaceHandler_AutoRunOnly(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	autoRunCmd := makeStartupCmd(t, "echo autorun", true)
	manualCmd := makeStartupCmd(t, "echo manual", false)

	pane0 := makePane(t, "pane-0", "/tmp", 0, []domain.StartupCommand{autoRunCmd, manualCmd})
	createWorkspaceWithPanes(t, repo, "ws-1", []*domain.Pane{pane0})

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	_, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Only autoRun commands should be written
	sess, ok := reg.Get("pane-0")
	if !ok {
		t.Fatal("session for pane-0 not found")
	}
	fakeSess := sess.(*apptest.FakeTerminalSession)
	// Access Writes after Handle returns — single-goroutine, no race.
	writes := fakeSess.Writes

	if len(writes) != 1 {
		t.Errorf("expected 1 Write (autoRun only), got %d", len(writes))
	}
	if len(writes) > 0 && string(writes[0]) != "echo autorun\n" {
		t.Errorf("expected autoRun command written, got %q", string(writes[0]))
	}
}

func TestOpenWorkspaceHandler_SetsLastOpened(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	pane0 := makePane(t, "pane-0", "/tmp", 0, nil)
	createWorkspaceWithPanes(t, repo, "ws-1", []*domain.Pane{pane0})

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	_, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// state should record the last opened workspace
	wsID, ok, err := state.Load(ctx)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if !ok {
		t.Fatal("expected last opened to be set")
	}
	if wsID != "ws-1" {
		t.Errorf("expected last opened to be 'ws-1', got %q", wsID)
	}
}

func TestOpenWorkspaceHandler_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	_, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "nonexistent"})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestOpenWorkspaceHandler_ClosesExistingSession(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	// Pre-register an existing session for pane-0
	existing := apptest.NewFakeTerminalSession("pane-0")
	reg.Add("pane-0", existing)

	pane0 := makePane(t, "pane-0", "/tmp", 0, nil)
	createWorkspaceWithPanes(t, repo, "ws-1", []*domain.Pane{pane0})

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	_, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// The existing session should have been closed (Done channel should be closed)
	select {
	case <-existing.Done():
		// ok — was closed
	default:
		t.Error("expected existing session to be closed before replacing")
	}

	// New session should be in registry
	newSess, ok := reg.Get("pane-0")
	if !ok {
		t.Fatal("expected new session in registry")
	}
	if newSess == existing {
		t.Error("expected a new session, not the old one")
	}
}

// errorAfterRunner is a test helper TerminalRunner that returns an error after N successful starts.
type errorAfterRunner struct {
	mu       sync.Mutex
	delegate port.TerminalRunner
	count    int
	failAt   int
	failErr  error
}

func newErrorAfterRunner(delegate port.TerminalRunner, failAt int, err error) *errorAfterRunner {
	return &errorAfterRunner{delegate: delegate, failAt: failAt, failErr: err}
}

func (r *errorAfterRunner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	r.mu.Lock()
	r.count++
	n := r.count
	r.mu.Unlock()
	if n > r.failAt {
		return nil, r.failErr
	}
	return r.delegate.Start(ctx, req)
}

func TestOpenWorkspaceHandler_StartErrorCleansUp(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	delegate := apptest.NewFakeTerminalRunner()
	runner := newErrorAfterRunner(delegate, 1, errors.New("pty start failed"))
	reg := session.NewRegistry()
	state := apptest.NewFakeAppStateStore()

	// Create workspace with 2 panes; first Start succeeds, second fails
	pane0 := makePane(t, "pane-0", "/tmp", 0, nil)
	pane1 := makePane(t, "pane-1", "/tmp", 1, nil)
	createWorkspaceWithPanes(t, repo, "ws-1", []*domain.Pane{pane0, pane1})

	handler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh")
	_, err := handler.Handle(ctx, command.OpenWorkspaceCommand{WorkspaceID: "ws-1"})
	if err == nil {
		t.Fatal("expected error when runner.Start fails")
	}

	// Registry should be empty (first session was cleaned up)
	if len(reg.IDs()) != 0 {
		t.Errorf("expected registry to be empty after failure, got IDs: %v", reg.IDs())
	}
}
