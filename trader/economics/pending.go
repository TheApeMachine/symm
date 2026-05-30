package economics

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

/*
pendingForward tracks one open entry awaiting a forward-return label.
*/
type pendingForward struct {
	symbol        string
	playbook      string
	entryPrice    float64
	roundTripCost float64
	openedAt      time.Time
}

/*
Pending tracks forward-return labels for open entries.
*/
type Pending struct {
	mu      sync.Mutex
	entries map[string]pendingForward
}

/*
NewPending instantiates a forward-return pending queue.
*/
func NewPending() *Pending {
	return &Pending{entries: make(map[string]pendingForward)}
}

/*
TrackEntry registers one entry for forward labeling.
*/
func (pending *Pending) TrackEntry(
	symbol, playbook string,
	entryPrice, roundTripCost float64,
	openedAt time.Time,
) {
	if symbol == "" || entryPrice <= 0 {
		return
	}

	pending.mu.Lock()
	defer pending.mu.Unlock()

	pending.entries[symbol] = pendingForward{
		symbol:        symbol,
		playbook:      playbook,
		entryPrice:    entryPrice,
		roundTripCost: roundTripCost,
		openedAt:      openedAt,
	}
}

/*
Drop removes a symbol from the forward queue (exit without label).
*/
func (pending *Pending) Drop(symbol string) {
	pending.mu.Lock()
	defer pending.mu.Unlock()

	delete(pending.entries, symbol)
}

/*
ResolveForward promotes matured entries and returns labels to record.
*/
func (pending *Pending) ResolveForward(symbol string, lastPrice float64, now time.Time) []Label {
	if lastPrice <= 0 {
		return nil
	}

	window := config.System.ExecutionForwardWindow

	if window <= 0 {
		window = 30 * time.Second
	}

	pending.mu.Lock()
	defer pending.mu.Unlock()

	entry, ok := pending.entries[symbol]

	if !ok {
		return nil
	}

	if now.Sub(entry.openedAt) < window {
		return nil
	}

	delete(pending.entries, symbol)

	return []Label{ForwardLabel(
		entry.symbol, entry.playbook, entry.entryPrice, lastPrice, entry.roundTripCost, now,
	)}
}
