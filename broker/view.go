package broker

import (
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Publish enqueues Frame on the UI channel. A saturated channel returns an error
instead of dropping the wallet frame silently.
*/
func (balance *Balance) Publish() error {
	frame, err := balance.Frame()

	if err != nil {
		return err
	}

	if balance.ui == nil {
		return nil
	}

	select {
	case balance.ui <- frame:
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
Frame marshals quote cash, every retained lot, and live stops. Wallet rows and
inventory are read under their own locks so projection never re-enters a single
mutex through Holdings.
*/
func (balance *Balance) Frame() ([]byte, error) {
	quote, err := balance.quoteRows()

	if err != nil {
		return nil, err
	}

	holdings := slices.Collect(balance.Lots())
	stops := balance.stopRows(holdings)

	frame, err := datura.Map[any]{
		"balances": quote,
		"holdings": holdings,
		"stops":    stops,
	}.Marshal()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return frame, nil
}

/*
quoteRows returns the quote cash row with available and reserved from the ledger.
*/
func (balance *Balance) quoteRows() ([]datura.Map[any], error) {
	balance.mu.RLock()
	defer balance.mu.RUnlock()

	if err := errnie.Require(map[string]any{
		"data": balance.data,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	row, ok := balance.data[balance.quote]

	if !ok {
		return []datura.Map[any]{}, nil
	}

	total, err := balance.decimal(row.Balance)

	if err != nil {
		return nil, err
	}

	reserved, err := balance.decimal(balance.ReservedCash())

	if err != nil {
		return nil, err
	}

	return []datura.Map[any]{{
		"asset":     balance.quote,
		"balance":   total,
		"available": total - reserved,
		"reserved":  reserved,
	}}, nil
}

/*
stopRows projects live stop gauges for lots that own a regulator.
*/
func (balance *Balance) stopRows(holdings []types.Holding) []map[string]any {
	stops := make([]map[string]any, 0)

	for _, holding := range holdings {
		lot := holding

		if frame := lot.StopFrame(); frame != nil {
			stops = append(stops, frame)
		}
	}

	return stops
}

/*
decimal converts a ledger decimal for the wire and fails on nil or NaN instead
of substituting zero.
*/
func (balance *Balance) decimal(value *decimal.Decimal) (float64, error) {
	if value == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: decimal required for wire",
			nil,
		))
	}

	out := value.Float64()

	if out != out {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: decimal is NaN",
			nil,
		))
	}

	return out, nil
}
