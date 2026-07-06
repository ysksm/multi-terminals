package jsonstore

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain"
)

// buildTestWorkspace builds a workspace with two panes, lastActive and maximized set.
// layout must accommodate at least 2 panes (e.g. split_vertical).
func buildTestWorkspace(t *testing.T) *domain.Workspace {
	t.Helper()

	wsID, err := domain.NewWorkspaceId("ws-mapper-1")
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	wsName, err := domain.NewWorkspaceName("Mapper Test")
	if err != nil {
		t.Fatalf("NewWorkspaceName: %v", err)
	}
	layout := domain.LayoutSplitVertical

	paneID1, _ := domain.NewPaneId("pane-1")
	dir1, _ := domain.NewDirectoryPath("/home/user")
	slot1, _ := domain.NewSlotIndex(0)
	cmd1, _ := domain.NewStartupCommand("bash", true)
	pane1, err := domain.NewPane(paneID1, dir1, slot1, domain.PaneTitle{}, domain.RemoteHost{}, []domain.StartupCommand{cmd1})
	if err != nil {
		t.Fatalf("NewPane pane1: %v", err)
	}

	paneID2, _ := domain.NewPaneId("pane-2")
	dir2, _ := domain.NewDirectoryPath("/tmp")
	slot2, _ := domain.NewSlotIndex(1)
	cmd2, _ := domain.NewStartupCommand("vim", false)
	cmd3, _ := domain.NewStartupCommand("htop", true)
	pane2, err := domain.NewPane(paneID2, dir2, slot2, domain.PaneTitle{}, domain.RemoteHost{}, []domain.StartupCommand{cmd2, cmd3})
	if err != nil {
		t.Fatalf("NewPane pane2: %v", err)
	}

	w, err := domain.ReconstituteWorkspace(wsID, wsName, layout, []*domain.Pane{pane1, pane2}, &paneID1, &paneID2)
	if err != nil {
		t.Fatalf("ReconstituteWorkspace: %v", err)
	}
	return w
}

// TestMapperRoundTrip tests that toRecord -> toDomain produces an equivalent workspace.
func TestMapperRoundTrip(t *testing.T) {
	original := buildTestWorkspace(t)

	rec := toRecord(original)

	// Verify record has expected version
	if rec.Version != CurrentSchemaVersion {
		t.Errorf("rec.Version: want %d, got %d", CurrentSchemaVersion, rec.Version)
	}

	// Round-trip through toDomain
	got, err := toDomain(rec)
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}

	// ID
	if got.ID().String() != original.ID().String() {
		t.Errorf("ID: want %q, got %q", original.ID().String(), got.ID().String())
	}
	// Name
	if got.Name().String() != original.Name().String() {
		t.Errorf("Name: want %q, got %q", original.Name().String(), got.Name().String())
	}
	// Layout
	if string(got.Layout()) != string(original.Layout()) {
		t.Errorf("Layout: want %q, got %q", original.Layout(), got.Layout())
	}
	// Panes count
	origPanes := original.Panes()
	gotPanes := got.Panes()
	if len(gotPanes) != len(origPanes) {
		t.Fatalf("Panes len: want %d, got %d", len(origPanes), len(gotPanes))
	}

	// Build a lookup by pane ID to avoid ordering issues
	origPaneMap := make(map[string]*domain.Pane, len(origPanes))
	for _, p := range origPanes {
		origPaneMap[p.ID().String()] = p
	}
	for _, gp := range gotPanes {
		op, ok := origPaneMap[gp.ID().String()]
		if !ok {
			t.Errorf("unexpected pane ID %q in result", gp.ID().String())
			continue
		}
		if gp.Directory().String() != op.Directory().String() {
			t.Errorf("pane %s Directory: want %q, got %q", gp.ID(), op.Directory().String(), gp.Directory().String())
		}
		if gp.Slot().Int() != op.Slot().Int() {
			t.Errorf("pane %s Slot: want %d, got %d", gp.ID(), op.Slot().Int(), gp.Slot().Int())
		}
		origCmds := op.Commands()
		gotCmds := gp.Commands()
		if len(gotCmds) != len(origCmds) {
			t.Errorf("pane %s Commands len: want %d, got %d", gp.ID(), len(origCmds), len(gotCmds))
			continue
		}
		for j, gc := range gotCmds {
			oc := origCmds[j]
			if gc.Command() != oc.Command() {
				t.Errorf("pane %s cmd[%d] Command: want %q, got %q", gp.ID(), j, oc.Command(), gc.Command())
			}
			if gc.AutoRun() != oc.AutoRun() {
				t.Errorf("pane %s cmd[%d] AutoRun: want %v, got %v", gp.ID(), j, oc.AutoRun(), gc.AutoRun())
			}
		}
	}

	// LastActivePaneId
	origLastActive, origHasLastActive := original.LastActivePaneId()
	gotLastActive, gotHasLastActive := got.LastActivePaneId()
	if origHasLastActive != gotHasLastActive {
		t.Errorf("LastActivePaneId ok: want %v, got %v", origHasLastActive, gotHasLastActive)
	} else if origHasLastActive && origLastActive.String() != gotLastActive.String() {
		t.Errorf("LastActivePaneId: want %q, got %q", origLastActive.String(), gotLastActive.String())
	}

	// MaximizedPaneId
	origMaximized, origHasMaximized := original.MaximizedPaneId()
	gotMaximized, gotHasMaximized := got.MaximizedPaneId()
	if origHasMaximized != gotHasMaximized {
		t.Errorf("MaximizedPaneId ok: want %v, got %v", origHasMaximized, gotHasMaximized)
	} else if origHasMaximized && origMaximized.String() != gotMaximized.String() {
		t.Errorf("MaximizedPaneId: want %q, got %q", origMaximized.String(), gotMaximized.String())
	}
}

