package domain

import "errors"

// Pane はワークスペース内の 1 つのターミナル枠を表すエンティティ。
type Pane struct {
	id        PaneId
	directory DirectoryPath
	slot      SlotIndex
	commands  []StartupCommand
}

// NewPane は Pane を生成する。commands は防御的にコピーされる。
func NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, commands []StartupCommand) (*Pane, error) {
	if id.IsZero() {
		return nil, errors.New("pane id must not be empty")
	}
	return &Pane{
		id:        id,
		directory: directory,
		slot:      slot,
		commands:  append([]StartupCommand(nil), commands...),
	}, nil
}

func (p *Pane) ID() PaneId               { return p.id }
func (p *Pane) Directory() DirectoryPath { return p.directory }
func (p *Pane) Slot() SlotIndex          { return p.slot }

// Commands は内部スライスの防御的コピーを返す。
func (p *Pane) Commands() []StartupCommand {
	return append([]StartupCommand(nil), p.commands...)
}

func (p *Pane) setDirectory(d DirectoryPath) { p.directory = d }

func (p *Pane) setCommands(c []StartupCommand) {
	p.commands = append([]StartupCommand(nil), c...)
}
