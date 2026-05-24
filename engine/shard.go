package engine

import "sync"

/*
ShardedStore isolates per-key mutex contention from map structure mutations.
*/
type ShardedStore struct {
	mapMu sync.RWMutex
}

/*
LockMap locks the shard map for structural changes.
*/
func (store *ShardedStore) LockMap() {
	store.mapMu.Lock()
}

/*
UnlockMap releases a structural map lock.
*/
func (store *ShardedStore) UnlockMap() {
	store.mapMu.Unlock()
}

/*
RLockMap locks the shard map for read-only traversal.
*/
func (store *ShardedStore) RLockMap() {
	store.mapMu.RLock()
}

/*
RUnlockMap releases a read-only map lock.
*/
func (store *ShardedStore) RUnlockMap() {
	store.mapMu.RUnlock()
}

/*
SymbolLock guards one symbol's mutable track state.
*/
type SymbolLock struct {
	mu sync.Mutex
}

/*
Lock acquires exclusive access to one symbol track.
*/
func (symbolLock *SymbolLock) Lock() {
	symbolLock.mu.Lock()
}

/*
Unlock releases exclusive access to one symbol track.
*/
func (symbolLock *SymbolLock) Unlock() {
	symbolLock.mu.Unlock()
}
