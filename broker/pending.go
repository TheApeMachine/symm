package broker

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
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
func (book *PendingBook) Update(frame map[string]any) error {
	if book == nil || len(frame) == 0 {
		return nil
	}

	updates, err := book.updates(frame)
	if err != nil {
		return err
	}

	for _, update := range updates {
		book.apply(update)
	}

	return nil
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

func (book *PendingBook) updates(frame map[string]any) ([]PendingOrder, error) {
	updates := make([]PendingOrder, 0, 1)
	rows, err := book.rows(frame)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		clOrdID := stringValue(row["cl_ord_id"])
		status := stringValue(row["order_status"])
		if status == "" {
			status = stringValue(row["status"])
		}

		updates = append(updates, PendingOrder{ClOrdID: clOrdID, Status: status})
	}

	if len(updates) > 0 {
		return updates, nil
	}

	clOrdID := stringValue(frame["cl_ord_id"])
	status := stringValue(frame["order_status"])
	if status == "" {
		status = stringValue(frame["status"])
	}

	if clOrdID == "" && status == "" {
		return nil, nil
	}

	return []PendingOrder{{ClOrdID: clOrdID, Status: status}}, nil
}

func (book *PendingBook) rows(frame map[string]any) ([]map[string]any, error) {
	switch data := frame["data"].(type) {
	case nil:
		return nil, nil
	case []map[string]any:
		return data, nil
	case []any:
		rows := make([]map[string]any, 0, len(data))
		for _, item := range data {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: pending data row object required",
					nil,
				))
			}

			rows = append(rows, row)
		}

		return rows, nil
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: pending data rows required",
			nil,
		))
	}
}

func terminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "filled", "rejected", "canceled", "cancelled", "expired":
		return true
	default:
		return false
	}
}
