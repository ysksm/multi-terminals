package domain

import "testing"

func mustPaneId(t *testing.T, v string) PaneId {
	t.Helper()
	id, err := NewPaneId(v)
	if err != nil {
		t.Fatalf("NewPaneId(%q): %v", v, err)
	}
	return id
}

func mustDir(t *testing.T, v string) DirectoryPath {
	t.Helper()
	d, err := NewDirectoryPath(v)
	if err != nil {
		t.Fatalf("NewDirectoryPath(%q): %v", v, err)
	}
	return d
}

func mustSlot(t *testing.T, v int) SlotIndex {
	t.Helper()
	s, err := NewSlotIndex(v)
	if err != nil {
		t.Fatalf("NewSlotIndex(%d): %v", v, err)
	}
	return s
}

func TestNewPane(t *testing.T) {
	cmd, _ := NewStartupCommand("ls", false)
	p, err := NewPane(mustPaneId(t, "p1"), mustDir(t, "/tmp"), mustSlot(t, 0), PaneTitle{}, RemoteHost{}, []StartupCommand{cmd})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.ID().Equals(mustPaneId(t, "p1")) {
		t.Error("ID mismatch")
	}
	if p.Directory().String() != "/tmp" {
		t.Errorf("Directory = %q", p.Directory().String())
	}
	if p.Slot().Int() != 0 {
		t.Errorf("Slot = %d", p.Slot().Int())
	}
	if len(p.Commands()) != 1 || p.Commands()[0].Command() != "ls" {
		t.Errorf("Commands = %#v", p.Commands())
	}
}

func TestNewPaneRejectsZeroId(t *testing.T) {
	var zero PaneId
	if _, err := NewPane(zero, mustDir(t, "/tmp"), mustSlot(t, 0), PaneTitle{}, RemoteHost{}, nil); err == nil {
		t.Error("zero pane id should error")
	}
}

func TestPaneCommandsIsDefensiveCopy(t *testing.T) {
	cmd, _ := NewStartupCommand("ls", false)
	src := []StartupCommand{cmd}
	p, _ := NewPane(mustPaneId(t, "p1"), mustDir(t, "/tmp"), mustSlot(t, 0), PaneTitle{}, RemoteHost{}, src)
	// 入力スライスを書き換えても内部状態は不変であること
	other, _ := NewStartupCommand("rm -rf /", true)
	src[0] = other
	if p.Commands()[0].Command() != "ls" {
		t.Error("Pane must not share backing array with input slice")
	}
	// 返り値を書き換えても内部状態は不変であること
	got := p.Commands()
	got[0] = other
	if p.Commands()[0].Command() != "ls" {
		t.Error("Commands() must return a defensive copy")
	}
}
