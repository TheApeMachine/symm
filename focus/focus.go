package focus

import (
	"maps"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
AnchorSymbol returns the dashboard anchor pair for chart and UI telemetry.
It is read from market.anchor_symbol, then the first market.default_symbols entry.
Discovery signals consume every pair the instrument catalog subscribes to.
*/
func AnchorSymbol() string {
	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	if anchor != "" {
		return anchor
	}

	defaults := viper.GetStringSlice("market.default_symbols")

	if len(defaults) > 0 {
		first := strings.TrimSpace(defaults[0])

		if first != "" {
			return first
		}
	}

	panic(errnie.Require(map[string]any{
		"market.anchor_symbol":   viper.GetString("market.anchor_symbol"),
		"market.default_symbols": defaults,
	}))
}

/*
Set is the shared set of symbols with an open position. The trader is its only
writer (on entry and exit); producers read it to decide whether to publish a
per-symbol UI frame, so the dashboard bus only carries data for symbols we are
actually trading. Reads are lock-free; writes serialize and copy-on-write so a
reader always sees a consistent snapshot.
*/
type StreamNotifier func(symbol string, added bool)

type Set struct {
	mu             sync.Mutex
	symbols        atomic.Pointer[map[string]struct{}]
	streamNotifier StreamNotifier
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
SetStreamNotifier registers a callback when chart-stream symbols are added or removed.
*/
func (set *Set) SetStreamNotifier(notifier StreamNotifier) {
	set.mu.Lock()
	defer set.mu.Unlock()

	set.streamNotifier = notifier
}

/*
Add marks symbol as in focus.
*/
func (set *Set) Add(symbol string) {
	set.mu.Lock()
	defer set.mu.Unlock()

	current := *set.symbols.Load()

	if _, ok := current[symbol]; ok {
		return
	}

	next := maps.Clone(current)
	next[symbol] = struct{}{}
	set.symbols.Store(&next)
	set.streamNotifierLocked(symbol, true)
}

/*
Remove drops symbol from focus.
*/
func (set *Set) Remove(symbol string) {
	set.mu.Lock()
	defer set.mu.Unlock()

	current := *set.symbols.Load()

	if _, ok := current[symbol]; !ok {
		return
	}

	next := maps.Clone(current)
	delete(next, symbol)
	set.symbols.Store(&next)
	set.streamNotifierLocked(symbol, false)
}

func (set *Set) streamNotifierLocked(symbol string, added bool) {
	if set.streamNotifier == nil {
		return
	}

	set.streamNotifier(symbol, added)
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
	if symbol == AnchorSymbol() {
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
