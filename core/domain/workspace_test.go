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
	p, err := NewPane(mustPaneId(t, id), mustDir(t, "/tmp"), mustSlot(t, slot), PaneTitle{}, nil)
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

func TestWorkspaceSetPaneTitle(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	panes := w.Panes()
	id := panes[0].ID()

	title, _ := NewPaneTitle("API server")
	if err := w.SetPaneTitle(id, title); err != nil {
		t.Fatalf("SetPaneTitle: %v", err)
	}
	if got := w.Panes()[0].Title().String(); got != "API server" {
		t.Fatalf("title not set: got %q", got)
	}

	// 存在しない pane はエラー
	missing, _ := NewPaneId("does-not-exist")
	if err := w.SetPaneTitle(missing, title); err == nil {
		t.Fatal("SetPaneTitle on missing pane: expected error, got nil")
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

func TestWorkspaceSetLastActivePane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("new workspace should have no last active pane")
	}
	if err := w.SetLastActivePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("SetLastActivePane: %v", err)
	}
	got, ok := w.LastActivePaneId()
	if !ok || !got.Equals(mustPaneId(t, "p0")) {
		t.Errorf("LastActivePaneId = %v, ok=%v", got, ok)
	}
	if err := w.SetLastActivePane(mustPaneId(t, "missing")); err == nil {
		t.Error("missing pane should error")
	}
}

func TestWorkspaceMaximizeAndRestore(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.AddPane(newPaneAt(t, "p1", 1))
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("new workspace should not be maximized")
	}
	if err := w.MaximizePane(mustPaneId(t, "p1")); err != nil {
		t.Fatalf("MaximizePane: %v", err)
	}
	got, ok := w.MaximizedPaneId()
	if !ok || !got.Equals(mustPaneId(t, "p1")) {
		t.Errorf("MaximizedPaneId = %v, ok=%v", got, ok)
	}
	w.RestoreLayout()
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("RestoreLayout should clear maximized")
	}
	if err := w.MaximizePane(mustPaneId(t, "missing")); err == nil {
		t.Error("maximizing missing pane should error")
	}
}

func TestRemovePaneClearsActiveAndMaximized(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.SetLastActivePane(mustPaneId(t, "p0"))
	_ = w.MaximizePane(mustPaneId(t, "p0"))
	if err := w.RemovePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("RemovePane: %v", err)
	}
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("removing active pane should clear lastActive")
	}
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("removing maximized pane should clear maximized")
	}
}

func TestWorkspaceChangeLayoutRejectsPaneSlotOutOfRange(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	// place a pane in slot 1 (valid for SplitVertical capacity 2)
	if err := w.AddPane(newPaneAt(t, "p1", 1)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	// shrinking to Single (capacity 1) must reject: pane sits at slot 1 >= 1
	if err := w.ChangeLayout(LayoutSingle); err == nil {
		t.Error("ChangeLayout to smaller layout with an out-of-range pane slot should error")
	}
	// layout must be unchanged after the rejected change
	if w.Layout() != LayoutSplitVertical {
		t.Errorf("Layout should remain SplitVertical after rejected ChangeLayout, got %q", w.Layout())
	}
}

// --- ReconstituteWorkspace tests ---

func TestReconstituteWorkspace_HappyPath(t *testing.T) {
	id := mustWorkspaceId(t, "ws1")
	name := mustName(t, "MyWS")
	p0 := newPaneAt(t, "p0", 0)
	p1 := newPaneAt(t, "p1", 1)
	lastActive := mustPaneId(t, "p0")
	maximized := mustPaneId(t, "p1")

	w, err := ReconstituteWorkspace(id, name, LayoutSplitVertical, []*Pane{p0, p1}, &lastActive, &maximized)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}

	if !w.ID().Equals(id) {
		t.Errorf("ID = %v, want %v", w.ID(), id)
	}
	if w.Name().String() != "MyWS" {
		t.Errorf("Name = %q, want %q", w.Name().String(), "MyWS")
	}
	if w.Layout() != LayoutSplitVertical {
		t.Errorf("Layout = %q, want %q", w.Layout(), LayoutSplitVertical)
	}
	panes := w.Panes()
	if len(panes) != 2 {
		t.Fatalf("Panes len = %d, want 2", len(panes))
	}
	gotLast, ok := w.LastActivePaneId()
	if !ok || !gotLast.Equals(lastActive) {
		t.Errorf("LastActivePaneId = %v, ok=%v", gotLast, ok)
	}
	gotMax, ok := w.MaximizedPaneId()
	if !ok || !gotMax.Equals(maximized) {
		t.Errorf("MaximizedPaneId = %v, ok=%v", gotMax, ok)
	}
}

