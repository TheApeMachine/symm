package crosssection

import (
	"sync"

	"github.com/theapemachine/symm/nomagique/core"
)

type MemberStore struct {
	*core.PrimitiveError
	mu      sync.RWMutex
	changes map[string]float64
}

func NewMemberStore() *MemberStore {
	return &MemberStore{
		PrimitiveError: core.NewPrimitiveError(),
		changes:        make(map[string]float64),
	}
}

func (store *MemberStore) Set(member string, change float64) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.changes[member] = change
}

func (store *MemberStore) Snapshot() map[string]float64 {
	store.mu.RLock()
	defer store.mu.RUnlock()

	snapshot := make(map[string]float64, len(store.changes))

	for key, val := range store.changes {
		snapshot[key] = val
	}

	return snapshot
}
