package broker

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"golang.org/x/time/rate"
)

/*
Balance is a centralized manager of the exchange wallet, and should be called by
any other object that wants to interact with the balance in any way.
*/
type Balance struct {
	*runtime.System
	transport Transport
	paper     *Paper
	Quote     string
	wallet    atomic.Pointer[kraken.Balance]
	trade     atomic.Pointer[kraken.TradeBalanceResult]
	value     *data.Value
	limiter   *rate.Limiter
}

func NewBalance(
	ctx context.Context,
	transport Transport,
	bankroll *decimal.Decimal,
) *Balance {
	balance := &Balance{
		System:    runtime.NewSystem(ctx, "balance"),
		transport: transport,
		Quote:     system.Cfg.Market.QuoteCurrency,
		limiter: rate.NewLimiter(rate.Limit(
			system.Cfg.WebSocket.DecayRate/float64(system.Cfg.WebSocket.CallsPerUpdate),
		), 1),
	}

	if paper, ok := transport.(*Paper); ok {
		balance.paper = paper
		balance.limiter = rate.NewLimiter(rate.Limit(
			system.Cfg.WebSocket.CallsPerSecond,
		), 1)
	}

	balance.Transition(runtime.READY)
	go balance.watch()
	return balance
}

/*
Wallet returns the latest polled wallet; poll publishes it atomically.
*/
func (balance *Balance) Wallet() *kraken.Balance {
	return balance.wallet.Load()
}

/*
Trade is the latest trade-balance reading from the same poll as Wallet.
*/
func (balance *Balance) Trade() *kraken.TradeBalanceResult {
	if balance == nil {
		return nil
	}

	return balance.trade.Load()
}

func (balance *Balance) cash(asset string) *decimal.Decimal {
	wallet := balance.Wallet()

	if wallet == nil {
		return nil
	}

	for _, entry := range wallet.Data {
		if entry.Asset == asset {
			if entry.Available != nil {
				return entry.Available
			}

			return entry.Balance
		}
	}

	alias := kraken.AssetAlias(asset)

	if alias != "" {
		for _, entry := range wallet.Data {
			if entry.Asset == alias {
				if entry.Available != nil {
					return entry.Available
				}

				return entry.Balance
			}
		}
	}

	return nil
}

func (balance *Balance) watch() {
	for {
		select {
		case <-balance.Context().Done():
			return
		default:
		}

		if err := balance.limiter.Wait(balance.Context()); err != nil {
			return
		}

		if err := balance.poll(); err != nil {
			errnie.Error(err)
		}
	}
}

func (balance *Balance) poll() (err error) {
	var tradeBalance *kraken.TradeBalanceResult

	if balance.paper != nil {
		wallet, paperErr := balance.paper.Balances()

		if paperErr != nil {
			return errnie.Error(paperErr)
		}

		if wallet == nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[balance] empty paper balance response",
				nil,
			))
		}

		balance.wallet.Store(wallet)
		tradeBalance, err = balance.paper.TradeBalance()

		if err != nil {
			return errnie.Error(err)
		}
	}

	if balance.paper == nil {
		client, restErr := kraken.NewAuthenticatedREST()

		if restErr != nil {
			return errnie.Error(restErr)
		}

		resp, balanceErr := client.Balances()

		if balanceErr != nil {
			return errnie.Error(balanceErr)
		}

		if resp == nil || resp.Result == nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[balance] empty balance response",
				nil,
			))
		}

		balance.wallet.Store(kraken.NewBalanceFromMap(resp.Result))

		// No asset parameter: Kraken values the trade balance in its documented
		// default asset, ZUSD.
		result, callErr := spot.Call[kraken.TradeBalanceResult](client, spot.RequestOptions{
			Auth:   true,
			Method: "POST",
			Path:   system.Cfg.WebSocket.Endpoints.TradeBalance,
		})

		if callErr != nil {
			return errnie.Error(callErr)
		}

		tradeBalance = &result.Result
	}

	if tradeBalance == nil || balance.Wallet() == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[balance] trade balance or wallet unavailable",
			nil,
		))
	}

	kept := *tradeBalance
	balance.trade.Store(&kept)

	balance.value = data.NewValue("balance", "update", "all").Append(
		map[string]any{
			"cash":       balance.cash(balance.Quote),
			"equity":     tradeBalance.Equity,
			"unrealized": tradeBalance.UnrealizedPnL,
		},
		"balance",
	)

	// Push the refreshed frame to the active dashboard hub; the hello
	// callback only carries the one captured at connect. With no
	// dashboard connected there is nothing to publish to.
	if balance.value != nil && ui.UI != nil {
		io.Copy(ui.UI, balance.value)
	}

	return nil
}
