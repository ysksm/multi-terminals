package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/application/taskmgmt"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/infrastructure/gitcli"
	"github.com/ysksm/multi-terminals/core/infrastructure/jirahttp"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"
	"github.com/ysksm/multi-terminals/core/infrastructure/procscan"
	"github.com/ysksm/multi-terminals/core/infrastructure/remoteterm"
	"github.com/ysksm/multi-terminals/core/infrastructure/setupexec"
	"github.com/ysksm/multi-terminals/core/infrastructure/sysopen"
	"github.com/ysksm/multi-terminals/core/infrastructure/terminal"
	"github.com/ysksm/multi-terminals/core/infrastructure/vtscreen"
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
	reg := session.NewRegistry()
	idgen := &uuidIDGen{}
	git := gitcli.New()

	// Remote execution key material: the instance's Ed25519 identity (used to
	// authenticate to other instances) is created on demand by the user, not
	// auto-generated on startup, so a fresh install exposes no key until the
	// user asks for one. authKeys is the list of public keys allowed to run
	// terminals on this machine; remote listening stays disabled until at least
	// one key is authorized.
	identityStore, err := remoteterm.NewIdentityStore(baseDir)
	if err != nil {
		return Deps{}, fmt.Errorf("BuildDeps: remote identity: %w", err)
	}
	authKeys := remoteterm.NewAuthorizedKeys(filepath.Join(baseDir, remoteterm.AuthorizedKeysFile))

	// Terminal runners: local PTY plus two remote transports, dispatched per
	// pane by RemoteHost. An "ssh://…" host connects to an ordinary sshd; any
	// other non-empty host connects to another multi-terminals instance's
	// WebSocket endpoint. The remote-terminal endpoint always executes with the
	// local runner — a serving instance is the execution target, never a relay.
	localRunner := terminal.NewRunner()
	runner := remoteterm.NewDispatchRunner(
		localRunner,
		remoteterm.NewRunner(identityStore.Current),
		remoteterm.NewSSHRunner(),
	)

	// エージェント稼働状況(claude/codex)の監視。プロセスのライフサイクルは
	// サーバと同じでよいので Stop は呼ばない。
	watcher := agentstatus.NewWatcher(registrySource(reg), procscan.Snapshot, 0)
	openHandler := command.NewOpenWorkspaceHandler(repo, runner, reg, state, "")
	openHandler.SetScreenFactory(vtscreen.Factory)
	watcher.Start()

	deleteWS := command.NewDeleteWorkspaceHandler(repo, reg)
	tasks, err := buildTaskService(baseDir, repo, idgen, git, reg, deleteWS)
	if err != nil {
		return Deps{}, fmt.Errorf("BuildDeps: task service: %w", err)
	}

	return Deps{
		Create:              command.NewCreateWorkspaceHandler(repo, idgen),
		Rename:              command.NewRenameWorkspaceHandler(repo),
		ChangeLayout:        command.NewChangeLayoutHandler(repo),
		Maximize:            command.NewMaximizePaneHandler(repo),
		Restore:             command.NewRestoreLayoutHandler(repo),
		SetLastActive:       command.NewSetLastActivePaneHandler(repo),
		Get:                 query.NewGetWorkspaceHandler(repo),
		List:                query.NewListWorkspacesHandler(repo),
		GetLastOpened:       query.NewGetLastOpenedWorkspaceHandler(state, repo),
		AddPane:             command.NewAddPaneHandler(repo, idgen),
		RemovePane:          command.NewRemovePaneHandler(repo),
		SetDir:              command.NewSetPaneDirectoryHandler(repo),
		SetTitle:            command.NewSetPaneTitleHandler(repo),
		SetRemoteHost:       command.NewSetPaneRemoteHostHandler(repo),
		SetCmds:             command.NewSetPaneStartupCommandsHandler(repo),
		OpenIn:              command.NewOpenPaneInHandler(repo, sysopen.New(), git),
		CloneRepo:           command.NewCloneRepositoryHandler(git),
		GetPaneGit:          query.NewGetPaneGitInfoHandler(repo, git),
		GitBranches:         query.NewListPaneBranchesHandler(repo, git),
		GitCheckout:         command.NewCheckoutPaneBranchHandler(repo, git),
		GitOp:               command.NewRunPaneGitOpHandler(repo, git),
		Open:                openHandler,
		Write:               command.NewWriteToPaneHandler(reg),
		Resize:              command.NewResizePaneHandler(reg),
		ClosePane:           command.NewClosePaneHandler(reg),
		DeleteWorkspace:     deleteWS,
		Tasks:               tasks,
		Registry:            reg,
		AgentStatus:         watcher,
		RemoteTerminal:      remoteterm.Handler(localRunner, authKeys),
		RemoteIdentityStore: identityStore,
		RemoteAuthKeys:      authKeys,
	}, nil
}

// buildTaskService はタスク管理サービスを JSON ストア・Jira クライアント・
// セットアップ実行器で組み立てる。
func buildTaskService(
	baseDir string,
	repo domain.WorkspaceRepository,
	idgen port.IDGenerator,
	git port.GitService,
	reg *session.Registry,
	deleteWS *command.DeleteWorkspaceHandler,
) (*taskmgmt.Service, error) {
	taskRepo, err := jsonstore.NewTaskRepository(baseDir)
	if err != nil {
		return nil, err
	}
	tplRepo, err := jsonstore.NewTemplateRepository(baseDir)
	if err != nil {
		return nil, err
	}
	bcRepo, err := jsonstore.NewBaseCloneRepository(baseDir)
	if err != nil {
		return nil, err
	}
	runRepo, err := jsonstore.NewSetupRunRepository(baseDir)
	if err != nil {
		return nil, err
	}
	return taskmgmt.New(taskmgmt.Deps{
		Tasks:       taskRepo,
		Templates:   tplRepo,
		BaseClones:  bcRepo,
		Runs:        runRepo,
		Settings:    jsonstore.NewSettingsStore(baseDir),
		JiraStore:   jsonstore.NewJiraConfigStore(baseDir),
		JiraFactory: jirahttp.Factory,
		Git:         git,
		Exec:        setupexec.New(),
		Workspaces:  repo,
		IDGen:       idgen,
		LivePaneIDs: reg.IDs,
		DeleteWorkspace: func(ctx context.Context, id string) error {
			return deleteWS.Handle(ctx, command.DeleteWorkspaceCommand{WorkspaceID: id})
		},
		BaseDir: baseDir,
	})
}