func TestReconstituteWorkspace_NilLastActiveAndMaximized(t *testing.T) {
	id := mustWorkspaceId(t, "ws2")
	name := mustName(t, "WS2")
	w, err := ReconstituteWorkspace(id, name, LayoutSingle, []*Pane{newPaneAt(t, "p0", 0)}, nil, nil)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("nil lastActive should yield no LastActivePaneId")
	}
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("nil maximized should yield no MaximizedPaneId")
	}
}

func TestReconstituteWorkspace_EmptyPanes(t *testing.T) {
	id := mustWorkspaceId(t, "ws3")
	name := mustName(t, "WS3")
	w, err := ReconstituteWorkspace(id, name, LayoutGrid2x2, nil, nil, nil)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace with no panes: %v", err)
	}
	if len(w.Panes()) != 0 {
		t.Errorf("expected 0 panes, got %d", len(w.Panes()))
	}
}

func TestReconstituteWorkspace_RejectsOverCapacity(t *testing.T) {
	id := mustWorkspaceId(t, "ws4")
	name := mustName(t, "WS4")
	panes := []*Pane{
		newPaneAt(t, "p0", 0),
		newPaneAt(t, "p1", 1),
	}
	// LayoutSingle has capacity 1, so 2 panes must fail
	if _, err := ReconstituteWorkspace(id, name, LayoutSingle, panes, nil, nil); err == nil {
		t.Error("pane count exceeding capacity should error")
	}
}

func TestReconstituteWorkspace_RejectsDuplicateSlot(t *testing.T) {
	id := mustWorkspaceId(t, "ws5")
	name := mustName(t, "WS5")
	p0 := newPaneAt(t, "p0", 0)
	p1 := newPaneAt(t, "p1", 0) // same slot as p0
	if _, err := ReconstituteWorkspace(id, name, LayoutSplitVertical, []*Pane{p0, p1}, nil, nil); err == nil {
		t.Error("duplicate slot should error")
	}
}

func TestReconstituteWorkspace_RejectsDuplicatePaneId(t *testing.T) {
	id := mustWorkspaceId(t, "ws6")
	name := mustName(t, "WS6")
	p0 := newPaneAt(t, "p0", 0)
	p0dup := newPaneAt(t, "p0", 1) // same id as p0
	if _, err := ReconstituteWorkspace(id, name, LayoutSplitVertical, []*Pane{p0, p0dup}, nil, nil); err == nil {
		t.Error("duplicate pane id should error")
	}
}

func TestReconstituteWorkspace_RejectsLastActiveNotInPanes(t *testing.T) {
	id := mustWorkspaceId(t, "ws7")
	name := mustName(t, "WS7")
	p0 := newPaneAt(t, "p0", 0)
	missing := mustPaneId(t, "missing")
	if _, err := ReconstituteWorkspace(id, name, LayoutSingle, []*Pane{p0}, &missing, nil); err == nil {
		t.Error("lastActive pointing to non-existent pane should error")
	}
}

func TestReconstituteWorkspace_RejectsMaximizedNotInPanes(t *testing.T) {
	id := mustWorkspaceId(t, "ws8")
	name := mustName(t, "WS8")
	p0 := newPaneAt(t, "p0", 0)
	missing := mustPaneId(t, "missing")
	if _, err := ReconstituteWorkspace(id, name, LayoutSingle, []*Pane{p0}, nil, &missing); err == nil {
		t.Error("maximized pointing to non-existent pane should error")
	}
}

func TestReconstituteWorkspace_PointerAliasIsolation(t *testing.T) {
	// Mutating the caller's PaneId pointers after reconstitution must not affect the workspace.
	id := mustWorkspaceId(t, "ws9")
	name := mustName(t, "WS9")
	p0 := newPaneAt(t, "p0", 0)
	lastActive := mustPaneId(t, "p0")
	maximized := mustPaneId(t, "p0")

	w, err := ReconstituteWorkspace(id, name, LayoutSingle, []*Pane{p0}, &lastActive, &maximized)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}
	// Overwrite the caller's variables — workspace must not be affected.
	lastActive = mustPaneId(t, "changed")
	maximized = mustPaneId(t, "changed")

	gotLast, ok := w.LastActivePaneId()
	if !ok || gotLast.String() != "p0" {
		t.Errorf("LastActivePaneId = %v, ok=%v; want p0", gotLast, ok)
	}
	gotMax, ok := w.MaximizedPaneId()
	if !ok || gotMax.String() != "p0" {
		t.Errorf("MaximizedPaneId = %v, ok=%v; want p0", gotMax, ok)
	}
}
