// Package web provides the HTTP server for multi-terminals.
// It wires CQRS handlers to HTTP endpoints and serves workspace/pane/runtime APIs.
package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/infrastructure/remoteterm"
)

// Deps holds all CQRS handler dependencies for the HTTP server.
type Deps struct {
	Create          *command.CreateWorkspaceHandler
	Rename          *command.RenameWorkspaceHandler
	ChangeLayout    *command.ChangeLayoutHandler
	Maximize        *command.MaximizePaneHandler
	Restore         *command.RestoreLayoutHandler
	SetLastActive   *command.SetLastActivePaneHandler
	Get             *query.GetWorkspaceHandler
	List            *query.ListWorkspacesHandler
	GetLastOpened   *query.GetLastOpenedWorkspaceHandler
	AddPane         *command.AddPaneHandler
	RemovePane      *command.RemovePaneHandler
	SetDir          *command.SetPaneDirectoryHandler
	SetTitle        *command.SetPaneTitleHandler
	SetRemoteHost   *command.SetPaneRemoteHostHandler
	SetCmds         *command.SetPaneStartupCommandsHandler
	OpenIn          *command.OpenPaneInHandler
	CloneRepo       *command.CloneRepositoryHandler
	GetPaneGit      *query.GetPaneGitInfoHandler
	Open            *command.OpenWorkspaceHandler
	Write           *command.WriteToPaneHandler
	Resize          *command.ResizePaneHandler
	ClosePane       *command.ClosePaneHandler
	DeleteWorkspace *command.DeleteWorkspaceHandler
	Registry        *session.Registry
	// RemoteTerminal serves the remote-execution WebSocket endpoint
	// (remoteterm.Handler). Nil disables the endpoint entirely.
	RemoteTerminal http.HandlerFunc
	// RemoteIdentityStore manages this instance's keypair (created on demand by
	// the user, regeneratable, deletable); RemoteAuthKeys is the list of client
	// public keys allowed to run terminals here. Nil disables the corresponding
	// key-management endpoints.
	RemoteIdentityStore *remoteterm.IdentityStore
	RemoteAuthKeys      *remoteterm.AuthorizedKeys
}

