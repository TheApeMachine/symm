package broker

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

/*
Tape records recent trade timestamps per symbol so brittle, silent books
cannot clear entry on a single vacuum print.
*/
type Tape struct {
	window time.Duration
	min    int
	by     sync.Map
}

type tapeRing struct {
	mu sync.Mutex
	at []time.Time
}

func NewTape() *Tape {
	return &Tape{
		window: system.Cfg.Risk.TapeWindow,
		min:    system.Cfg.Risk.MinTapeTrades,
	}
}

/*
Note appends each print's venue timestamp (or now if zero) and prunes the
window. Called from desk.Update on every trade frame.
*/
func (tape *Tape) Note(trades []kraken.TradeData) {
	if tape == nil || len(trades) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, trade := range trades {
		if trade.Symbol == "" {
			continue
		}

		at := trade.Timestamp.UTC()
		if at.IsZero() {
			at = now
		}

		tape.note(trade.Symbol, at, now)
	}
}

func (tape *Tape) note(symbol string, at, now time.Time) {
	raw, _ := tape.by.LoadOrStore(symbol, &tapeRing{})
	ring := raw.(*tapeRing)

	ring.mu.Lock()
	defer ring.mu.Unlock()

	ring.at = append(ring.at, at)
	ring.at = pruneTape(ring.at, now.Add(-tape.window))
}

/*
Count is the number of prints for symbol still inside the liveness window.
*/
func (tape *Tape) Count(symbol string, now time.Time) int {
	if tape == nil || symbol == "" {
		return 0
	}

	raw, ok := tape.by.Load(symbol)
	if !ok {
		return 0
	}

	ring := raw.(*tapeRing)

	ring.mu.Lock()
	defer ring.mu.Unlock()

	if now.IsZero() {
		now = time.Now().UTC()
	}

	ring.at = pruneTape(ring.at, now.Add(-tape.window))
	return len(ring.at)
}

/*
Alive is true when the symbol has at least the configured min prints in-window.
*/
func (tape *Tape) Alive(symbol string) bool {
	if tape == nil {
		return false
	}

	return tape.Count(symbol, time.Now().UTC()) >= tape.min
}

func pruneTape(at []time.Time, cutoff time.Time) []time.Time {
	kept := at[:0]

	for _, stamp := range at {
		if stamp.After(cutoff) {
			kept = append(kept, stamp)
		}
	}

	return kept
}
