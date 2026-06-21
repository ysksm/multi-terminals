package session_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// newHub wraps a FakeTerminalSession in a Session hub for registry tests.
func newHub(id string) (*session.Session, *apptest.FakeTerminalSession) {
	fake := apptest.NewFakeTerminalSession(id)
	return session.NewSession(fake), fake
}

func TestRegistry_AddGetRemove(t *testing.T) {
	r := session.NewRegistry()

	s, _ := newHub("sess-1")
	r.Add("sess-1", s)

	got, ok := r.Get("sess-1")
	if !ok {
		t.Fatal("expected to find session after Add")
	}
	if got.ID() != "sess-1" {
		t.Errorf("expected ID 'sess-1', got %q", got.ID())
	}

	r.Remove("sess-1")
	_, ok = r.Get("sess-1")
	if ok {
		t.Fatal("expected session to be absent after Remove")
	}
}

func TestRegistry_IDs(t *testing.T) {
	r := session.NewRegistry()
	sa, _ := newHub("a")
	sb, _ := newHub("b")
	r.Add("a", sa)
	r.Add("b", sb)

	ids := r.IDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["a"] || !found["b"] {
		t.Errorf("unexpected IDs: %v", ids)
	}
}

func TestRegistry_CloseAll(t *testing.T) {
	r := session.NewRegistry()
	s1, _ := newHub("s1")
	s2, _ := newHub("s2")
	r.Add("s1", s1)
	r.Add("s2", s2)

	r.CloseAll()

	if ids := r.IDs(); len(ids) != 0 {
		t.Errorf("expected empty registry after CloseAll, got %v", ids)
	}
	// hub Done channels should close once CloseAll triggers inner session close.
	for _, hub := range []*session.Session{s1, s2} {
		select {
		case <-hub.Done():
			// ok
		case <-time.After(3 * time.Second):
			t.Error("hub.Done() did not close after CloseAll")
		}
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := session.NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected false for nonexistent session")
	}
}

func TestRegistry_RemoveNonexistent(t *testing.T) {
	r := session.NewRegistry()
	// Should not panic
	r.Remove("nonexistent")
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := session.NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := string(rune('a' + i%26))
			s, _ := newHub(id)
			r.Add(id, s)
			r.Get(id)
			r.IDs()
		}()
	}
	wg.Wait()
}
