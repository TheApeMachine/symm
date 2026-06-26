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
	stamp  time.Time
	window time.Duration
}

/*
stageMemory tracks per-symbol stage matches for one branch. The validity window
is not a fixed wall-clock TTL: it is derived per symbol from that symbol's own
measurement cadence so a fast, liquid pair and a slow, thin one each get a window
proportional to how often they actually measure. A stage stays armed for a budget
of median inter-measurement intervals after it matched.
*/
type stageMemory struct {
	mu      sync.Mutex
	matches map[string]stageMatch
}

func newStageMemory() *stageMemory {
	return &stageMemory{
		matches: make(map[string]stageMatch),
	}
}

/*
record stores that this branch matched for the given symbol at now, carrying the
cadence-derived validity window measured for this symbol on this tick. now is an
artifact timestamp, not wall-clock — sequential ordering compares stamps.
*/
func (memory *stageMemory) record(symbol string, now time.Time, window time.Duration) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	if prior, ok := memory.matches[symbol]; ok && window <= 0 && prior.stamp.Before(now) {
		window = now.Sub(prior.stamp)
	}

	memory.matches[symbol] = stageMatch{stamp: now, window: window}
}

/*
active reports whether this branch matched for the given symbol and the match is
still within its cadence-derived window relative to now (an artifact timestamp).
A match with no derived window (cadence unknown — a single observation) is valid
on its own tick only, so the sequence cannot fire from one lonely stamp.
*/
func (memory *stageMemory) active(symbol string, now time.Time) bool {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	match, ok := memory.matches[symbol]
	if !ok {
		return false
	}

	if match.window <= 0 {
		return match.stamp.Equal(now)
	}

	if now.Sub(match.stamp) > match.window {
		delete(memory.matches, symbol)
		return false
	}

	return true
}

/*
matchedBefore reports whether this branch's recorded match for the symbol is
strictly older than now (an artifact timestamp) and still within its window.
Nested sequential branches use this so a child stage only fires on a measurement
batch that arrived AFTER the parent matched — "stage A fired, THEN stage B",
never both collapsed onto the same instant.
*/
func (memory *stageMemory) matchedBefore(symbol string, now time.Time) bool {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	match, ok := memory.matches[symbol]
	if !ok {
		return false
	}

	if match.window <= 0 {
		if match.stamp.Before(now) {
			delete(memory.matches, symbol)
			return true
		}

		return false
	}

	if now.Sub(match.stamp) > match.window {
		delete(memory.matches, symbol)
		return false
	}

	return match.stamp.Before(now)
}

/*
clear removes a symbol's match (e.g. after a child branch fires its action).
*/
func (memory *stageMemory) clear(symbol string) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	delete(memory.matches, symbol)
}
