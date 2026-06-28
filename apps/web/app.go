package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"
	"github.com/ysksm/multi-terminals/core/infrastructure/terminal"
)

// uuidIDGen is a local port.IDGenerator implementation that produces
// collision-resistant IDs using crypto/rand (stdlib only).
type uuidIDGen struct{}

// Compile-time interface assertion.
var _ port.IDGenerator = (*uuidIDGen)(nil)

// NewID returns a 32-character lowercase hex string derived from 16 random bytes.
func (g *uuidIDGen) NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic; panic is acceptable here.
		panic(fmt.Sprintf("uuidIDGen: crypto/rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

// BuildDeps wires all infrastructure implementations and CQRS handlers into a
// Deps value ready for NewMux. baseDir is the base directory for jsonstore files.
func BuildDeps(baseDir string) (Deps, error) {
	repo, err := jsonstore.NewWorkspaceRepository(baseDir)
	if err != nil {
		return Deps{}, fmt.Errorf("BuildDeps: workspace repository: %w", err)
	}

	state := jsonstore.NewAppStateStore(baseDir)
	runner := terminal.NewRunner()
	reg := session.NewRegistry()
	idgen := &uuidIDGen{}

	return Deps{
		Create:          command.NewCreateWorkspaceHandler(repo, idgen),
		Rename:          command.NewRenameWorkspaceHandler(repo),
		ChangeLayout:    command.NewChangeLayoutHandler(repo),
		Maximize:        command.NewMaximizePaneHandler(repo),
		Restore:         command.NewRestoreLayoutHandler(repo),
		SetLastActive:   command.NewSetLastActivePaneHandler(repo),
		Get:             query.NewGetWorkspaceHandler(repo),
		List:            query.NewListWorkspacesHandler(repo),
		GetLastOpened:   query.NewGetLastOpenedWorkspaceHandler(state, repo),
		AddPane:         command.NewAddPaneHandler(repo, idgen),
		RemovePane:      command.NewRemovePaneHandler(repo),
		SetDir:          command.NewSetPaneDirectoryHandler(repo),
		SetCmds:         command.NewSetPaneStartupCommandsHandler(repo),
		Open:            command.NewOpenWorkspaceHandler(repo, runner, reg, state, ""),
		Write:           command.NewWriteToPaneHandler(reg),
		Resize:          command.NewResizePaneHandler(reg),
		ClosePane:       command.NewClosePaneHandler(reg),
		DeleteWorkspace: command.NewDeleteWorkspaceHandler(repo, reg),
		Registry:        reg,
	}, nil
}
