package broker

import (
	"context"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
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

/*
The venue's own request budgets bound how often watch may run.

Live: Kraken's private REST counter decays by 0.33 per second on the Starter
tier, the lowest tier any account has, and each poll spends two points (Balance
and TradeBalance).

Paper: `kraken paper status` prices the wallet through one public Ticker call,
and Kraken's public REST budget is one call per second.
*/
const (
	privateDecayPerSecond = 0.33
	privateCallsPerUpdate = 2
	publicCallsPerSecond  = 1
)

func NewBalance(
	ctx context.Context,
	transport Transport,
) *Balance {
	balance := &Balance{
		System:    runtime.NewSystem(ctx, "balance"),
		transport: transport,
		Quote:     system.Cfg.Market.QuoteCurrency,
		limiter: rate.NewLimiter(rate.Limit(
			privateDecayPerSecond/privateCallsPerUpdate,
		), 1),
	}

	if paper, ok := transport.(*Paper); ok {
		balance.paper = paper
		balance.limiter = rate.NewLimiter(rate.Limit(
			publicCallsPerSecond,
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

	return nil
}
