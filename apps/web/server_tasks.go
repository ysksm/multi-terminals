package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ysksm/multi-terminals/core/application/taskmgmt"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// registerTaskRoutes はタスク管理(Jira 取り込み / テンプレート / ベースクローン /
// セットアップ実行 / 設定)のエンドポイントを登録する。Deps.Tasks が nil なら何もしない。
func (d Deps) registerTaskRoutes(mux *http.ServeMux) {
	if d.Tasks == nil {
		return
	}
	// タスク
	mux.HandleFunc("GET /api/tasks", d.handleListTasks)
	mux.HandleFunc("POST /api/tasks", d.handleCreateLocalTask)
	mux.HandleFunc("GET /api/tasks/next-key", d.handleNextLocalKey)
	mux.HandleFunc("POST /api/tasks/preview", d.handlePreviewTasks)
	mux.HandleFunc("POST /api/tasks/import", d.handleImportTasks)
	mux.HandleFunc("POST /api/tasks/sync", d.handleSyncAllTasks)
	mux.HandleFunc("GET /api/tasks/by-key/{key}", d.handleGetTaskByKey)
	mux.HandleFunc("GET /api/tasks/{id}", d.handleGetTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", d.handlePatchTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", d.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/sync", d.handleSyncTask)
	mux.HandleFunc("POST /api/tasks/{id}/link-jira", d.handleLinkJira)
	mux.HandleFunc("POST /api/tasks/{id}/setup", d.handleStartSetup)
	mux.HandleFunc("POST /api/tasks/{id}/workdir/delete", d.handleDeleteWorkDir)

	// セットアップ実行
	mux.HandleFunc("GET /api/setup-runs/{id}", d.handleGetSetupRun)
	mux.HandleFunc("GET /api/setup-runs/{id}/stream", d.handleSetupRunStream)
	mux.HandleFunc("POST /api/setup-runs/{id}/retry", d.handleRetrySetupRun)
	mux.HandleFunc("POST /api/setup-runs/{id}/skip", d.handleSkipSetupStep)
	mux.HandleFunc("POST /api/setup-runs/{id}/abort", d.handleAbortSetupRun)

	// 環境テンプレート
	mux.HandleFunc("GET /api/templates", d.handleListTemplates)
	mux.HandleFunc("POST /api/templates", d.handleCreateTemplate)
	mux.HandleFunc("GET /api/templates/{id}", d.handleGetTemplate)
	mux.HandleFunc("PUT /api/templates/{id}", d.handleUpdateTemplate)
	mux.HandleFunc("DELETE /api/templates/{id}", d.handleDeleteTemplate)

	// ベースクローン
	mux.HandleFunc("GET /api/base-clones", d.handleListBaseClones)
	mux.HandleFunc("POST /api/base-clones", d.handleCreateBaseClone)
	mux.HandleFunc("POST /api/base-clones/update", d.handleUpdateAllBaseClones)
	mux.HandleFunc("POST /api/base-clones/{id}/update", d.handleUpdateBaseClone)
	mux.HandleFunc("DELETE /api/base-clones/{id}", d.handleDeleteBaseClone)

	// 設定
	mux.HandleFunc("GET /api/task-settings", d.handleGetTaskSettings)
	mux.HandleFunc("PUT /api/task-settings", d.handlePutTaskSettings)
	mux.HandleFunc("GET /api/jira/config", d.handleGetJiraConfig)
	mux.HandleFunc("PUT /api/jira/config", d.handlePutJiraConfig)
	mux.HandleFunc("POST /api/jira/test", d.handleTestJira)
}

// ---- タスク ----

func (d Deps) handleListTasks(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.List(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// keysBody は番号列の入力。keys(配列)か text(自由入力)のどちらかで受ける。
type keysBody struct {
	Keys       []string `json:"keys"`
	Text       string   `json:"text"`
	TemplateID string   `json:"templateId"`
	Setup      bool     `json:"setup"`
}

func (b keysBody) keys() []string {
	if len(b.Keys) > 0 {
		return b.Keys
	}
	return taskmgmt.ParseJiraKeys(b.Text)
}

func (d Deps) handlePreviewTasks(w http.ResponseWriter, r *http.Request) {
	var body keysBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.Preview(r.Context(), body.keys(), body.TemplateID)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d Deps) handleImportTasks(w http.ResponseWriter, r *http.Request) {
	var body keysBody
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.Import(r.Context(), body.keys(), body.TemplateID, body.Setup)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d Deps) handleSyncAllTasks(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.SyncAll(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleGetTask(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleGetTaskByKey(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.FindByKey(r.Context(), r.PathValue("key"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handlePatchTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note       *string          `json:"note"`
		Env        *task.Env        `json:"env"`
		LocalState *string          `json:"localState"`
		Local      *task.LocalInput `json:"local"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.Update(r.Context(), r.PathValue("id"), taskmgmt.UpdateInput{
		Note: body.Note, Env: body.Env, LocalState: body.LocalState, Local: body.Local,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if err := d.Tasks.Delete(r.Context(), r.PathValue("id")); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleSyncTask(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.Sync(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleStartSetup(w http.ResponseWriter, r *http.Request) {
	runID, err := d.Tasks.StartSetup(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
}

func (d Deps) handleDeleteWorkDir(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.DeleteWorkDir(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- ローカルタスク(Jira に依存しない手動作成) ----

func (d Deps) handleCreateLocalTask(w http.ResponseWriter, r *http.Request) {
	var body taskmgmt.LocalCreateInput
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.CreateLocal(r.Context(), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d Deps) handleNextLocalKey(w http.ResponseWriter, r *http.Request) {
	key, err := d.Tasks.NextLocalKey(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key})
}

func (d Deps) handleLinkJira(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.LinkJira(r.Context(), r.PathValue("id"), body.Key)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- セットアップ実行 ----

func (d Deps) handleGetSetupRun(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.GetSetupRun(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetupRunStream は実行の進捗を SSE で配信する。接続直後に現在値を送り、
// 以後は更新のたびに送る。終端状態を送ったら閉じる。
func (d Deps) handleSetupRunStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusNotImplemented)
		return
	}
	snap, ch, cancel, err := d.Tasks.SubscribeSetupRun(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	send := func(run *task.SetupRun) bool {
		b, err := json.Marshal(run)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		f.Flush()
		return true
	}
	terminal := func(run *task.SetupRun) bool {
		switch run.State {
		case task.RunDone, task.RunFailed, task.RunAborted:
			return true
		}
		return false
	}
	if !send(snap) || terminal(snap) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case run := <-ch:
			if !send(run) || terminal(run) {
				return
			}
		}
	}
}

func (d Deps) handleRetrySetupRun(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.RetrySetupRun(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (d Deps) handleSkipSetupStep(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.SkipSetupStep(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (d Deps) handleAbortSetupRun(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.AbortSetupRun(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- 環境テンプレート ----

func (d Deps) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.ListTemplates(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.GetTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body taskmgmt.TemplateInput
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.CreateTemplate(r.Context(), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d Deps) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	var body taskmgmt.TemplateInput
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.UpdateTemplate(r.Context(), r.PathValue("id"), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if err := d.Tasks.DeleteTemplate(r.Context(), r.PathValue("id")); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- ベースクローン ----

func (d Deps) handleListBaseClones(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.ListBaseClones(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleCreateBaseClone(w http.ResponseWriter, r *http.Request) {
	var body taskmgmt.BaseCloneInput
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.CreateBaseClone(r.Context(), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d Deps) handleUpdateAllBaseClones(w http.ResponseWriter, r *http.Request) {
	out, errs, err := d.Tasks.UpdateAllBaseClones(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"baseClones": out, "errors": errs})
}

func (d Deps) handleUpdateBaseClone(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.UpdateBaseClone(r.Context(), r.PathValue("id"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleDeleteBaseClone(w http.ResponseWriter, r *http.Request) {
	removeFiles := r.URL.Query().Get("files") == "1"
	if err := d.Tasks.DeleteBaseClone(r.Context(), r.PathValue("id"), removeFiles); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- 設定 ----

func (d Deps) handleGetTaskSettings(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.GetSettings(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handlePutTaskSettings(w http.ResponseWriter, r *http.Request) {
	var body task.Settings
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.PutSettings(r.Context(), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleGetJiraConfig(w http.ResponseWriter, r *http.Request) {
	out, err := d.Tasks.GetJiraConfig(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handlePutJiraConfig(w http.ResponseWriter, r *http.Request) {
	var body taskmgmt.JiraConfigInput
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out, err := d.Tasks.PutJiraConfig(r.Context(), body)
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleTestJira(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, d.Tasks.TestJira(r.Context()))
}
