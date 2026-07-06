package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
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
func (id WorkspaceId) IsZero() bool              { return id.value == "" }
func (id WorkspaceId) Equals(o WorkspaceId) bool { return id.value == o.value }

// PaneId は pane の一意識別子。
type PaneId struct{ value string }

func NewPaneId(value string) (PaneId, error) {
	if strings.TrimSpace(value) == "" {
		return PaneId{}, errors.New("pane id must not be empty")
	}
	return PaneId{value: value}, nil
}

func (id PaneId) String() string       { return id.value }
func (id PaneId) IsZero() bool         { return id.value == "" }
func (id PaneId) Equals(o PaneId) bool { return id.value == o.value }

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

func (s SlotIndex) Int() int                { return s.value }
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

// RemoteHost はペインのターミナルを実行するリモートホスト（例: "192.168.1.10:8080"、
// "https://host.example:8443"）。空は「ローカル実行」を表す。
type RemoteHost struct{ value string }

// NewRemoteHost は前後空白をトリムして RemoteHost を生成する。
// 空は許容（ローカル実行）。内部の空白・制御文字を含む場合はエラー。
func NewRemoteHost(value string) (RemoteHost, error) {
	v := strings.TrimSpace(value)
	for _, r := range v {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return RemoteHost{}, errors.New("remote host must not contain whitespace or control characters")
		}
	}
	return RemoteHost{value: v}, nil
}

func (h RemoteHost) String() string { return h.value }
func (h RemoteHost) IsZero() bool   { return h.value == "" }

// MaxPaneTitleLen は PaneTitle の最大長（ルーン単位）。
const MaxPaneTitleLen = 100

// PaneTitle はペインの表示名。空は「未設定」を表す。
type PaneTitle struct{ value string }

// NewPaneTitle は前後空白をトリムして PaneTitle を生成する。
// 空は許容（未設定）。最大長超過・制御文字を含む場合はエラー。
func NewPaneTitle(value string) (PaneTitle, error) {
	v := strings.TrimSpace(value)
	if utf8.RuneCountInString(v) > MaxPaneTitleLen {
		return PaneTitle{}, fmt.Errorf("pane title must be at most %d characters", MaxPaneTitleLen)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return PaneTitle{}, errors.New("pane title must not contain control characters")
		}
	}
	return PaneTitle{value: v}, nil
}

func (t PaneTitle) String() string { return t.value }
func (t PaneTitle) IsZero() bool   { return t.value == "" }
