package broker

import (
	"iter"
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Frame projects the desk wallet onto the terminal wire: one quote cash row,
every retained lot (open and closed) for positions plus audit, and live stops.
*/
func (balance *Balance) Frame() []byte {
	if balance == nil || balance.data == nil {
		return nil
	}

	frame, err := datura.Map[any]{
		"balances": balance.quoteRows(),
		"holdings": slices.Collect(balance.lots()),
		"stops":    balance.stopRows(),
	}.Marshal()

	if err != nil {
		return nil
	}

	return frame
}

/*
quoteRows returns the quote cash row with available and reserved from the ledger.
*/
func (balance *Balance) quoteRows() []datura.Map[any] {
	row, ok := balance.data[balance.quote]

	if !ok {
		return []datura.Map[any]{}
	}

	total := wireFloat(row.Balance)
	reserved := wireFloat(balance.ledger.ReservedCash())
	available := total - reserved

	return []datura.Map[any]{{
		"asset":     balance.quote,
		"balance":   total,
		"available": available,
		"reserved":  reserved,
	}}
}

/*
lots yields every retained holding, including closed lots for the audit rail.
*/
func (balance *Balance) lots() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		if balance.holdings == nil {
			return
		}

		for symbol, holding := range balance.holdings {
			if symbol != holding.Symbol {
				continue
			}

			if !yield(*holding) {
				return
			}
		}
	}
}

/*
stopRows projects live stop gauges for open lots that own a regulator.
*/
func (balance *Balance) stopRows() []map[string]any {
	stops := make([]map[string]any, 0)

	for holding := range balance.Holdings() {
		lot := holding

		if frame := lot.StopFrame(); frame != nil {
			stops = append(stops, frame)
		}
	}

	return stops
}

/*
syncWallet materializes open holdings from non-quote wallet balances and closes
lots whose exchange qty has gone to zero so restart inventory reaches the UI.
*/
func (balance *Balance) syncWallet() {
	if balance.data == nil || balance.holdings == nil || balance.quote == "" {
		return
	}

	seen := make(map[string]struct{})

	for asset, row := range balance.data {
		if asset == "" || asset == balance.quote {
			continue
		}

		if row.Balance == nil || row.Balance.Sign() <= 0 {
			balance.closeWalletLot(asset)
			continue
		}

		symbol := asset + "/" + balance.quote
		seen[symbol] = struct{}{}
		balance.upsertWalletLot(symbol, asset, row.Balance)
	}

	for symbol, holding := range balance.holdings {
		if holding.Status != types.OPEN {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		if holding.Asset == "" {
			continue
		}

		if err := balance.transitionHolding(holding, types.CLOSED); err != nil {
			errnie.Error(err)
		}

		holding.Qty = decimal.NewFromInt64(0)
		holding.SellableQty = decimal.NewFromInt64(0)
	}
}

/*
upsertWalletLot opens or refreshes a wallet-backed holding from exchange qty.
*/
func (balance *Balance) upsertWalletLot(
	symbol, asset string,
	qty *decimal.Decimal,
) {
	if holding, ok := balance.holdings[symbol]; ok {
		holding.Qty = qty.Copy()
		holding.Asset = asset
		holding.SellableQty = balance.sellable(asset, qty)

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			if err := balance.transitionHolding(holding, types.OPEN); err != nil {
				errnie.Error(err)
			}
		}

		return
	}

	balance.holdings[symbol] = &types.Holding{
		Symbol:      symbol,
		Asset:       asset,
		Qty:         qty.Copy(),
		SellableQty: balance.sellable(asset, qty),
		Status:      types.OPEN,
	}
}

func (balance *Balance) sellable(
	asset string,
	qty *decimal.Decimal,
) *decimal.Decimal {
	reserved := balance.ledger.ReservedAsset(asset)

	return qty.Copy().Sub(reserved)
}

/*
closeWalletLot marks a wallet-backed lot closed when exchange qty is zero.
*/
func (balance *Balance) closeWalletLot(asset string) {
	symbol := asset + "/" + balance.quote
	holding, ok := balance.holdings[symbol]

	if !ok {
		return
	}

	if err := balance.transitionHolding(holding, types.CLOSED); err != nil {
		errnie.Error(err)
	}

	holding.Qty = decimal.NewFromInt64(0)
	holding.SellableQty = decimal.NewFromInt64(0)
}

/*
transitionHolding applies one canonical holding lifecycle edge and fails loud on
illegal transitions.
*/
func (balance *Balance) transitionHolding(
	holding *types.Holding,
	next types.Status,
) error {
	status, err := types.Transition(holding.Status, next)

	if err != nil {
		return errnie.Error(err)
	}

	holding.Status = status

	return nil
}

func wireFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	out := value.Float64()

	if out != out {
		return 0
	}

	return out
}
