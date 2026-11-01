package broker

import (
	"context"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/decimal"
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
	label     string
	wallet    atomic.Pointer[kraken.Balance]
	trade     atomic.Pointer[kraken.TradeBalanceResult]
	reading   atomic.Pointer[map[string]any]
	limiter   *rate.Limiter
}

func NewBalance(
	ctx context.Context,
	transport Transport,
	bankroll *decimal.Decimal,
) *Balance {
	return NewLabeledBalance(ctx, transport, bankroll, "paper")
}

func NewLabeledBalance(
	ctx context.Context,
	transport Transport,
	bankroll *decimal.Decimal,
	label string,
) *Balance {
	if label == "" {
		label = "paper"
	}

	balance := &Balance{
		System:    runtime.NewSystem(ctx, "balance"),
		transport: transport,
		Quote:     system.Cfg.Market.QuoteCurrency,
		label:     label,
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
	return balance.trade.Load()
}

/*
Reading is the latest cash/equity/unrealized map for the dashboard.
*/
func (balance *Balance) Reading() map[string]any {
	ptr := balance.reading.Load()

	if ptr == nil {
		return nil
	}

	return *ptr
}

/*
Publish pushes the cached reading to the dashboard as {"balance": {...}}
and under balances.<label> for the multi-book top bar.
*/
func (balance *Balance) Publish() {
	if ui.UI == nil {
		return
	}

	reading := balance.Reading()

	if reading == nil {
		return
	}

	label := balance.label
	if label == "" {
		label = "paper"
	}

	ui.UI.WriteJSON(map[string]any{
		"balance": reading,
		"balances": map[string]any{
			label: reading,
		},
	})
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

		balance.wallet.Store(kraken.NewBalanceFromMap(balanceMapFromSDK(resp.Result)))

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

	reading := map[string]any{
		"cash":       balance.cash(balance.Quote),
		"equity":     tradeBalance.Equity,
		"unrealized": tradeBalance.UnrealizedPnL,
	}
	balance.reading.Store(&reading)
	balance.Publish()

	return nil
}
