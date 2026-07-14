package broker

import (
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Reconcile calculates the realized USD outcome from unique exchange fills and
their reported USD-equivalent fees. It rejects incomplete or inconsistent fill
sets rather than estimating missing execution data from ticker prices.
*/
func (price *Price) Reconcile(
	symbol string,
	executions []*kraken.Execution,
) (*PositionQuote, error) {
	if !strings.HasSuffix(symbol, "/USD") {
		return nil, errnie.Err(
			errnie.Validation,
			"USD-equivalent fees cannot reconcile non-USD position "+symbol,
			nil,
		)
	}

	entryCost := *decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
	exitProceeds := entryCost
	entryFee := entryCost
	exitFee := entryCost
	entryQuantity := entryCost
	exitQuantity := entryCost
	mark := entryCost
	seen := make(map[string]struct{})

	for _, execution := range executions {
		for _, fill := range execution.Data {
			if fill.Symbol != symbol || fill.ExecType != "trade" {
				continue
			}

			if fill.ExecID == "" || fill.Cost.Sign() <= 0 || fill.LastQty <= 0 {
				return nil, errnie.Err(
					errnie.Validation,
					"incomplete execution data for "+symbol,
					nil,
				)
			}

			if _, exists := seen[fill.ExecID]; exists {
				continue
			}

			seen[fill.ExecID] = struct{}{}
			quantity := decimal.NewFromFloat64(fill.LastQty).SetScale(decimal.DefaultScale)

			switch fill.Side {
			case "buy":
				entryCost = *entryCost.Add(&fill.Cost)
				entryFee = *entryFee.Add(&fill.FeeUsdEquiv)
				entryQuantity = *entryQuantity.Add(quantity)
			case "sell":
				exitProceeds = *exitProceeds.Add(&fill.Cost)
				exitFee = *exitFee.Add(&fill.FeeUsdEquiv)
				exitQuantity = *exitQuantity.Add(quantity)
				mark = fill.LastPrice
			default:
				return nil, errnie.Err(
					errnie.Validation,
					"unsupported execution side for "+symbol,
					nil,
				)
			}
		}
	}

	if entryCost.Sign() <= 0 || exitProceeds.Sign() <= 0 ||
		entryQuantity.Cmp(&exitQuantity) != 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"unreconciled entry and exit executions for "+symbol,
			nil,
		)
	}

	invested := entryCost.Add(&entryFee)
	pnl := exitProceeds.Sub(&entryCost).Sub(&entryFee).Sub(&exitFee)
	returnPct := pnl.Div(invested).Mul(decimal.NewFromInt64(100))

	return &PositionQuote{
		Mark: mark, EntryNotional: entryCost, ExitNotional: exitProceeds,
		EntryFee: entryFee, ExitFee: exitFee, PnL: *pnl,
		ReturnPct: returnPct.Float64(),
	}, nil
}
