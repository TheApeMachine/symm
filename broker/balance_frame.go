package broker

import (
	"iter"
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
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

	return datura.Map[any]{
		"balances": balance.quoteRows(),
		"holdings": slices.Collect(balance.lots()),
		"stops":    balance.stopRows(),
	}.Marshal()
}

/*
quoteRows returns the single quote-currency cash row the wallet rail expects.
*/
func (balance *Balance) quoteRows() []datura.Map[any] {
	value, ok := balance.data.Load(balance.quote)

	if !ok {
		return []datura.Map[any]{}
	}

	row := value.(*kraken.BalanceData)
	total := wireFloat(row.Balance)

	return []datura.Map[any]{{
		"asset":     balance.quote,
		"balance":   total,
		"available": total,
		"reserved":  0.0,
	}}
}

/*
lots yields every retained holding, including closed lots for the audit rail.
Strategy slot math keeps using Holdings(), which skips closed inventory.
*/
func (balance *Balance) lots() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		if balance.holdings == nil {
			return
		}

		balance.holdings.Range(func(key, value any) bool {
			holding := value.(*types.Holding)

			if key.(string) != holding.Symbol {
				return true
			}

			return yield(*holding)
		})
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

	balance.data.Range(func(key, value any) bool {
		asset := key.(string)

		if asset == "" || asset == balance.quote {
			return true
		}

		row := value.(*kraken.BalanceData)

		if row.Balance == nil || row.Balance.Sign() <= 0 {
			balance.closeWalletLot(asset)
			return true
		}

		symbol := asset + "/" + balance.quote
		seen[symbol] = struct{}{}
		balance.upsertWalletLot(symbol, asset, row.Balance)

		return true
	})

	balance.holdings.Range(func(key, value any) bool {
		holding := value.(*types.Holding)

		if holding.Status != types.OPEN {
			return true
		}

		if _, ok := seen[key.(string)]; ok {
			return true
		}

		// Pending thesis enters are not wallet-backed until Desk fills them.
		if holding.Asset == "" {
			return true
		}

		holding.Status = types.CLOSED
		holding.Qty = decimal.NewFromInt64(0)

		return true
	})
}

/*
upsertWalletLot opens or refreshes a wallet-backed holding so restart inventory
and live balance sync keep Qty/Asset aligned with the exchange row.
*/
func (balance *Balance) upsertWalletLot(
	symbol, asset string,
	qty *decimal.Decimal,
) {
	if value, ok := balance.holdings.Load(symbol); ok {
		holding := value.(*types.Holding)
		holding.Qty = qty.Copy()
		holding.Asset = asset

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			holding.Status = types.OPEN
		}

		return
	}

	balance.holdings.Store(symbol, &types.Holding{
		Symbol: symbol,
		Asset:  asset,
		Qty:    qty.Copy(),
		Status: types.OPEN,
	})
}

/*
closeWalletLot marks a wallet-backed lot closed when its exchange qty has gone
to zero so the desk and UI stop treating drained inventory as open.
*/
func (balance *Balance) closeWalletLot(asset string) {
	symbol := asset + "/" + balance.quote
	value, ok := balance.holdings.Load(symbol)

	if !ok {
		return
	}

	holding := value.(*types.Holding)
	holding.Status = types.CLOSED
	holding.Qty = decimal.NewFromInt64(0)
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
