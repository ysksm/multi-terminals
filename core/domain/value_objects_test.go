package domain

import "testing"

func TestNewWorkspaceId(t *testing.T) {
	id, err := NewWorkspaceId("ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "ws-1" {
		t.Errorf("String() = %q, want %q", id.String(), "ws-1")
	}
	if id.IsZero() {
		t.Error("id should not be zero")
	}
	if _, err := NewWorkspaceId("   "); err == nil {
		t.Error("blank id should error")
	}
	var zero WorkspaceId
	if !zero.IsZero() {
		t.Error("zero value should report IsZero")
	}
}

func TestWorkspaceIdEquals(t *testing.T) {
	a, _ := NewWorkspaceId("ws-1")
	b, _ := NewWorkspaceId("ws-1")
	c, _ := NewWorkspaceId("ws-2")
	if !a.Equals(b) {
		t.Error("equal ids should be Equals")
	}
	if a.Equals(c) {
		t.Error("different ids should not be Equals")
	}
}

func TestNewPaneId(t *testing.T) {
	id, err := NewPaneId("pane-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "pane-1" {
		t.Errorf("String() = %q", id.String())
	}
	if _, err := NewPaneId(""); err == nil {
		t.Error("empty pane id should error")
	}
}

func TestNewWorkspaceName(t *testing.T) {
	n, err := NewWorkspaceName("My WS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.String() != "My WS" {
		t.Errorf("String() = %q", n.String())
	}
	if _, err := NewWorkspaceName(" "); err == nil {
		t.Error("blank name should error")
	}
}

func TestNewDirectoryPath(t *testing.T) {
	p, err := NewDirectoryPath("/home/me/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.String() != "/home/me/proj" {
		t.Errorf("String() = %q", p.String())
	}
	if _, err := NewDirectoryPath(""); err == nil {
		t.Error("empty path should error")
	}
}

func TestNewSlotIndex(t *testing.T) {
	s, err := NewSlotIndex(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Int() != 0 {
		t.Errorf("Int() = %d", s.Int())
	}
	if _, err := NewSlotIndex(-1); err == nil {
		t.Error("negative slot should error")
	}
	a, _ := NewSlotIndex(2)
	b, _ := NewSlotIndex(2)
	if !a.Equals(b) {
		t.Error("equal slots should be Equals")
	}
}

func TestNewStartupCommand(t *testing.T) {
	c, err := NewStartupCommand("npm run dev", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Command() != "npm run dev" {
		t.Errorf("Command() = %q", c.Command())
	}
	if !c.AutoRun() {
		t.Error("AutoRun() should be true")
	}
	if _, err := NewStartupCommand("  ", false); err == nil {
		t.Error("blank command should error")
	}
}
