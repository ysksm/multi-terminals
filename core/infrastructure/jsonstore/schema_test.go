package jsonstore

import (
	"encoding/json"
	"testing"
)

// TestWorkspaceRecordRoundTrip confirms that workspaceRecord marshals and unmarshals
// correctly, including omitempty behaviour for optional pointer fields.
func TestWorkspaceRecordRoundTrip(t *testing.T) {
	t.Run("nil optional fields are omitted in JSON", func(t *testing.T) {
		rec := workspaceRecord{
			Version: CurrentSchemaVersion,
			ID:      "ws-1",
			Name:    "My Workspace",
			Layout:  "single",
			Panes: []paneRecord{
				{
					ID:        "p-1",
					Directory: "/home/user",
					Slot:      0,
					Commands: []startupCommandRecord{
						{Command: "ls", AutoRun: true},
					},
				},
			},
			LastActivePaneID: nil,
			MaximizedPaneID:  nil,
		}

		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// nil pointer fields should be absent from JSON due to omitempty
		jsonStr := string(data)
		if contains(jsonStr, "lastActivePaneId") {
			t.Errorf("expected lastActivePaneId to be omitted when nil, got: %s", jsonStr)
		}
		if contains(jsonStr, "maximizedPaneId") {
			t.Errorf("expected maximizedPaneId to be omitted when nil, got: %s", jsonStr)
		}

		// unmarshal back
		var got workspaceRecord
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		assertWorkspaceRecordEqual(t, rec, got)
	})

	t.Run("non-nil optional fields are present in JSON and survive round-trip", func(t *testing.T) {
		lastActive := "p-1"
		maximized := "p-2"

		rec := workspaceRecord{
			Version: CurrentSchemaVersion,
			ID:      "ws-2",
			Name:    "Multi Pane",
			Layout:  "grid_2x2",
			Panes: []paneRecord{
				{ID: "p-1", Directory: "/tmp", Slot: 0, Commands: nil},
				{ID: "p-2", Directory: "/var", Slot: 1, Commands: []startupCommandRecord{
					{Command: "vim", AutoRun: false},
				}},
			},
			LastActivePaneID: &lastActive,
			MaximizedPaneID:  &maximized,
		}

		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(data)
		if !contains(jsonStr, "lastActivePaneId") {
			t.Errorf("expected lastActivePaneId to be present, got: %s", jsonStr)
		}
		if !contains(jsonStr, "maximizedPaneId") {
			t.Errorf("expected maximizedPaneId to be present, got: %s", jsonStr)
		}

		var got workspaceRecord
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		assertWorkspaceRecordEqual(t, rec, got)
	})

	t.Run("CurrentSchemaVersion is 1", func(t *testing.T) {
		if CurrentSchemaVersion != 1 {
			t.Errorf("expected CurrentSchemaVersion = 1, got %d", CurrentSchemaVersion)
		}
	})
}

// TestAppStateRecordRoundTrip confirms that appStateRecord marshals and unmarshals.
func TestAppStateRecordRoundTrip(t *testing.T) {
	rec := appStateRecord{
		Version:               CurrentSchemaVersion,
		LastOpenedWorkspaceID: "ws-abc",
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got appStateRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Version != rec.Version {
		t.Errorf("Version: want %d, got %d", rec.Version, got.Version)
	}
	if got.LastOpenedWorkspaceID != rec.LastOpenedWorkspaceID {
		t.Errorf("LastOpenedWorkspaceID: want %q, got %q", rec.LastOpenedWorkspaceID, got.LastOpenedWorkspaceID)
	}
}

// contains is a helper to check substring presence without importing strings in test.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

func assertWorkspaceRecordEqual(t *testing.T, want, got workspaceRecord) {
	t.Helper()
	if got.Version != want.Version {
		t.Errorf("Version: want %d, got %d", want.Version, got.Version)
	}
	if got.ID != want.ID {
		t.Errorf("ID: want %q, got %q", want.ID, got.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name: want %q, got %q", want.Name, got.Name)
	}
	if got.Layout != want.Layout {
		t.Errorf("Layout: want %q, got %q", want.Layout, got.Layout)
	}
	if len(got.Panes) != len(want.Panes) {
		t.Fatalf("Panes length: want %d, got %d", len(want.Panes), len(got.Panes))
	}
	for i := range want.Panes {
		wp := want.Panes[i]
		gp := got.Panes[i]
		if gp.ID != wp.ID {
			t.Errorf("Panes[%d].ID: want %q, got %q", i, wp.ID, gp.ID)
		}
		if gp.Directory != wp.Directory {
			t.Errorf("Panes[%d].Directory: want %q, got %q", i, wp.Directory, gp.Directory)
		}
		if gp.Slot != wp.Slot {
			t.Errorf("Panes[%d].Slot: want %d, got %d", i, wp.Slot, gp.Slot)
		}
		if len(gp.Commands) != len(wp.Commands) {
			t.Errorf("Panes[%d].Commands length: want %d, got %d", i, len(wp.Commands), len(gp.Commands))
		}
		for j := range wp.Commands {
			wc := wp.Commands[j]
			gc := gp.Commands[j]
			if gc.Command != wc.Command {
				t.Errorf("Panes[%d].Commands[%d].Command: want %q, got %q", i, j, wc.Command, gc.Command)
			}
			if gc.AutoRun != wc.AutoRun {
				t.Errorf("Panes[%d].Commands[%d].AutoRun: want %v, got %v", i, j, wc.AutoRun, gc.AutoRun)
			}
		}
	}
	// optional pointer fields
	if want.LastActivePaneID == nil {
		if got.LastActivePaneID != nil {
			t.Errorf("LastActivePaneID: want nil, got %q", *got.LastActivePaneID)
		}
	} else {
		if got.LastActivePaneID == nil {
			t.Errorf("LastActivePaneID: want %q, got nil", *want.LastActivePaneID)
		} else if *got.LastActivePaneID != *want.LastActivePaneID {
			t.Errorf("LastActivePaneID: want %q, got %q", *want.LastActivePaneID, *got.LastActivePaneID)
		}
	}
	if want.MaximizedPaneID == nil {
		if got.MaximizedPaneID != nil {
			t.Errorf("MaximizedPaneID: want nil, got %q", *got.MaximizedPaneID)
		}
	} else {
		if got.MaximizedPaneID == nil {
			t.Errorf("MaximizedPaneID: want %q, got nil", *want.MaximizedPaneID)
		} else if *got.MaximizedPaneID != *want.MaximizedPaneID {
			t.Errorf("MaximizedPaneID: want %q, got %q", *want.MaximizedPaneID, *got.MaximizedPaneID)
		}
	}
}
