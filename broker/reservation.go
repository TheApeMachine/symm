package broker

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Reservation is a first-class local claim on quote cash. It never mutates the
cached Kraken Available/Reserved row — exchange snapshots stay authoritative,
and Book/Release only touch the claim ledger.
*/
type Reservation struct {
	ID     string
	Amount *decimal.Decimal
}

var reservationSeq atomic.Uint64

/*
Book atomically checks effective quote availability and inserts a claim.
Effective available is exchange Available minus active local reservations.
*/
func (balance *Balance) Book(
	amount, fraction *decimal.Decimal,
) (*Reservation, error) {
	if balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance missing for reservation",
			nil,
		))
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if err := balance.validate(map[string]any{
		"model": balance.model,
	}); err != nil {
		return nil, err
	}

	reserved, err := balance.reservation(amount, fraction)

	if err != nil {
		return nil, err
	}

	effective, err := balance.effectiveAvailableLocked()

	if err != nil {
		return nil, err
	}

	if effective.Sub(reserved).Sign() < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"insufficient available balance to reserve",
			nil,
		))
	}

	if balance.books == nil {
		balance.books = map[string]*Reservation{}
	}

	id := fmt.Sprintf(
		"%d-%d",
		time.Now().UnixNano(),
		reservationSeq.Add(1),
	)

	row := &Reservation{ID: id, Amount: reserved.Copy()}
	balance.books[id] = row

	return &Reservation{ID: id, Amount: reserved.Copy()}, nil
}

/*
Release deletes a Book claim. It does not rewrite the exchange snapshot.
Missing ids are a no-op so idempotent cancel paths stay safe.
*/
func (balance *Balance) Release(id string) error {
	if balance == nil || id == "" {
		return nil
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if balance.books != nil {
		delete(balance.books, id)
	}

	return nil
}

/*
RestoreClaim reinserts a durable reservation id after restart without mutating
the exchange Available row. Effective available subtracts it again.
*/
func (balance *Balance) RestoreClaim(id string, amount *decimal.Decimal) error {
	if balance == nil || id == "" || amount == nil || amount.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"restore claim requires id and positive amount",
			nil,
		))
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if balance.books == nil {
		balance.books = map[string]*Reservation{}
	}

	balance.books[id] = &Reservation{ID: id, Amount: amount.Copy()}

	return nil
}

/*
Consume drops the Book ledger entry after the broker accepted the order so a
later cancel cannot double-credit while wallet sync owns the cash.
*/
func (balance *Balance) Consume(id string) {
	if balance == nil || id == "" {
		return
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if balance.books != nil {
		delete(balance.books, id)
	}
}

/*
Funded reports whether reservation id still covers amount for Enter.
*/
func (balance *Balance) Funded(id string, amount *decimal.Decimal) bool {
	if balance == nil || id == "" || amount == nil {
		return false
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	row, ok := balance.books[id]

	if !ok || row == nil || row.Amount == nil {
		return false
	}

	return row.Amount.Cmp(amount) >= 0
}

/*
Snapshots returns open reservation ids and amounts for compact recovery.
*/
func (balance *Balance) Snapshots() []Reservation {
	if balance == nil {
		return nil
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if len(balance.books) == 0 {
		return nil
	}

	rows := make([]Reservation, 0, len(balance.books))

	for _, row := range balance.books {
		if row == nil || row.Amount == nil {
			continue
		}

		rows = append(rows, Reservation{
			ID:     row.ID,
			Amount: row.Amount.Copy(),
		})
	}

	return rows
}

/*
ErrInsufficientReservation is returned when Enter lacks a covering Book claim.
*/
func ErrInsufficientReservation() error {
	return errnie.Err(
		errnie.Validation,
		"reservation does not cover order cost",
		nil,
	)
}
