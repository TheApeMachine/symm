package market

import (
	"sync"
	"sync/atomic"
)

/*
BookHealthEvent reports one book checksum transition for a symbol.
*/
type BookHealthEvent struct {
	Signal        string
	Symbol        string
	Recovered     bool
	TotalDiverged int
}

/*
BookHealthSink receives book integrity transitions for UI and eval summaries.
*/
type BookHealthSink interface {
	BookHealth(event BookHealthEvent)
}

/*
BookIntegrityReport is the replay eval book checksum outcome.
*/
type BookIntegrityReport struct {
	DivergedSymbols  int
	DivergenceEvents int
	Diverged         []string
}

var bookHealth struct {
	mu       sync.RWMutex
	sink     BookHealthSink
	diverged map[bookHealthKey]struct{}
	events   atomic.Int64
}

type bookHealthKey struct {
	signal string
	symbol string
}

func init() {
	bookHealth.diverged = make(map[bookHealthKey]struct{})
}

/*
SetBookHealthSink installs the process-wide book health observer.
*/
func SetBookHealthSink(sink BookHealthSink) {
	bookHealth.mu.Lock()
	bookHealth.sink = sink
	bookHealth.mu.Unlock()
}

/*
ResetBookHealth clears divergence state for a fresh eval or replay run.
*/
func ResetBookHealth() {
	bookHealth.mu.Lock()
	bookHealth.diverged = make(map[bookHealthKey]struct{})
	bookHealth.mu.Unlock()
	bookHealth.events.Store(0)
}

/*
RecordBookDivergence marks a symbol's maintained book out of sync with Kraken.
*/
func RecordBookDivergence(signal, symbol string) {
	if signal == "" || symbol == "" {
		return
	}

	key := bookHealthKey{signal: signal, symbol: symbol}

	bookHealth.mu.Lock()

	_, already := bookHealth.diverged[key]

	if !already {
		bookHealth.diverged[key] = struct{}{}
	}

	total := len(bookHealth.diverged)
	bookHealth.mu.Unlock()

	if already {
		return
	}

	bookHealth.events.Add(1)

	if !replayActive() {
		RequestBookFeedRestart()
	}

	emitBookHealth(BookHealthEvent{
		Signal:        signal,
		Symbol:        symbol,
		Recovered:     false,
		TotalDiverged: total,
	})
}

/*
RecordBookRecovery clears a symbol after a fresh snapshot realigns the book.
*/
func RecordBookRecovery(signal, symbol string) {
	if signal == "" || symbol == "" {
		return
	}

	key := bookHealthKey{signal: signal, symbol: symbol}

	bookHealth.mu.Lock()

	_, wasDiverged := bookHealth.diverged[key]

	if wasDiverged {
		delete(bookHealth.diverged, key)
	}

	total := len(bookHealth.diverged)
	bookHealth.mu.Unlock()

	if !wasDiverged {
		return
	}

	emitBookHealth(BookHealthEvent{
		Signal:        signal,
		Symbol:        symbol,
		Recovered:     true,
		TotalDiverged: total,
	})
}

/*
BookIntegritySummary returns the current divergence snapshot.
*/
func BookIntegritySummary() BookIntegrityReport {
	bookHealth.mu.RLock()
	defer bookHealth.mu.RUnlock()

	symbols := make(map[string]struct{}, len(bookHealth.diverged))
	diverged := make([]string, 0, len(bookHealth.diverged))

	for key := range bookHealth.diverged {
		if _, ok := symbols[key.symbol]; ok {
			continue
		}

		symbols[key.symbol] = struct{}{}
		diverged = append(diverged, key.symbol)
	}

	return BookIntegrityReport{
		DivergedSymbols:  len(symbols),
		DivergenceEvents: int(bookHealth.events.Load()),
		Diverged:         diverged,
	}
}

func emitBookHealth(event BookHealthEvent) {
	bookHealth.mu.RLock()
	sink := bookHealth.sink
	bookHealth.mu.RUnlock()

	if sink == nil {
		return
	}

	sink.BookHealth(event)
}
