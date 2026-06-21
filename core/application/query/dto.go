package query

import (
	"sort"

	"github.com/ysksm/multi-terminals/core/domain"
)

// StartupCommandDTO は StartupCommand の読み取り DTO。
type StartupCommandDTO struct {
	Command string
	AutoRun bool
}

// PaneDTO は Pane の読み取り DTO。
type PaneDTO struct {
	ID        string
	Directory string
	Slot      int
	Commands  []StartupCommandDTO
}

// WorkspaceDTO は Workspace の読み取り DTO。
type WorkspaceDTO struct {
	ID               string
	Name             string
	Layout           string
	Panes            []PaneDTO
	LastActivePaneID *string
	MaximizedPaneID  *string
}

// toWorkspaceDTO は domain.Workspace を WorkspaceDTO に変換する。
// Panes は SlotIndex 昇順に並ぶ。LastActivePaneID / MaximizedPaneID は設定ありのとき *string、なければ nil。
func toWorkspaceDTO(w *domain.Workspace) WorkspaceDTO {
	rawPanes := w.Panes()

	// SlotIndex 昇順にソート
	sort.Slice(rawPanes, func(i, j int) bool {
		return rawPanes[i].Slot().Int() < rawPanes[j].Slot().Int()
	})

	panes := make([]PaneDTO, len(rawPanes))
	for i, p := range rawPanes {
		cmds := p.Commands()
		dtoCommands := make([]StartupCommandDTO, len(cmds))
		for j, c := range cmds {
			dtoCommands[j] = StartupCommandDTO{
				Command: c.Command(),
				AutoRun: c.AutoRun(),
			}
		}
		panes[i] = PaneDTO{
			ID:        p.ID().String(),
			Directory: p.Directory().String(),
			Slot:      p.Slot().Int(),
			Commands:  dtoCommands,
		}
	}

	dto := WorkspaceDTO{
		ID:     w.ID().String(),
		Name:   w.Name().String(),
		Layout: string(w.Layout()),
		Panes:  panes,
	}

	if id, ok := w.LastActivePaneId(); ok {
		s := id.String()
		dto.LastActivePaneID = &s
	}

	if id, ok := w.MaximizedPaneId(); ok {
		s := id.String()
		dto.MaximizedPaneID = &s
	}

	return dto
}
