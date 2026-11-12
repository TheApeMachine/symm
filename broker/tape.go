package broker

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/system"
)

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

func NewTape() *Tape {
	return &Tape{}
}

func newSymbolTape() *symbolTape {
	n := 2 * int(system.Cfg.Risk.TapeWindow/time.Minute)

	return &symbolTape{
		buckets:      make([]float64, n),
		bucketMinute: make([]int64, n),
	}
}

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
Uptake sums quote dollars in [now-window, now] and the equal window before it.
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
