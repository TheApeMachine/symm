package engine

import "sync"

/*
SymbolLock embeds a per-symbol mutex for track stores that shard map access.
*/
type SymbolLock struct {
	mu sync.Mutex
}

/*
Lock acquires the per-symbol mutex.
*/
func (symbolLock *SymbolLock) Lock() {
	symbolLock.mu.Lock()
}

/*
Unlock releases the per-symbol mutex.
*/
func (symbolLock *SymbolLock) Unlock() {
	symbolLock.mu.Unlock()
}

/*
ShardedStore protects a track-store symbol map during structural changes.
*/
type ShardedStore struct {
	mapMu sync.RWMutex
}

/*
LockMap serializes map insertions.
*/
func (shardedStore *ShardedStore) LockMap() {
	shardedStore.mapMu.Lock()
}

/*
UnlockMap releases the map insertion lock.
*/
func (shardedStore *ShardedStore) UnlockMap() {
	shardedStore.mapMu.Unlock()
}

/*
RLockMap shares read access to the symbol map.
*/
func (shardedStore *ShardedStore) RLockMap() {
	shardedStore.mapMu.RLock()
}

/*
RUnlockMap releases the shared map read lock.
*/
func (shardedStore *ShardedStore) RUnlockMap() {
	shardedStore.mapMu.RUnlock()
}
