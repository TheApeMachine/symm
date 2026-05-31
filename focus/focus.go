package focus

import (
	"maps"
	"sync"
	"sync/atomic"
)

/*
AnchorSymbol always streams on the UI bus alongside symbols with open positions.
*/
const AnchorSymbol = "BTC/EUR"

/*
Set is the shared set of symbols with an open position. The trader is its only
writer (on entry and exit); producers read it to decide whether to publish a
per-symbol UI frame, so the dashboard bus only carries data for symbols we are
actually trading. Reads are lock-free; writes serialize and copy-on-write so a
reader always sees a consistent snapshot.
*/
type Set struct {
	mu      sync.Mutex
	symbols atomic.Pointer[map[string]struct{}]
}

/*
NewSet returns an empty focus set.
*/
func NewSet() *Set {
	set := &Set{}
	empty := make(map[string]struct{})
	set.symbols.Store(&empty)

	return set
}

/*
Add marks symbol as in focus.
*/
func (set *Set) Add(symbol string) {
	set.mu.Lock()
	defer set.mu.Unlock()

	next := maps.Clone(*set.symbols.Load())
	next[symbol] = struct{}{}
	set.symbols.Store(&next)
}

/*
Remove drops symbol from focus.
*/
func (set *Set) Remove(symbol string) {
	set.mu.Lock()
	defer set.mu.Unlock()

	next := maps.Clone(*set.symbols.Load())
	delete(next, symbol)
	set.symbols.Store(&next)
}

/*
Has reports whether symbol currently has an open position.
*/
func (set *Set) Has(symbol string) bool {
	_, ok := (*set.symbols.Load())[symbol]

	return ok
}

/*
Streams reports whether symbol should publish per-pair UI telemetry.
*/
func (set *Set) Streams(symbol string) bool {
	if symbol == AnchorSymbol {
		return true
	}

	return set.Has(symbol)
}

/*
Snapshot returns the focused symbols as a slice.
*/
func (set *Set) Snapshot() []string {
	current := *set.symbols.Load()
	symbols := make([]string, 0, len(current))

	for symbol := range current {
		symbols = append(symbols, symbol)
	}

	return symbols
}
