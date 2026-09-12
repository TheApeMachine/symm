package broker

import (
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

/*
NewBalance binds the existing account and fetches its initial holdings.
*/
func NewBalance(api *websocket.API) *Balance {
	balance := &Balance{
		api:   api,
		Quote: system.Cfg.Market.QuoteCurrency,
	}

	balance.Update()
	return balance
}

/*
Update replaces the entire map; readers cannot observe a partially refreshed account.
*/
func (balance *Balance) Update() {
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

	balance.wallet.Store(result)
}

/*
Assets copies the current account map for callers that retain or transform it.
*/
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	balanceData := balance.wallet.Load()
	out := make(map[string]*decimal.Decimal)

	for _, data := range balanceData.Data {
		out[data.Asset] = data.Balance
	}

	return out
}

/* Cash reports the quote balance exactly as the exchange last stated it. */
func (balance *Balance) Cash() *decimal.Decimal { return balance.Assets()[balance.Quote] }

/*
Refresh reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (balance *Balance) Refresh(instrument *Instrument) (err error) {
	if balance.Status() == runtime.BUSY {
		return
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
