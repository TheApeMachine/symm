package replay

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
measurementForSymbol returns a price-bearing row for symbol. When the caller's row
belongs to another market (entry-batch preemption), the ledger's last observed
price for symbol is used instead of cross-contaminating exit fills.
*/
func (ledger *replayLedger) measurementForSymbol(
	symbol string,
	row types.Measurement,
) types.Measurement {
	if row.Symbol == symbol && row.Last > 0 {
		return row
	}

	lastPrice := ledger.lastObservedPrice(symbol)

	if lastPrice <= 0 {
		return row
	}

	quoted, ok := ledger.lastQuotedRows[symbol]

	if !ok || quoted.Last != lastPrice {
		return types.Measurement{
			Symbol: symbol,
			Last:   lastPrice,
			At:     row.At,
		}
	}

	exitRow := quoted
	exitRow.Symbol = symbol

	return exitRow
}

func (ledger *replayLedger) lastObservedPrice(symbol string) float64 {
	prices := ledger.pricePaths[symbol]

	if len(prices) == 0 {
		return 0
	}

	return prices[len(prices)-1]
}

func snapshotsForSymbol(symbol string, snapshots []types.Measurement) []types.Measurement {
	if len(snapshots) == 0 {
		return nil
	}

	filtered := make([]types.Measurement, 0, len(snapshots))

	for _, snapshot := range snapshots {
		if snapshot.Symbol == symbol {
			filtered = append(filtered, snapshot)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return filtered
}
