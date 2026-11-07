package broker

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/system"
)

type tapePrint struct {
	at    time.Time
	quote float64
}

// Tape keeps quote notional so entry can compare the last window with the one before it.
type Tape struct {
	mu     sync.Mutex
	prints map[string][]tapePrint
}

func NewTape() *Tape {
	return &Tape{prints: map[string][]tapePrint{}}
}

// Note records one print's quote dollars and drops anything older than two windows.
func (tape *Tape) Note(symbol string, quote float64, at time.Time) {
	if symbol == "" || quote <= 0 || at.IsZero() {
		return
	}

	tape.mu.Lock()
	defer tape.mu.Unlock()

	cutoff := at.Add(-2 * system.Cfg.Risk.TapeWindow)
	kept := tape.prints[symbol][:0]

	for _, print := range tape.prints[symbol] {
		if print.at.Before(cutoff) {
			continue
		}

		kept = append(kept, print)
	}

	tape.prints[symbol] = append(kept, tapePrint{at: at, quote: quote})
}

// Uptake sums quote dollars in [now-window, now] and the equal window before it.
func (tape *Tape) Uptake(symbol string, now time.Time) (recent, prior float64) {
	tape.mu.Lock()
	defer tape.mu.Unlock()

	window := system.Cfg.Risk.TapeWindow
	recentStart := now.Add(-window)
	priorStart := now.Add(-2 * window)

	for _, print := range tape.prints[symbol] {
		if print.at.After(now) {
			continue
		}

		if !print.at.Before(recentStart) {
			recent += print.quote
			continue
		}

		if !print.at.Before(priorStart) {
			prior += print.quote
		}
	}

	return recent, prior
}
