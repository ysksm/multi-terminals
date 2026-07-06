package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/infrastructure/gitcli"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"
	"github.com/ysksm/multi-terminals/core/infrastructure/remoteterm"
	"github.com/ysksm/multi-terminals/core/infrastructure/sysopen"
	"github.com/ysksm/multi-terminals/core/infrastructure/terminal"
)

// RemoteTokenEnv is the environment variable holding the shared secret for
// remote terminal execution. When set, this instance (1) serves the
// remote-terminal endpoint so other instances can run terminals on this
// machine, and (2) uses the same token when connecting to other instances.
// When unset, the endpoint rejects all connections and only local panes work.
const RemoteTokenEnv = "MULTI_TERMINALS_REMOTE_TOKEN"

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
	reg := session.NewRegistry()
	idgen := &uuidIDGen{}
	git := gitcli.New()

	// Terminal runners: local PTY plus remote dial-out, dispatched per pane by
	// RemoteHost. The remote-terminal endpoint always executes with the local
	// runner — a serving instance is the execution target, never a relay.
	remoteToken := os.Getenv(RemoteTokenEnv)
	localRunner := terminal.NewRunner()
	runner := remoteterm.NewDispatchRunner(localRunner, remoteterm.NewRunner(remoteToken))

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
		SetTitle:        command.NewSetPaneTitleHandler(repo),
		SetRemoteHost:   command.NewSetPaneRemoteHostHandler(repo),
		SetCmds:         command.NewSetPaneStartupCommandsHandler(repo),
		OpenIn:          command.NewOpenPaneInHandler(repo, sysopen.New(), git),
		CloneRepo:       command.NewCloneRepositoryHandler(git),
		GetPaneGit:      query.NewGetPaneGitInfoHandler(repo, git),
		Open:            command.NewOpenWorkspaceHandler(repo, runner, reg, state, ""),
		Write:           command.NewWriteToPaneHandler(reg),
		Resize:          command.NewResizePaneHandler(reg),
		ClosePane:       command.NewClosePaneHandler(reg),
		DeleteWorkspace: command.NewDeleteWorkspaceHandler(repo, reg),
		Registry:        reg,
		RemoteTerminal:  remoteterm.Handler(localRunner, remoteToken),
	}, nil
}
