package broker

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/system"
)

/*
symbolTape is per-symbol so tape uptake on BTC does not serialize on ETH prints.
*/
type symbolTape struct {
	mu           sync.Mutex
	buckets      []float64
	bucketMinute []int64
}

/*
Tape keeps per-symbol 1-minute quote-notional buckets so entry can compare
the last risk.tape_window with the equal window before it.
*/
type Tape struct {
	symbols sync.Map // map[string]*symbolTape
}

/*
NewTape is lazy per symbol — most of the universe never prints during a session.
*/
func NewTape() *Tape {
	return &Tape{}
}

/*
newSymbolTape keeps two windows of buckets because BookIntact compares recent
uptake against the equal prior window, not a single rolling sum.
*/
func newSymbolTape() *symbolTape {
	n := 2 * int(system.Cfg.Risk.TapeWindow/time.Minute)

	return &symbolTape{
		buckets:      make([]float64, n),
		bucketMinute: make([]int64, n),
	}
}

/*
lane defers allocation until first trade so silent symbols pay no tape storage.
*/
func (tape *Tape) lane(symbol string) *symbolTape {
	if raw, ok := tape.symbols.Load(symbol); ok {
		return raw.(*symbolTape)
	}

	raw, _ := tape.symbols.LoadOrStore(symbol, newSymbolTape())
	return raw.(*symbolTape)
}

/*
Note adds quote dollars to the print's minute bucket.
*/
func (tape *Tape) Note(symbol string, quote float64, at time.Time) {
	if symbol == "" || quote <= 0 || at.IsZero() {
		return
	}

	lane := tape.lane(symbol)
	lane.mu.Lock()
	defer lane.mu.Unlock()

	n := int64(len(lane.buckets))
	minute := at.Unix() / 60
	idx := minute % n

	if lane.bucketMinute[idx] != minute {
		lane.bucketMinute[idx] = minute
		lane.buckets[idx] = 0
	}

	lane.buckets[idx] += quote
}

/*
Uptake is the BookIntact gate: rising quote flow must beat the prior window,
not just absolute notional, to reject dead tapes.
*/
func (tape *Tape) Uptake(symbol string, now time.Time) (recent, prior float64) {
	raw, ok := tape.symbols.Load(symbol)

	if !ok {
		return 0, 0
	}

	lane := raw.(*symbolTape)
	lane.mu.Lock()
	defer lane.mu.Unlock()

	n := int64(len(lane.buckets))
	window := int64(system.Cfg.Risk.TapeWindow / time.Minute)
	current := now.Unix() / 60

	for i := int64(0); i < window; i++ {
		recentMinute := current - i
		idx := recentMinute % n

		if lane.bucketMinute[idx] == recentMinute {
			recent += lane.buckets[idx]
		}

		priorMinute := current - window - i
		priorIdx := priorMinute % n

		if lane.bucketMinute[priorIdx] == priorMinute {
			prior += lane.buckets[priorIdx]
		}
	}

	return recent, prior
}
