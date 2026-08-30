package advisor

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

/*
Store is the bounded latest-by-key Perspective store that lets PositionRisk
read current descriptive context without waiting for analytics and without any
event backlog or channel. It holds, per PerspectiveKey, the single latest valid
Perspective: latest sequence wins, an errored Perspective never replaces a
valid latest state, and reads never block (a brief shared lock guards each key).

This is the delivery mechanism strategy/ADVISORS.md specifies for Perspectives:
current context, superseded in place, locally readable by PositionRisk so risk
protection never waits for analytics (§9, §19.1).
*/
type Store struct {
	mu          sync.RWMutex
	perspectives map[types.PerspectiveKey]types.Perspective
}

/*
NewStore constructs an empty latest-by-key Perspective store.
*/
func NewStore() *Store {
	return &Store{
		perspectives: make(map[types.PerspectiveKey]types.Perspective),
	}
}

/*
Put records one Perspective under its key. Latest sequence wins; an errored
Perspective is discarded rather than replacing a valid latest state. The value
is copied in, so later mutation of the caller's Perspective cannot corrupt the
stored state.
*/
func (store *Store) Put(perspective *types.Perspective) {
	if store == nil || perspective == nil || perspective.Err != nil {
		return
	}

	key := perspective.Key()

	store.mu.Lock()
	defer store.mu.Unlock()

	current, found := store.perspectives[key]

	if found && current.Sequence > perspective.Sequence {
		return
	}

	store.perspectives[key] = *perspective
}

/*
Latest returns the current Perspective for a key and whether one is held. It
never blocks beyond the brief read lock and never returns a stale errored state
(errored states are never stored).
*/
func (store *Store) Latest(key types.PerspectiveKey) (types.Perspective, bool) {
	if store == nil {
		return types.Perspective{}, false
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	perspective, found := store.perspectives[key]

	return perspective, found
}