// TestToDomainUnknownVersion verifies that toDomain rejects future schema versions.
func TestToDomainUnknownVersion(t *testing.T) {
	rec := workspaceRecord{
		Version: 999,
		ID:      "ws-1",
		Name:    "Test",
		Layout:  "single",
		Panes:   []paneRecord{{ID: "p-1", Directory: "/tmp", Slot: 0}},
	}
	_, err := toDomain(rec)
	if err == nil {
		t.Fatal("expected error for version 999, got nil")
	}
	if !containsStr(err.Error(), "unsupported schema version") {
		t.Errorf("expected 'unsupported schema version' in error, got: %v", err)
	}
}

// TestToDomainVersionZero verifies that toDomain rejects version 0.
func TestToDomainVersionZero(t *testing.T) {
	rec := workspaceRecord{
		Version: 0,
		ID:      "ws-1",
		Name:    "Test",
		Layout:  "single",
		Panes:   []paneRecord{{ID: "p-1", Directory: "/tmp", Slot: 0}},
	}
	_, err := toDomain(rec)
	if err == nil {
		t.Fatal("expected error for version 0, got nil")
	}
}

// TestToDomainInvalidLayout verifies that toDomain rejects an unknown layout string.
func TestToDomainInvalidLayout(t *testing.T) {
	rec := workspaceRecord{
		Version: CurrentSchemaVersion,
		ID:      "ws-1",
		Name:    "Test",
		Layout:  "totally_unknown_layout",
		Panes:   []paneRecord{},
	}
	_, err := toDomain(rec)
	if err == nil {
		t.Fatal("expected error for invalid layout, got nil")
	}
}

// TestToRecordVersionIsCurrentSchemaVersion verifies toRecord always sets the current version.
func TestToRecordVersionIsCurrentSchemaVersion(t *testing.T) {
	w := buildTestWorkspace(t)
	rec := toRecord(w)
	if rec.Version != CurrentSchemaVersion {
		t.Errorf("want Version=%d, got %d", CurrentSchemaVersion, rec.Version)
	}
}

// TestToDomainNilOptionalFields verifies toDomain handles nil lastActive and maximized.
func TestToDomainNilOptionalFields(t *testing.T) {
	rec := workspaceRecord{
		Version:          CurrentSchemaVersion,
		ID:               "ws-nil-opts",
		Name:             "No Opts",
		Layout:           "single",
		Panes:            []paneRecord{{ID: "p-1", Directory: "/home", Slot: 0}},
		LastActivePaneID: nil,
		MaximizedPaneID:  nil,
	}
	w, err := toDomain(rec)
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("expected LastActivePaneId to be unset")
	}
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("expected MaximizedPaneId to be unset")
	}
}

func TestPaneTitleRoundTripAndBackwardCompat(t *testing.T) {
	// ラウンドトリップ: title を持つ pane を record 化→ドメイン復元で保持される
	title, _ := domain.NewPaneTitle("API")
	dir, _ := domain.NewDirectoryPath("/tmp")
	slot, _ := domain.NewSlotIndex(0)
	pid, _ := domain.NewPaneId("p1")
	pane, _ := domain.NewPane(pid, dir, slot, title, domain.RemoteHost{}, nil)
	wsID, _ := domain.NewWorkspaceId("w1")
	name, _ := domain.NewWorkspaceName("ws")
	w, _ := domain.NewWorkspace(wsID, name, domain.LayoutSingle)
	_ = w.AddPane(pane)

	rec := toRecord(w)
	got, err := toDomain(rec)
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}
	if got.Panes()[0].Title().String() != "API" {
		t.Fatalf("round-trip lost title: %q", got.Panes()[0].Title().String())
	}

	// 後方互換: title フィールドが無い JSON を読み込んでも空タイトルで成功する
	const legacy = `{"version":1,"id":"w1","name":"ws","layout":"single","panes":[{"id":"p1","directory":"/tmp","slot":0,"commands":[]}]}`
	var lrec workspaceRecord
	if err := json.Unmarshal([]byte(legacy), &lrec); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	lw, err := toDomain(lrec)
	if err != nil {
		t.Fatalf("toDomain legacy: %v", err)
	}
	if !lw.Panes()[0].Title().IsZero() {
		t.Fatalf("legacy pane should have empty title, got %q", lw.Panes()[0].Title().String())
	}
}

// containsStr checks if s contains sub (helper for test package to avoid import issues).
func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Verify toDomain wraps errors properly using errors package.
var _ = errors.New // ensure errors import is used
