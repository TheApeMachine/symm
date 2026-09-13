package broker

import (
	"context"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
Balance is a centralized manager of the exchange wallet, and should be called by
any other object that wants to interact with the balance in any way.
*/
type Balance struct {
	*runtime.System
	api          *websocket.API
	Quote        string
	wallet       atomic.Pointer[kraken.Balance]
	tradeBalance atomic.Pointer[kraken.TradeBalanceResult]
}

func NewBalance(api *websocket.API) *Balance {
	ctx := context.Background()

	if api != nil && api.Context() != nil {
		ctx = api.Context()
	}

	balance := &Balance{
		System: runtime.NewSystem(ctx, "balance"),
		api:    api,
		Quote:  system.Cfg.Market.QuoteCurrency,
	}

	balance.Update()
	return balance
}

/*
Update replaces the entire map; readers cannot observe a partially refreshed account.
*/
func (balance *Balance) Update() {
	if balance == nil || balance.api == nil {
		return
	}

	if balance.Status() == runtime.BUSY {
		return
	}

	balance.Transition(runtime.BUSY)
	defer balance.Transition(runtime.READY)

	result, err := balance.api.Balance()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[balance] failed to retrieve account balance",
			err,
		))

		return
	}

	if result != nil {
		balance.wallet.Store(result)
	}
}

/*
Assets copies the current account map for callers that retain or transform it.
*/
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	balanceData := balance.wallet.Load()
	out := make(map[string]*decimal.Decimal)

	if balanceData == nil {
		return out
	}

	for _, data := range balanceData.Data {
		out[data.Asset] = data.Balance
	}

	return out
}

/* Cash reports the quote balance exactly as the exchange last stated it. */
func (balance *Balance) Cash() *decimal.Decimal { return balance.Assets()[balance.Quote] }

/* Equity returns total portfolio equity from authoritative trade balance or cash. */
func (balance *Balance) Equity() *decimal.Decimal {
	tradeBalance := balance.tradeBalance.Load()

	if tradeBalance != nil && tradeBalance.Equity != nil {
		return tradeBalance.Equity
	}

	if tradeBalance != nil && tradeBalance.EquivalentBalance != nil {
		return tradeBalance.EquivalentBalance
	}

	return balance.Cash()
}

/* Unrealized returns unrealized profit/loss across all open positions. */
func (balance *Balance) Unrealized() *decimal.Decimal {
	tradeBalance := balance.tradeBalance.Load()

	if tradeBalance != nil && tradeBalance.UnrealizedPnL != nil {
		return tradeBalance.UnrealizedPnL
	}

	return decimal.NewFromInt64(0)
}

/*
Refresh reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (balance *Balance) Refresh(instrument *Instrument) (err error) {
	if balance == nil || balance.api == nil {
		return nil
	}

	if balance.Status() == runtime.BUSY {
		return nil
	}

	balance.Transition(runtime.BUSY)
	defer balance.Transition(runtime.READY)

	balance.Update()
	tradeBalance, err := balance.api.TradeBalance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: could not fetch trade balance",
			err,
		))
	}

	balance.tradeBalance.Store(tradeBalance)

	return nil
}
