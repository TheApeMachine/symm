package broker

import (
	"iter"
	"math"
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
View projects wallet cash, inventory lots, and stop gauges onto the UI wire.
It reads wallet rows and inventory under their own locks before marshaling.
*/
type View struct {
	wallet    *Wallet
	ledger    *Ledger
	inventory *Inventory
	ui        chan []byte
}

/*
Publish enqueues Frame on the UI channel. A saturated channel returns an error
instead of dropping the wallet frame silently.
*/
func (view *View) Publish() error {
	payload, err := view.Frame()

	if err != nil {
		return err
	}

	if view.ui == nil {
		return nil
	}

	select {
	case view.ui <- payload:
		return nil
	default:
		return errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"balance: ui channel saturated; dropped wallet frame",
			nil,
		))
	}
}

/*
Frame marshals quote cash, every retained lot, and live stops for the UI wire.
*/
func (view *View) Frame() ([]byte, error) {
	quote, err := view.quoteRows()

	if err != nil {
		return nil, err
	}

	holdings := slices.Collect(view.lots())
	stops := view.stopRows(holdings)

	payload, err := datura.Map[any]{
		"balances": quote,
		"holdings": holdings,
		"stops":    stops,
	}.Marshal()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return payload, nil
}

/*
lots yields every retained holding for the wire, including closed audit lots.
*/
func (view *View) lots() iter.Seq[types.Holding] {
	return view.inventory.Lots()
}

/*
quoteRows returns the quote cash row with available and reserved from the ledger.
*/
func (view *View) quoteRows() ([]datura.Map[any], error) {
	view.wallet.mu.RLock()
	defer view.wallet.mu.RUnlock()

	if err := errnie.Require(map[string]any{
		"data": view.wallet.data,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	row, ok := view.wallet.data[view.wallet.quote]

	if !ok {
		return []datura.Map[any]{}, nil
	}

	total, err := view.decimal(row.Balance)

	if err != nil {
		return nil, err
	}

	reserved, err := view.decimal(view.ledger.ReservedCash())

	if err != nil {
		return nil, err
	}

	return []datura.Map[any]{{
		"asset":     view.wallet.quote,
		"balance":   total,
		"available": total - reserved,
		"reserved":  reserved,
	}}, nil
}

/*
stopRows projects live stop gauges for lots that own a regulator.
*/
func (view *View) stopRows(holdings []types.Holding) []map[string]any {
	stops := make([]map[string]any, 0)

	for _, holding := range holdings {
		lot := holding

		if stopFrame := lot.StopFrame(); stopFrame != nil {
			stops = append(stops, stopFrame)
		}
	}

	return stops
}

/*
decimal converts a ledger decimal for the wire and fails on nil, NaN, or Inf
instead of substituting zero.
*/
func (view *View) decimal(value *decimal.Decimal) (float64, error) {
	if value == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: decimal required for wire",
			nil,
		))
	}

	out := value.Float64()

	if math.IsNaN(out) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: decimal is NaN",
			nil,
		))
	}

	if math.IsInf(out, 0) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: decimal is infinite",
			nil,
		))
	}

	return out, nil
}
