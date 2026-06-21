package session

import (
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// Registry holds live TerminalSession instances keyed by their ID.
// All methods are safe for concurrent use.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]port.TerminalSession
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]port.TerminalSession),
	}
}

// Add registers s under the given id, replacing any existing entry (without closing it).
func (r *Registry) Add(id string, s port.TerminalSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = s
}

// Get returns the session registered under id, or (nil, false) if not found.
func (r *Registry) Get(id string) (port.TerminalSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}

// Remove deletes the session registered under id. If id is not registered, it is a no-op.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// IDs returns a snapshot of all currently registered session IDs.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	return ids
}

// CloseAll closes every registered session and clears the registry.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = make(map[string]port.TerminalSession)
	r.mu.Unlock()

	for _, s := range sessions {
		_ = s.Close()
	}
}
