package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
positionState tracks peak price and playbook metadata for held symbols.
*/
type positionState struct {
	Playbook   string
	EntryScore float64
	Peak       float64
	EntryAt    time.Time
}

type positionBook struct {
	mu    sync.Mutex
	byKey map[string]positionState
}

func newPositionBook() *positionBook {
	return &positionBook{byKey: make(map[string]positionState)}
}

func (book *positionBook) Open(symbol string, state positionState) {
	book.mu.Lock()
	defer book.mu.Unlock()

	book.byKey[symbol] = state
}

func (book *positionBook) Close(symbol string) {
	book.mu.Lock()
	defer book.mu.Unlock()

	delete(book.byKey, symbol)
}

func (book *positionBook) Get(symbol string) (positionState, bool) {
	book.mu.Lock()
	defer book.mu.Unlock()

	state, ok := book.byKey[symbol]

	return state, ok
}

func (book *positionBook) UpdatePeak(symbol string, last float64) {
	book.mu.Lock()
	defer book.mu.Unlock()

	state, ok := book.byKey[symbol]

	if !ok {
		return
	}

	if last > state.Peak {
		state.Peak = last
		book.byKey[symbol] = state
	}
}

/*
pumpTrailBreached reports whether a pump position should ratchet out on retrace.
*/
func (book *positionBook) PumpTrailBreached(symbol string, last float64) bool {
	state, ok := book.Get(symbol)

	if !ok || state.Playbook != string(perspectives.PlaybookPump) || state.Peak <= 0 || last <= 0 {
		return false
	}

	retrace := (state.Peak - last) / state.Peak

	return retrace >= config.System.PumpTrailPct
}
