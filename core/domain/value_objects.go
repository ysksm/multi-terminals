package domain

import (
	"errors"
	"strings"
)

// WorkspaceId はワークスペースの一意識別子。
type WorkspaceId struct{ value string }

func NewWorkspaceId(value string) (WorkspaceId, error) {
	if strings.TrimSpace(value) == "" {
		return WorkspaceId{}, errors.New("workspace id must not be empty")
	}
	return WorkspaceId{value: value}, nil
}

func (id WorkspaceId) String() string            { return id.value }
func (id WorkspaceId) IsZero() bool               { return id.value == "" }
func (id WorkspaceId) Equals(o WorkspaceId) bool  { return id.value == o.value }

// PaneId は pane の一意識別子。
type PaneId struct{ value string }

func NewPaneId(value string) (PaneId, error) {
	if strings.TrimSpace(value) == "" {
		return PaneId{}, errors.New("pane id must not be empty")
	}
	return PaneId{value: value}, nil
}

func (id PaneId) String() string        { return id.value }
func (id PaneId) IsZero() bool           { return id.value == "" }
func (id PaneId) Equals(o PaneId) bool   { return id.value == o.value }

// WorkspaceName はワークスペースの表示名（非空）。
type WorkspaceName struct{ value string }

func NewWorkspaceName(value string) (WorkspaceName, error) {
	if strings.TrimSpace(value) == "" {
		return WorkspaceName{}, errors.New("workspace name must not be empty")
	}
	return WorkspaceName{value: value}, nil
}

func (n WorkspaceName) String() string { return n.value }

// DirectoryPath は pane の作業ディレクトリ（非空。存在確認はアプリ層の責務）。
type DirectoryPath struct{ value string }

func NewDirectoryPath(value string) (DirectoryPath, error) {
	if strings.TrimSpace(value) == "" {
		return DirectoryPath{}, errors.New("directory path must not be empty")
	}
	return DirectoryPath{value: value}, nil
}

func (p DirectoryPath) String() string { return p.value }

// SlotIndex はレイアウト内の位置（0 始まり）。
type SlotIndex struct{ value int }

func NewSlotIndex(value int) (SlotIndex, error) {
	if value < 0 {
		return SlotIndex{}, errors.New("slot index must be >= 0")
	}
	return SlotIndex{value: value}, nil
}

func (s SlotIndex) Int() int             { return s.value }
func (s SlotIndex) Equals(o SlotIndex) bool { return s.value == o.value }

// StartupCommand は pane を開いたときに実行する候補コマンドと自動実行フラグ。
type StartupCommand struct {
	command string
	autoRun bool
}

func NewStartupCommand(command string, autoRun bool) (StartupCommand, error) {
	if strings.TrimSpace(command) == "" {
		return StartupCommand{}, errors.New("startup command must not be empty")
	}
	return StartupCommand{command: command, autoRun: autoRun}, nil
}

func (c StartupCommand) Command() string { return c.command }
func (c StartupCommand) AutoRun() bool   { return c.autoRun }
