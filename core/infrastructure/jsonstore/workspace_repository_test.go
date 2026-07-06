package jsonstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain"
)

// buildRepoWorkspace creates a simple workspace suitable for repository tests.
func buildRepoWorkspace(t *testing.T, idStr, nameStr string) *domain.Workspace {
	t.Helper()

	wsID, err := domain.NewWorkspaceId(idStr)
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	wsName, err := domain.NewWorkspaceName(nameStr)
	if err != nil {
		t.Fatalf("NewWorkspaceName: %v", err)
	}
	layout := domain.LayoutSingle

	paneID, _ := domain.NewPaneId("pane-repo-1")
	dir, _ := domain.NewDirectoryPath("/tmp")
	slot, _ := domain.NewSlotIndex(0)
	pane, err := domain.NewPane(paneID, dir, slot, domain.PaneTitle{}, domain.RemoteHost{}, nil)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	w, err := domain.ReconstituteWorkspace(wsID, wsName, layout, []*domain.Pane{pane}, nil, nil)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}
	return w
}

// TestNewWorkspaceRepository verifies that NewWorkspaceRepository creates the workspaces dir.
func TestNewWorkspaceRepository(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}
	expected := filepath.Join(base, "workspaces")
	if repo.dir != expected {
		t.Errorf("repo.dir: want %q, got %q", expected, repo.dir)
	}
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("workspaces directory was not created at %q", expected)
	}
}

// TestSaveCreatesJSONFile verifies that Save writes a .json file and no .tmp file remains.
func TestSaveCreatesJSONFile(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	w := buildRepoWorkspace(t, "ws-save-1", "Save Test")
	ctx := context.Background()

	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// .json file must exist
	jsonPath := filepath.Join(repo.dir, w.ID().String()+".json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Errorf(".json file not found at %q", jsonPath)
	}

	// .tmp file must not remain
	tmpPath := jsonPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist at %q", tmpPath)
	}
}

// TestSaveFindByIDRoundTrip verifies that a saved workspace can be retrieved via FindByID.
func TestSaveFindByIDRoundTrip(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	original := buildRepoWorkspace(t, "ws-roundtrip-1", "RoundTrip Test")
	ctx := context.Background()

	if err := repo.Save(ctx, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, original.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	// Verify identity
	if got.ID().String() != original.ID().String() {
		t.Errorf("ID: want %q, got %q", original.ID().String(), got.ID().String())
	}
	if got.Name().String() != original.Name().String() {
		t.Errorf("Name: want %q, got %q", original.Name().String(), got.Name().String())
	}
	if string(got.Layout()) != string(original.Layout()) {
		t.Errorf("Layout: want %q, got %q", original.Layout(), got.Layout())
	}
	if len(got.Panes()) != len(original.Panes()) {
		t.Errorf("Panes len: want %d, got %d", len(original.Panes()), len(got.Panes()))
	}
}

// TestFindByIDNotFound verifies that FindByID returns ErrWorkspaceNotFound for unknown IDs.
func TestFindByIDNotFound(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	missingID, _ := domain.NewWorkspaceId("ws-does-not-exist")
	ctx := context.Background()

	_, err = repo.FindByID(ctx, missingID)
	if err == nil {
		t.Fatal("expected error for non-existent workspace, got nil")
	}
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

// TestListEmpty verifies that List returns an empty (non-nil) slice when no workspaces exist.
func TestListEmpty(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	ctx := context.Background()
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 workspaces, got %d", len(got))
	}
}

// TestListMultiple verifies that List returns all saved workspaces.
func TestListMultiple(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	ctx := context.Background()
	w1 := buildRepoWorkspace(t, "ws-list-1", "List WS 1")
	w2 := buildRepoWorkspace(t, "ws-list-2", "List WS 2")

	if err := repo.Save(ctx, w1); err != nil {
		t.Fatalf("Save w1: %v", err)
	}
	if err := repo.Save(ctx, w2); err != nil {
		t.Fatalf("Save w2: %v", err)
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(got))
	}

	// Verify both IDs are present
	ids := make(map[string]bool)
	for _, ws := range got {
		ids[ws.ID().String()] = true
	}
	if !ids["ws-list-1"] {
		t.Error("ws-list-1 not found in List result")
	}
	if !ids["ws-list-2"] {
		t.Error("ws-list-2 not found in List result")
	}
}

