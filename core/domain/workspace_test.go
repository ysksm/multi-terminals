package domain

import "testing"

func mustWorkspaceId(t *testing.T, v string) WorkspaceId {
	t.Helper()
	id, err := NewWorkspaceId(v)
	if err != nil {
		t.Fatalf("NewWorkspaceId(%q): %v", v, err)
	}
	return id
}

func mustName(t *testing.T, v string) WorkspaceName {
	t.Helper()
	n, err := NewWorkspaceName(v)
	if err != nil {
		t.Fatalf("NewWorkspaceName(%q): %v", v, err)
	}
	return n
}

func newTestWorkspace(t *testing.T, layout LayoutPreset) *Workspace {
	t.Helper()
	w, err := NewWorkspace(mustWorkspaceId(t, "ws1"), mustName(t, "WS"), layout)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	return w
}

func TestNewWorkspace(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	if !w.ID().Equals(mustWorkspaceId(t, "ws1")) {
		t.Error("ID mismatch")
	}
	if w.Name().String() != "WS" {
		t.Errorf("Name = %q", w.Name().String())
	}
	if w.Layout() != LayoutGrid2x2 {
		t.Errorf("Layout = %q", w.Layout())
	}
	if len(w.Panes()) != 0 {
		t.Errorf("new workspace should have no panes, got %d", len(w.Panes()))
	}
}

func TestNewWorkspaceRejectsInvalidLayout(t *testing.T) {
	if _, err := NewWorkspace(mustWorkspaceId(t, "ws1"), mustName(t, "WS"), LayoutPreset("bogus")); err == nil {
		t.Error("invalid layout should error")
	}
}

func TestNewWorkspaceRejectsZeroId(t *testing.T) {
	var zero WorkspaceId
	if _, err := NewWorkspace(zero, mustName(t, "WS"), LayoutSingle); err == nil {
		t.Error("zero workspace id should error")
	}
}

func TestWorkspaceRename(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	w.Rename(mustName(t, "Renamed"))
	if w.Name().String() != "Renamed" {
		t.Errorf("Name = %q", w.Name().String())
	}
}

func TestWorkspaceChangeLayout(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	if err := w.ChangeLayout(LayoutSplitVertical); err != nil {
		t.Fatalf("ChangeLayout: %v", err)
	}
	if w.Layout() != LayoutSplitVertical {
		t.Errorf("Layout = %q", w.Layout())
	}
	if err := w.ChangeLayout(LayoutPreset("bogus")); err == nil {
		t.Error("invalid layout should error")
	}
}
