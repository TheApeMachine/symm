package broker

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
)

/*
PendingBook stores immutable pending-order snapshots keyed by client order id.
*/
type PendingBook struct {
	snapshot atomic.Pointer[pendingSnapshot]
}

type pendingSnapshot struct {
	orders map[string]PendingOrder
}

/*
PendingOrder tracks an order the desk submitted but has not terminally observed.
*/
type PendingOrder struct {
	ClOrdID    string
	DecisionID string
	ActionID   string
	Symbol     string
	Side       string
	OrderType  string
	Qty        float64
	Notional   float64
	CreatedAt  time.Time
	Status     string
	Protective bool
}

/*
NewPendingBook instantiates an empty pending ledger.
*/
func NewPendingBook() *PendingBook {
	book := &PendingBook{}
	book.snapshot.Store(&pendingSnapshot{orders: map[string]PendingOrder{}})

	return book
}

/*
Add records one submitted order unless its client id is already pending.
*/
func (book *PendingBook) Add(order PendingOrder) bool {
	if book == nil || order.ClOrdID == "" {
		return false
	}

	for {
		oldSnapshot := book.snapshot.Load()
		next := copyPending(oldSnapshot)
		if _, exists := next[order.ClOrdID]; exists {
			return false
		}

		next[order.ClOrdID] = order
		if book.snapshot.CompareAndSwap(oldSnapshot, &pendingSnapshot{orders: next}) {
			return true
		}
	}
}

/*
Update folds an execution update into the pending ledger.
*/
func (book *PendingBook) Update(artifact *datura.Artifact) {
	if book == nil || artifact == nil {
		return
	}

	for _, update := range pendingUpdates(artifact) {
		book.apply(update)
	}
}

func (book *PendingBook) apply(update PendingOrder) {
	if update.ClOrdID == "" {
		return
	}

	for {
		oldSnapshot := book.snapshot.Load()
		next := copyPending(oldSnapshot)
		current, exists := next[update.ClOrdID]
		if !exists {
			return
		}

		current.Status = update.Status
		if terminalStatus(update.Status) {
			delete(next, update.ClOrdID)
		} else {
			next[update.ClOrdID] = current
		}

		if book.snapshot.CompareAndSwap(oldSnapshot, &pendingSnapshot{orders: next}) {
			return
		}
	}
}

/*
Count returns the number of currently pending orders.
*/
func (book *PendingBook) Count() int {
	if book == nil {
		return 0
	}

	snapshot := book.snapshot.Load()
	if snapshot == nil {
		return 0
	}

	return len(snapshot.orders)
}

func copyPending(snapshot *pendingSnapshot) map[string]PendingOrder {
	next := map[string]PendingOrder{}
	if snapshot == nil {
		return next
	}

	for id, order := range snapshot.orders {
		next[id] = order
	}

	return next
}

func pendingUpdates(artifact *datura.Artifact) []PendingOrder {
	updates := make([]PendingOrder, 0, 1)
	for index := 0; ; index++ {
		prefix := []any{"data", index}
		clOrdID := datura.Peek[string](artifact, append(prefix, "cl_ord_id")...)
		status := datura.Peek[string](artifact, append(prefix, "order_status")...)
		if status == "" {
			status = datura.Peek[string](artifact, append(prefix, "status")...)
		}

		if clOrdID == "" && status == "" {
			break
		}

		updates = append(updates, PendingOrder{ClOrdID: clOrdID, Status: status})
	}

	if len(updates) > 0 {
		return updates
	}

	clOrdID := datura.Peek[string](artifact, "cl_ord_id")
	status := datura.Peek[string](artifact, "order_status")
	if status == "" {
		status = datura.Peek[string](artifact, "status")
	}

	if clOrdID == "" && status == "" {
		return nil
	}

	return []PendingOrder{{ClOrdID: clOrdID, Status: status}}
}

func terminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "filled", "rejected", "canceled", "cancelled", "expired":
		return true
	default:
		return false
	}
}
