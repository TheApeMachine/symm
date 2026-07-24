package broker

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
Reservation holds cash and/or base-asset claims plus an optional slot for one
intent until Commit or Release.
*/
type Reservation struct {
	IntentID string
	Symbol   string
	Asset    string
	Cash     *decimal.Decimal
	Qty      *decimal.Decimal
	Slot     bool
}

/*
Ledger tracks open reservations keyed by intent ID. Desk mutates it on the
serial broker path; readers observe Reserved totals for UI and Available.
*/
type Ledger struct {
	mu    sync.Mutex
	byID  map[string]Reservation
	cash  *decimal.Decimal
	qty   map[string]*decimal.Decimal
	slots int
}

/*
NewLedger constructs an empty reservation ledger.
*/
func NewLedger() *Ledger {
	return &Ledger{
		byID: make(map[string]Reservation),
		cash: decimal.NewFromInt64(0),
		qty:  make(map[string]*decimal.Decimal),
	}
}

/*
Reserve claims quote cash and optionally a slot for intentID.
*/
func (ledger *Ledger) Reserve(
	intentID, symbol string,
	cash *decimal.Decimal,
	slot bool,
) error {
	return ledger.claim(Reservation{
		IntentID: intentID,
		Symbol:   symbol,
		Cash:     cash,
		Slot:     slot,
	})
}

/*
ReserveAsset claims sellable base inventory for an exit intent.
*/
func (ledger *Ledger) ReserveAsset(
	intentID, asset string,
	qty *decimal.Decimal,
) error {
	return ledger.claim(Reservation{
		IntentID: intentID,
		Asset:    asset,
		Qty:      qty,
	})
}

func (ledger *Ledger) claim(reservation Reservation) error {
	if ledger == nil {
		return types.ValidationError{
			Component: "reservation",
			Detail:    "ledger required",
		}
	}

	if reservation.IntentID == "" {
		return types.ValidationError{
			Component: "reservation",
			Detail:    "intent id required",
		}
	}

	hasCash := reservation.Cash != nil && reservation.Cash.Sign() > 0
	hasQty := reservation.Qty != nil && reservation.Qty.Sign() > 0

	if !hasCash && !hasQty {
		return types.ValidationError{
			Component: "reservation",
			Detail:    "positive cash or qty required",
		}
	}

	if hasQty && reservation.Asset == "" {
		return types.ValidationError{
			Component: "reservation",
			Detail:    "asset required when qty is set",
		}
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if _, exists := ledger.byID[reservation.IntentID]; exists {
		return types.ConflictError{
			Component: "reservation",
			Detail:    "already open for " + reservation.IntentID,
		}
	}

	stored := Reservation{
		IntentID: reservation.IntentID,
		Symbol:   reservation.Symbol,
		Asset:    reservation.Asset,
		Slot:     reservation.Slot,
	}

	if hasCash {
		stored.Cash = reservation.Cash.Copy()
		ledger.cash = ledger.cash.Add(stored.Cash)
	}

	if hasQty {
		stored.Qty = reservation.Qty.Copy()
		prior := ledger.qty[reservation.Asset]

		if prior == nil {
			ledger.qty[reservation.Asset] = stored.Qty.Copy()
		}

		if prior != nil {
			ledger.qty[reservation.Asset] = prior.Add(stored.Qty)
		}
	}

	if reservation.Slot {
		ledger.slots++
	}

	ledger.byID[reservation.IntentID] = stored

	return nil
}

/*
Commit clears a reservation after the venue accepts the intent.
*/
func (ledger *Ledger) Commit(intentID string) error {
	return ledger.release(intentID)
}

/*
Release drops a reservation without a fill (reject / cancel).
*/
func (ledger *Ledger) Release(intentID string) error {
	return ledger.release(intentID)
}

func (ledger *Ledger) release(intentID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	reservation, ok := ledger.byID[intentID]

	if !ok {
		return types.NotFoundError{
			Component: "reservation",
			Detail:    "missing for " + intentID,
		}
	}

	delete(ledger.byID, intentID)

	if reservation.Cash != nil {
		ledger.cash = ledger.cash.Sub(reservation.Cash)
	}

	if reservation.Qty != nil && reservation.Asset != "" {
		prior := ledger.qty[reservation.Asset]

		if prior != nil {
			ledger.qty[reservation.Asset] = prior.Sub(reservation.Qty)
		}
	}

	if reservation.Slot {
		ledger.slots--
	}

	return nil
}

/*
ReservedCash is the sum of open cash claims.
*/
func (ledger *Ledger) ReservedCash() *decimal.Decimal {
	if ledger == nil {
		return decimal.NewFromInt64(0)
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if ledger.cash == nil {
		return decimal.NewFromInt64(0)
	}

	return ledger.cash.Copy()
}

/*
ReservedAsset is the sum of open sell claims for asset.
*/
func (ledger *Ledger) ReservedAsset(asset string) *decimal.Decimal {
	if ledger == nil {
		return decimal.NewFromInt64(0)
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	qty := ledger.qty[asset]

	if qty == nil {
		return decimal.NewFromInt64(0)
	}

	return qty.Copy()
}

/*
ReservedSlots counts open slot claims.
*/
func (ledger *Ledger) ReservedSlots() int {
	if ledger == nil {
		return 0
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	return ledger.slots
}
