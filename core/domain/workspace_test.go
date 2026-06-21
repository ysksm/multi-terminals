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

func newPaneAt(t *testing.T, id string, slot int) *Pane {
	t.Helper()
	p, err := NewPane(mustPaneId(t, id), mustDir(t, "/tmp"), mustSlot(t, slot), nil)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	return p
}

func TestWorkspaceAddPane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	if err := w.AddPane(newPaneAt(t, "p0", 0)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if err := w.AddPane(newPaneAt(t, "p1", 1)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if len(w.Panes()) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(w.Panes()))
	}
}

func TestWorkspaceAddPaneRejectsOverCapacity(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	if err := w.AddPane(newPaneAt(t, "p0", 0)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if err := w.AddPane(newPaneAt(t, "p1", 0)); err == nil {
		t.Error("adding beyond capacity should error")
	}
}

func TestWorkspaceAddPaneRejectsSlotOutOfRange(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	if err := w.AddPane(newPaneAt(t, "p0", 2)); err == nil {
		t.Error("slot >= capacity should error")
	}
}

func TestWorkspaceAddPaneRejectsDuplicateSlot(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.AddPane(newPaneAt(t, "p1", 0)); err == nil {
		t.Error("duplicate slot should error")
	}
}

func TestWorkspaceAddPaneRejectsDuplicateId(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.AddPane(newPaneAt(t, "p0", 1)); err == nil {
		t.Error("duplicate pane id should error")
	}
}

func TestWorkspaceRemovePane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.AddPane(newPaneAt(t, "p1", 1))
	if err := w.RemovePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("RemovePane: %v", err)
	}
	if len(w.Panes()) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(w.Panes()))
	}
	if err := w.RemovePane(mustPaneId(t, "missing")); err == nil {
		t.Error("removing missing pane should error")
	}
}

func TestWorkspaceSetPaneDirectory(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.SetPaneDirectory(mustPaneId(t, "p0"), mustDir(t, "/var/log")); err != nil {
		t.Fatalf("SetPaneDirectory: %v", err)
	}
	if w.Panes()[0].Directory().String() != "/var/log" {
		t.Errorf("Directory = %q", w.Panes()[0].Directory().String())
	}
	if err := w.SetPaneDirectory(mustPaneId(t, "missing"), mustDir(t, "/x")); err == nil {
		t.Error("missing pane should error")
	}
}

func TestWorkspaceSetPaneStartupCommands(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	cmd, _ := NewStartupCommand("npm run dev", true)
	if err := w.SetPaneStartupCommands(mustPaneId(t, "p0"), []StartupCommand{cmd}); err != nil {
		t.Fatalf("SetPaneStartupCommands: %v", err)
	}
	got := w.Panes()[0].Commands()
	if len(got) != 1 || got[0].Command() != "npm run dev" || !got[0].AutoRun() {
		t.Errorf("Commands = %#v", got)
	}
	if err := w.SetPaneStartupCommands(mustPaneId(t, "missing"), nil); err == nil {
		t.Error("missing pane should error")
	}
}
