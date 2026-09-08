package broker

import (
	"maps"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
	NewFundedBalance opens an independent simulated quote account. Positions

own its inventory; live balances continue to be supplied by the venue.
*/
func NewFundedBalance(quote string, cash *decimal.Decimal) (*Balance, error) {
	if quote == "" || cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "balance: positive funding and quote currency required", nil))
	}
	balance := &Balance{Quote: quote}
	balance.snapshot.Store(&balanceSnapshot{assets: map[string]*decimal.Decimal{quote: cash}, status: types.READY, from: time.Now()})
	return balance, nil
}

/*
	Settle books one complete simulated fill. The regulator owns cumulative fill

reconciliation; live account observations cannot be modified through this path.
*/
func (balance *Balance) Settle(fill kraken.ExecutionData) error {
	if balance.api != nil || fill.CumCost == nil || fill.FeeUsdEquiv == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "balance: complete simulated fill required", nil))
	}
	balance.mu.Lock()
	defer balance.mu.Unlock()
	cash := balance.Cash()

	switch fill.Side {
	case "buy":
		cash = cash.Sub(fill.CumCost).Sub(fill.FeeUsdEquiv)
	case "sell":
		cash = cash.Add(fill.CumCost).Sub(fill.FeeUsdEquiv)
	default:
		return errnie.Error(errnie.Err(errnie.Validation, "balance: unknown fill side", nil))
	}

	if cash.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "balance: fill exceeds available cash", nil))
	}
	assets := maps.Clone(balance.snapshot.Load().assets)
	assets[balance.Quote] = cash
	balance.snapshot.Store(&balanceSnapshot{assets: assets, status: types.READY, from: fill.Timestamp})
	return nil
}
