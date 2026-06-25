package logic

import (
	"sync"
	"time"
)

/*
stageMatch records when a branch's condition group last matched for a symbol.
This enables cross-tick sequential evaluation: a parent branch that matched on
a prior tick allows its children to evaluate on the current tick's measurements,
implementing "stage A fired, THEN stage B fired after it" (tree.yml comments).
*/
type stageMatch struct {
	stamp time.Time
}

/*
stageMemory tracks per-symbol stage matches for one branch. The TTL is derived
from the measurement cadence so it adapts rather than being a fixed window.
*/
type stageMemory struct {
	mu      sync.Mutex
	matches map[string]stageMatch
	ttl     time.Duration
}

const (
	// ponytail: fixed initial TTL until cadence-derived TTL is wired
	// from statutil.MedianCadence on real measurement stamps. Upgrade:
	// pass cadence into Evaluate and scale TTL = cadence * WindowDepth.
	defaultStageTTL = 30 * time.Second
)

func newStageMemory() *stageMemory {
	return &stageMemory{
		matches: make(map[string]stageMatch),
		ttl:     defaultStageTTL,
	}
}

/*
record stores that this branch matched for the given symbol at now.
*/
func (memory *stageMemory) record(symbol string, now time.Time) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	memory.matches[symbol] = stageMatch{stamp: now}
}

/*
active reports whether this branch matched for the given symbol within the TTL.
*/
func (memory *stageMemory) active(symbol string, now time.Time) bool {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	match, ok := memory.matches[symbol]
	if !ok {
		return false
	}

	if now.Sub(match.stamp) > memory.ttl {
		delete(memory.matches, symbol)
		return false
	}

	return true
}

/*
clear removes a symbol's match (e.g. after a child branch fires its action).
*/
func (memory *stageMemory) clear(symbol string) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	delete(memory.matches, symbol)
}
