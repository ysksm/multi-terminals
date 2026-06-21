package apptest

import (
	"context"
	"fmt"
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// コンパイル時インターフェース適合確認
var _ port.IDGenerator = (*FakeIDGen)(nil)
var _ domain.WorkspaceRepository = (*FakeRepo)(nil)

// FakeIDGen は port.IDGenerator のテスト用実装。
// 事前に登録した固定 ID 列を順に返し、尽きたら連番 "id-N" を生成する。
type FakeIDGen struct {
	mu      sync.Mutex
	ids     []string
	pos     int
	counter int
}

// NewFakeIDGen は固定 ID 列を持つ FakeIDGen を返す。
func NewFakeIDGen(ids ...string) *FakeIDGen {
	return &FakeIDGen{ids: ids}
}

// NewID は次の固定 ID、または連番 "id-N" を返す。
func (f *FakeIDGen) NewID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pos < len(f.ids) {
		id := f.ids[f.pos]
		f.pos++
		return id
	}
	f.counter++
	return fmt.Sprintf("id-%d", f.counter)
}

// FakeRepo は domain.WorkspaceRepository のインメモリ実装。
type FakeRepo struct {
	mu            sync.RWMutex
	store         map[string]*domain.Workspace
	SaveCallCount int
	LastSavedID   string
}

// NewFakeRepo は空の FakeRepo を返す。
func NewFakeRepo() *FakeRepo {
	return &FakeRepo{store: make(map[string]*domain.Workspace)}
}

func (r *FakeRepo) Save(_ context.Context, w *domain.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[w.ID().String()] = w
	r.SaveCallCount++
	r.LastSavedID = w.ID().String()
	return nil
}

func (r *FakeRepo) FindByID(_ context.Context, id domain.WorkspaceId) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.store[id.String()]
	if !ok {
		return nil, domain.ErrWorkspaceNotFound
	}
	return w, nil
}

func (r *FakeRepo) List(_ context.Context) ([]*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.Workspace, 0, len(r.store))
	for _, w := range r.store {
		result = append(result, w)
	}
	return result, nil
}

func (r *FakeRepo) Delete(_ context.Context, id domain.WorkspaceId) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[id.String()]; !ok {
		return domain.ErrWorkspaceNotFound
	}
	delete(r.store, id.String())
	return nil
}