// TestDeleteNormal verifies that Delete removes a workspace and FindByID returns ErrWorkspaceNotFound.
func TestDeleteNormal(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	ctx := context.Background()
	w := buildRepoWorkspace(t, "ws-delete-1", "Delete WS")

	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := repo.Delete(ctx, w.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// FindByID must now return ErrWorkspaceNotFound
	_, err = repo.FindByID(ctx, w.ID())
	if err == nil {
		t.Fatal("expected ErrWorkspaceNotFound after delete, got nil")
	}
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

// TestDeleteNotFound verifies that Delete returns ErrWorkspaceNotFound for a non-existent ID.
func TestDeleteNotFound(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	ctx := context.Background()
	missingID, _ := domain.NewWorkspaceId("ws-delete-missing")

	err = repo.Delete(ctx, missingID)
	if err == nil {
		t.Fatal("expected ErrWorkspaceNotFound, got nil")
	}
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

// TestFindByIDCorruptJSON verifies that FindByID returns an error (not a panic) when the
// JSON file on disk is corrupt.
func TestFindByIDCorruptJSON(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	// Write a corrupt JSON file directly into the workspaces directory.
	corruptID := "ws-corrupt-json"
	corruptPath := filepath.Join(repo.dir, corruptID+".json")
	if err := os.WriteFile(corruptPath, []byte("{not valid json!!!"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	wsID, _ := domain.NewWorkspaceId(corruptID)
	_, err = repo.FindByID(ctx, wsID)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}

// TestListCorruptJSON verifies that List returns an error (not a panic) when one of the
// JSON files in the directory is corrupt.
func TestListCorruptJSON(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	// Write a corrupt JSON file directly into the workspaces directory.
	corruptPath := filepath.Join(repo.dir, "ws-bad.json")
	if err := os.WriteFile(corruptPath, []byte("{{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	_, err = repo.List(ctx)
	if err == nil {
		t.Fatal("expected error for corrupt JSON in List, got nil")
	}
}

// TestFindByIDUnknownVersion verifies that FindByID returns an error containing
// "unsupported schema version" when the file has an unknown (future) version.
func TestFindByIDUnknownVersion(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	// Write a record with a future schema version.
	futureID := "ws-future-version"
	rec := `{"version":999,"id":"ws-future-version","name":"Future","layout":"single","panes":[]}`
	futurePath := filepath.Join(repo.dir, futureID+".json")
	if err := os.WriteFile(futurePath, []byte(rec), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	wsID, _ := domain.NewWorkspaceId(futureID)
	_, err = repo.FindByID(ctx, wsID)
	if err == nil {
		t.Fatal("expected error for unknown schema version, got nil")
	}
	if !containsString(err.Error(), "unsupported schema version") {
		t.Errorf("error should mention 'unsupported schema version', got: %v", err)
	}
}

// TestListIgnoresNonJSONFiles verifies that List does not fail or include files
// that do not have a ".json" extension.
func TestListIgnoresNonJSONFiles(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	// Write a non-JSON file into the workspaces directory.
	readmePath := filepath.Join(repo.dir, "README.txt")
	if err := os.WriteFile(readmePath, []byte("this is not JSON"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 workspaces (README.txt must be ignored), got %d", len(got))
	}
}

// buildFullWorkspace creates a workspace with multiple panes, a lastActive pane,
// and a maximized pane. It uses grid_2x2 layout to accommodate 2 panes.
func buildFullWorkspace(t *testing.T) *domain.Workspace {
	t.Helper()

	wsID, err := domain.NewWorkspaceId("ws-full-roundtrip")
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	wsName, err := domain.NewWorkspaceName("Full Round-Trip Workspace")
	if err != nil {
		t.Fatalf("NewWorkspaceName: %v", err)
	}
	layout := domain.LayoutGrid2x2

	// Pane 0
	pane0ID, _ := domain.NewPaneId("pane-full-0")
	dir0, _ := domain.NewDirectoryPath("/home/user")
	slot0, _ := domain.NewSlotIndex(0)
	cmd0, _ := domain.NewStartupCommand("echo hello", true)
	pane0, err := domain.NewPane(pane0ID, dir0, slot0, domain.PaneTitle{}, domain.RemoteHost{}, []domain.StartupCommand{cmd0})
	if err != nil {
		t.Fatalf("NewPane 0: %v", err)
	}

	// Pane 1
	pane1ID, _ := domain.NewPaneId("pane-full-1")
	dir1, _ := domain.NewDirectoryPath("/var/log")
	slot1, _ := domain.NewSlotIndex(1)
	cmd1a, _ := domain.NewStartupCommand("ls -la", false)
	cmd1b, _ := domain.NewStartupCommand("pwd", true)
	pane1, err := domain.NewPane(pane1ID, dir1, slot1, domain.PaneTitle{}, domain.RemoteHost{}, []domain.StartupCommand{cmd1a, cmd1b})
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	lastActiveID := pane0ID
	maximizedID := pane1ID

	w, err := domain.ReconstituteWorkspace(wsID, wsName, layout, []*domain.Pane{pane0, pane1}, &lastActiveID, &maximizedID)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}
	return w
}

// TestFullRoundTrip verifies that a workspace with multiple panes, lastActive, and
// maximized survives a Save→FindByID cycle with all fields intact.
func TestFullRoundTrip(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	original := buildFullWorkspace(t)
	ctx := context.Background()

	if err := repo.Save(ctx, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, original.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	// ID / Name / Layout
	if got.ID().String() != original.ID().String() {
		t.Errorf("ID: want %q, got %q", original.ID().String(), got.ID().String())
	}
	if got.Name().String() != original.Name().String() {
		t.Errorf("Name: want %q, got %q", original.Name().String(), got.Name().String())
	}
	if string(got.Layout()) != string(original.Layout()) {
		t.Errorf("Layout: want %q, got %q", original.Layout(), got.Layout())
	}

	// Pane count
	origPanes := original.Panes()
	gotPanes := got.Panes()
	if len(gotPanes) != len(origPanes) {
		t.Fatalf("Panes count: want %d, got %d", len(origPanes), len(gotPanes))
	}

	// Build a map from ID to pane for order-independent comparison.
	origPaneMap := make(map[string]*domain.Pane, len(origPanes))
	for _, p := range origPanes {
		origPaneMap[p.ID().String()] = p
	}
	for _, gp := range gotPanes {
		op, ok := origPaneMap[gp.ID().String()]
		if !ok {
			t.Errorf("unexpected pane id %q in result", gp.ID().String())
			continue
		}
		if gp.Directory().String() != op.Directory().String() {
			t.Errorf("pane %q directory: want %q, got %q", gp.ID().String(), op.Directory().String(), gp.Directory().String())
		}
		if gp.Slot().Int() != op.Slot().Int() {
			t.Errorf("pane %q slot: want %d, got %d", gp.ID().String(), op.Slot().Int(), gp.Slot().Int())
		}
		origCmds := op.Commands()
		gotCmds := gp.Commands()
		if len(gotCmds) != len(origCmds) {
			t.Errorf("pane %q commands count: want %d, got %d", gp.ID().String(), len(origCmds), len(gotCmds))
		} else {
			for i, gc := range gotCmds {
				oc := origCmds[i]
				if gc.Command() != oc.Command() {
					t.Errorf("pane %q command[%d].Command: want %q, got %q", gp.ID().String(), i, oc.Command(), gc.Command())
				}
				if gc.AutoRun() != oc.AutoRun() {
					t.Errorf("pane %q command[%d].AutoRun: want %v, got %v", gp.ID().String(), i, oc.AutoRun(), gc.AutoRun())
				}
			}
		}
	}

	// LastActivePaneId
	origLastActive, origHasLA := original.LastActivePaneId()
	gotLastActive, gotHasLA := got.LastActivePaneId()
	if origHasLA != gotHasLA {
		t.Errorf("LastActivePaneId presence: want %v, got %v", origHasLA, gotHasLA)
	} else if origHasLA && origLastActive.String() != gotLastActive.String() {
		t.Errorf("LastActivePaneId: want %q, got %q", origLastActive.String(), gotLastActive.String())
	}

	// MaximizedPaneId
	origMaximized, origHasMax := original.MaximizedPaneId()
	gotMaximized, gotHasMax := got.MaximizedPaneId()
	if origHasMax != gotHasMax {
		t.Errorf("MaximizedPaneId presence: want %v, got %v", origHasMax, gotHasMax)
	} else if origHasMax && origMaximized.String() != gotMaximized.String() {
		t.Errorf("MaximizedPaneId: want %q, got %q", origMaximized.String(), gotMaximized.String())
	}
}

// TestPathTraversalRejected verifies that Save, FindByID, and Delete all return errors
// when presented with a workspace ID that contains path-traversal components (Fix 3).
func TestPathTraversalRejected(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	ctx := context.Background()

	// Construct the malicious ID directly (domain.NewWorkspaceId allows arbitrary strings).
	maliciousID, err := domain.NewWorkspaceId("../evil")
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}

	// FindByID must error.
	if _, err := repo.FindByID(ctx, maliciousID); err == nil {
		t.Error("FindByID with path-traversal id: expected error, got nil")
	}

	// Delete must error.
	if err := repo.Delete(ctx, maliciousID); err == nil {
		t.Error("Delete with path-traversal id: expected error, got nil")
	}

	// Save must error: build a minimal workspace with the malicious ID.
	paneID, _ := domain.NewPaneId("pane-trav-1")
	dir, _ := domain.NewDirectoryPath("/tmp")
	slot, _ := domain.NewSlotIndex(0)
	pane, _ := domain.NewPane(paneID, dir, slot, domain.PaneTitle{}, domain.RemoteHost{}, nil)
	wsName, _ := domain.NewWorkspaceName("Traversal WS")
	ws, _ := domain.ReconstituteWorkspace(maliciousID, wsName, domain.LayoutSingle, []*domain.Pane{pane}, nil, nil)
	if err := repo.Save(ctx, ws); err == nil {
		t.Error("Save with path-traversal id: expected error, got nil")
	}

	// Confirm no stray "evil.json" was created in the parent of the workspaces dir.
	evilPath := filepath.Join(base, "evil.json")
	if _, err := os.Stat(evilPath); !os.IsNotExist(err) {
		t.Errorf("path-traversal created stray file at %q", evilPath)
	}
}

// TestListIgnoresTmpFiles verifies that List ignores leftover .json.tmp files (Fix 4).
func TestListIgnoresTmpFiles(t *testing.T) {
	base := t.TempDir()
	repo, err := NewWorkspaceRepository(base)
	if err != nil {
		t.Fatalf("NewWorkspaceRepository: %v", err)
	}

	// Place a leftover .json.tmp file directly in the workspaces directory.
	tmpFile := filepath.Join(repo.dir, "ws-leftover.json.tmp")
	if err := os.WriteFile(tmpFile, []byte(`{"not":"valid json for a workspace"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error for .json.tmp file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 workspaces (tmp file must be ignored), got %d", len(got))
	}
}

// containsString is a helper that checks whether substr appears in s.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