// NewMux registers all routes and returns the HTTP mux.
// Routing uses Go 1.22+ method+path patterns.
func NewMux(d Deps) *http.ServeMux {
	mux := http.NewServeMux()

	// Workspace collection
	mux.HandleFunc("GET /api/workspaces", d.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", d.handleCreateWorkspace)

	// Workspace item
	mux.HandleFunc("GET /api/workspaces/{id}", d.handleGetWorkspace)
	mux.HandleFunc("PATCH /api/workspaces/{id}", d.handlePatchWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", d.handleDeleteWorkspace)

	// Workspace actions
	mux.HandleFunc("POST /api/workspaces/{id}/maximize", d.handleMaximize)
	mux.HandleFunc("POST /api/workspaces/{id}/restore", d.handleRestore)
	mux.HandleFunc("POST /api/workspaces/{id}/active-pane", d.handleSetActivePane)
	mux.HandleFunc("POST /api/workspaces/{id}/open", d.handleOpenWorkspace)

	// Pane CRUD
	mux.HandleFunc("POST /api/workspaces/{id}/panes", d.handleAddPane)
	mux.HandleFunc("DELETE /api/workspaces/{id}/panes/{paneId}", d.handleRemovePane)
	mux.HandleFunc("PUT /api/workspaces/{id}/panes/{paneId}/directory", d.handleSetPaneDirectory)
	mux.HandleFunc("PUT /api/workspaces/{id}/panes/{paneId}/title", d.handleSetPaneTitle)
	mux.HandleFunc("PUT /api/workspaces/{id}/panes/{paneId}/remote-host", d.handleSetPaneRemoteHost)
	mux.HandleFunc("PUT /api/workspaces/{id}/panes/{paneId}/commands", d.handleSetPaneCommands)
	mux.HandleFunc("POST /api/workspaces/{id}/panes/{paneId}/open-in", d.handleOpenPaneIn)
	mux.HandleFunc("GET /api/workspaces/{id}/panes/{paneId}/git", d.handleGetPaneGit)

	// Repository actions
	mux.HandleFunc("POST /api/repos/clone", d.handleCloneRepo)

	// Global queries
	mux.HandleFunc("GET /api/last-opened", d.handleGetLastOpened)

	// Session list (live pane IDs for resume)
	mux.HandleFunc("GET /api/sessions", d.handleListSessions)

	// WebSocket pane I/O
	mux.HandleFunc("GET /api/panes/{paneId}/io", d.handlePaneIO)

	// Remote terminal execution (WebSocket, key-authenticated). Other
	// multi-terminals instances connect here to run terminals on this machine.
	if d.RemoteTerminal != nil {
		mux.Handle("GET /api/remote/terminal", d.RemoteTerminal)
	}

	// Remote key management: this instance's own keypair (user-created,
	// regeneratable, deletable) and the list of client keys allowed to run
	// terminals here.
	if d.RemoteIdentityStore != nil {
		mux.HandleFunc("GET /api/remote/identity", d.handleGetRemoteIdentity)
		mux.HandleFunc("POST /api/remote/identity", d.handleCreateRemoteIdentity)
		mux.HandleFunc("POST /api/remote/identity/regenerate", d.handleRegenerateRemoteIdentity)
		mux.HandleFunc("DELETE /api/remote/identity", d.handleDeleteRemoteIdentity)
	}
	if d.RemoteAuthKeys != nil {
		mux.HandleFunc("GET /api/remote/authorized-keys", d.handleListAuthorizedKeys)
		mux.HandleFunc("POST /api/remote/authorized-keys", d.handleAddAuthorizedKey)
		mux.HandleFunc("DELETE /api/remote/authorized-keys", d.handleRemoveAuthorizedKey)
	}

	return mux
}

// ---- JSON helpers ----

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// readJSON decodes the request body into v.
func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// mapErr maps domain/application errors to HTTP status codes and writes a JSON error body.
func mapErr(w http.ResponseWriter, err error) {
	var ve *apperr.ValidationError
	switch {
	case errors.Is(err, domain.ErrWorkspaceNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, command.ErrSessionNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

// ---- Workspace handlers ----

func (d Deps) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	dtos, err := d.List.Handle(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dtos)
}

func (d Deps) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Layout string `json:"layout"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := d.Create.Handle(r.Context(), command.CreateWorkspaceCommand{
		Name:   body.Name,
		Layout: body.Layout,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": result.WorkspaceID})
}

func (d Deps) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dto, err := d.Get.Handle(r.Context(), query.GetWorkspaceQuery{WorkspaceID: id})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// patchBody holds optional fields for PATCH /api/workspaces/{id}.
type patchBody struct {
	Name   *string `json:"name"`
	Layout *string `json:"layout"`
}

func (d Deps) handlePatchWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body patchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Name != nil {
		if err := d.Rename.Handle(r.Context(), command.RenameWorkspaceCommand{
			WorkspaceID: id,
			Name:        *body.Name,
		}); err != nil {
			mapErr(w, err)
			return
		}
	}
	if body.Layout != nil {
		if err := d.ChangeLayout.Handle(r.Context(), command.ChangeLayoutCommand{
			WorkspaceID: id,
			Layout:      *body.Layout,
		}); err != nil {
			mapErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleMaximize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		PaneID string `json:"paneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.Maximize.Handle(r.Context(), command.MaximizePaneCommand{
		WorkspaceID: id,
		PaneID:      body.PaneID,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := d.Restore.Handle(r.Context(), command.RestoreLayoutCommand{
		WorkspaceID: id,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSetActivePane(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		PaneID string `json:"paneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.SetLastActive.Handle(r.Context(), command.SetLastActivePaneCommand{
		WorkspaceID: id,
		PaneID:      body.PaneID,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleGetLastOpened(w http.ResponseWriter, r *http.Request) {
	dto, found, err := d.GetLastOpened.Handle(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, map[string]interface{}{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"found":     true,
		"workspace": dto,
	})
}

// ---- Pane handlers ----

func (d Deps) handleAddPane(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Directory  string `json:"directory"`
		Slot       int    `json:"slot"`
		Title      string `json:"title"`
		RemoteHost string `json:"remoteHost"`
		Commands   []struct {
			Command string `json:"command"`
			AutoRun bool   `json:"autoRun"`
		} `json:"commands"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cmds := make([]command.StartupCommandInput, len(body.Commands))
	for i, c := range body.Commands {
		cmds[i] = command.StartupCommandInput{Command: c.Command, AutoRun: c.AutoRun}
	}
	result, err := d.AddPane.Handle(r.Context(), command.AddPaneCommand{
		WorkspaceID: id,
		Directory:   body.Directory,
		Slot:        body.Slot,
		Title:       body.Title,
		RemoteHost:  body.RemoteHost,
		Commands:    cmds,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"paneId": result.PaneID})
}

func (d Deps) handleRemovePane(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	if err := d.RemovePane.Handle(r.Context(), command.RemovePaneCommand{
		WorkspaceID: id,
		PaneID:      paneID,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSetPaneDirectory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Directory string `json:"directory"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.SetDir.Handle(r.Context(), command.SetPaneDirectoryCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Directory:   body.Directory,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSetPaneTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.SetTitle.Handle(r.Context(), command.SetPaneTitleCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Title:       body.Title,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSetPaneRemoteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		RemoteHost string `json:"remoteHost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.SetRemoteHost.Handle(r.Context(), command.SetPaneRemoteHostCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		RemoteHost:  body.RemoteHost,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSetPaneCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Commands []struct {
			Command string `json:"command"`
			AutoRun bool   `json:"autoRun"`
		} `json:"commands"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cmds := make([]command.StartupCommandInput, len(body.Commands))
	for i, c := range body.Commands {
		cmds[i] = command.StartupCommandInput{Command: c.Command, AutoRun: c.AutoRun}
	}
	if err := d.SetCmds.Handle(r.Context(), command.SetPaneStartupCommandsCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Commands:    cmds,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleOpenPaneIn(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.OpenIn.Handle(r.Context(), command.OpenPaneInCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Target:      body.Target,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleGetPaneGit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	dto, err := d.GetPaneGit.Handle(r.Context(), query.GetPaneGitInfoQuery{
		WorkspaceID: id,
		PaneID:      paneID,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// ---- Repository handlers ----

func (d Deps) handleCloneRepo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL  string `json:"url"`
		Dest string `json:"dest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := d.CloneRepo.Handle(r.Context(), command.CloneRepositoryCommand{
		URL:  body.URL,
		Dest: body.Dest,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": result.Path})
}

// ---- Runtime handlers ----

func (d Deps) handleOpenWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := d.Open.Handle(r.Context(), command.OpenWorkspaceCommand{WorkspaceID: id})
	if err != nil {
		mapErr(w, err)
		return
	}
	type openedPaneJSON struct {
		PaneID string `json:"paneId"`
	}
	panes := make([]openedPaneJSON, len(result.Panes))
	for i, p := range result.Panes {
		panes[i] = openedPaneJSON{PaneID: p.PaneID}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"panes": panes})
}

// handleListSessions returns the IDs of all currently live pane sessions.
// The response is always {"paneIds": [...]}, never null.
func (d Deps) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	ids := d.Registry.IDs()
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"paneIds": ids})
}

// ---- Remote key management handlers ----

// identityJSON is the response body for the identity endpoints: exists reports
// whether a key has been created, and the key fields are present only when it
// has. The private key never leaves the server.
func identityJSON(id *remoteterm.Identity, exists bool) map[string]interface{} {
	if !exists || id == nil {
		return map[string]interface{}{"exists": false}
	}
	return map[string]interface{}{
		"exists":      true,
		"publicKey":   id.PublicKeyString(),
		"fingerprint": id.Fingerprint(),
	}
}

// handleGetRemoteIdentity reports whether this instance has a key and, if so,
// returns its public key for pasting into another instance's authorized list.
func (d Deps) handleGetRemoteIdentity(w http.ResponseWriter, _ *http.Request) {
	id, exists := d.RemoteIdentityStore.Current()
	writeJSON(w, http.StatusOK, identityJSON(id, exists))
}

// handleCreateRemoteIdentity creates the instance key on explicit user action.
// It never overwrites an existing key: a second create returns 409 so the user
// must choose regenerate deliberately.
func (d Deps) handleCreateRemoteIdentity(w http.ResponseWriter, _ *http.Request) {
	id, err := d.RemoteIdentityStore.Generate()
	if errors.Is(err, remoteterm.ErrIdentityExists) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a key already exists; regenerate it to replace it"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, identityJSON(id, true))
}

// handleRegenerateRemoteIdentity replaces the key with a fresh one. The public
// key changes, so other machines that authorized the old key must re-authorize.
func (d Deps) handleRegenerateRemoteIdentity(w http.ResponseWriter, _ *http.Request) {
	id, err := d.RemoteIdentityStore.Regenerate()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, identityJSON(id, true))
}

// handleDeleteRemoteIdentity removes the key so the instance has none until one
// is created again.
func (d Deps) handleDeleteRemoteIdentity(w http.ResponseWriter, _ *http.Request) {
	if err := d.RemoteIdentityStore.Delete(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListAuthorizedKeys returns all authorized client keys. The response
// is always {"keys":[...], "enabled":bool}, never null.
func (d Deps) handleListAuthorizedKeys(w http.ResponseWriter, _ *http.Request) {
	keys := d.RemoteAuthKeys.List()
	if keys == nil {
		keys = []remoteterm.AuthorizedKey{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys":    keys,
		"enabled": len(keys) > 0,
	})
}

func (d Deps) handleAddAuthorizedKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key     string `json:"key"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.RemoteAuthKeys.Add(body.Key, body.Comment); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleRemoveAuthorizedKey deletes the key given in the "key" query
// parameter (the key contains base64 characters, so it travels URL-encoded).
func (d Deps) handleRemoveAuthorizedKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key query parameter required"})
		return
	}
	if err := d.RemoteAuthKeys.Remove(key); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := d.DeleteWorkspace.Handle(r.Context(), command.DeleteWorkspaceCommand{WorkspaceID: id}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
