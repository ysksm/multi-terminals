package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// handleAgentStatus は現在のエージェント稼働状況スナップショットを返す。
// SSE が使えない環境(ポーリングフォールバック)向け。
func (d Deps) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"panes": d.AgentStatus.Current()})
}

// handleAgentStatusStream は稼働状況を SSE で配信する。接続直後に現在の
// スナップショットを 1 回送り、以後は変化時のみ送る。
func (d Deps) handleAgentStatusStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	snap, ch, cancel := d.AgentStatus.Subscribe()
	defer cancel()

	send := func(s agentstatus.Snapshot) bool {
		b, err := json.Marshal(map[string]any{"panes": s})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		f.Flush()
		return true
	}
	if !send(snap) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case s := <-ch:
			if !send(s) {
				return
			}
		}
	}
}

// registrySource は Registry のライブセッションを agentstatus.Source に
// 適合させる。PID を持たないセッション(リモート等)は対象外。
func registrySource(reg *session.Registry) agentstatus.Source {
	return func() []agentstatus.SessionInfo {
		var out []agentstatus.SessionInfo
		for _, id := range reg.IDs() {
			s, ok := reg.Get(id)
			if !ok {
				continue
			}
			pid := s.Pid()
			if pid <= 0 {
				continue
			}
			out = append(out, agentstatus.SessionInfo{
				PaneID:     id,
				Pid:        pid,
				Tail:       s.Tail(agentstatus.TailBytes),
				LastOutput: s.LastOutputAt(),
			})
		}
		return out
	}
}
