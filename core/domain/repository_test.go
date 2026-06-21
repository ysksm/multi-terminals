package domain

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo は WorkspaceRepository を満たすことをコンパイル時に検証するための最小実装。
type fakeRepo struct {
	store map[string]*Workspace
}

func newFakeRepo() *fakeRepo { return &fakeRepo{store: map[string]*Workspace{}} }

func (r *fakeRepo) Save(_ context.Context, w *Workspace) error {
	r.store[w.ID().String()] = w
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id WorkspaceId) (*Workspace, error) {
	w, ok := r.store[id.String()]
	if !ok {
		return nil, ErrWorkspaceNotFound
	}
	return w, nil
}

func (r *fakeRepo) List(_ context.Context) ([]*Workspace, error) {
	out := make([]*Workspace, 0, len(r.store))
	for _, w := range r.store {
		out = append(out, w)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id WorkspaceId) error {
	if _, ok := r.store[id.String()]; !ok {
		return ErrWorkspaceNotFound
	}
	delete(r.store, id.String())
	return nil
}

// コンパイル時アサーション
var _ WorkspaceRepository = (*fakeRepo)(nil)

func TestWorkspaceRepositoryContract(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()

	id, _ := NewWorkspaceId("ws1")
	name, _ := NewWorkspaceName("WS")
	w, _ := NewWorkspace(id, name, LayoutSingle)

	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.ID().Equals(id) {
		t.Error("FindByID returned wrong workspace")
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, id); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
