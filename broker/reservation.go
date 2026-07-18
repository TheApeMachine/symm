package broker

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Reservation is a first-class claim on quote cash owned by Balance until
Consume forgets it after a live order, or Release returns it on cancel/fail.
*/
type Reservation struct {
	ID     string
	Amount *decimal.Decimal
}

var reservationSeq atomic.Uint64

/*
Book reserves quote cash and returns an owned Reservation so cancel and fail
paths can release by id without guessing amounts from Decisions.
*/
func (balance *Balance) Book(
	amount, fraction *decimal.Decimal,
) (*Reservation, error) {
	reserved, err := balance.Reserve(amount, fraction, false)

	if err != nil || reserved == nil {
		return nil, err
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

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
Release returns a Book claim to Available. Missing ids are a no-op so idempotent
cancel paths stay safe.
*/
func (balance *Balance) Release(id string) error {
	if balance == nil || id == "" {
		return nil
	}

	balance.mu.Lock()
	row, ok := balance.books[id]

	if ok {
		delete(balance.books, id)
	}

	balance.mu.Unlock()

	if !ok || row == nil || row.Amount == nil {
		return nil
	}

	_, err := balance.Reserve(row.Amount, nil, true)

	return err
}

/*
Consume drops the Book ledger entry after the broker accepted the order so a
later cancel cannot double-credit Available while wallet sync owns the cash.
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
