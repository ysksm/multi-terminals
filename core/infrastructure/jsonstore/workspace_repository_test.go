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
	pane, err := domain.NewPane(paneID, dir, slot, nil)
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
