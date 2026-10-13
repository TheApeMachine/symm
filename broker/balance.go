package broker

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
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
	transport   Transport
	paper       *Paper
	Quote       string
	wallet      *kraken.Balance
	measurement *data.Measurement
	limiter     *rate.Limiter
	uiTee       runtime.Tee
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
	tee runtime.Tee,
) *Balance {
	balance := &Balance{
		System:    runtime.NewSystem(ctx, "balance"),
		transport: transport,
		Quote:     system.Cfg.Market.QuoteCurrency,
		uiTee:     tee,
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

func (balance *Balance) cash(asset string) *decimal.Decimal {
	if balance.wallet == nil {
		return nil
	}

	for _, entry := range balance.wallet.Data {
		if entry.Asset == asset {
			if entry.Available != nil {
				return entry.Available
			}

			return entry.Balance
		}
	}

	alias := krakenAssetAlias(asset)

	if alias != "" {
		for _, entry := range balance.wallet.Data {
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

func krakenAssetAlias(asset string) string {
	switch asset {
	case "USD":
		return "ZUSD"
	case "ZUSD":
		return "USD"
	case "EUR":
		return "ZEUR"
	case "ZEUR":
		return "EUR"
	case "GBP":
		return "ZGBP"
	case "ZGBP":
		return "GBP"
	case "CAD":
		return "ZCAD"
	case "ZCAD":
		return "CAD"
	case "JPY":
		return "ZJPY"
	case "ZJPY":
		return "JPY"
	case "BTC", "XBT":
		return "XXBT"
	case "XXBT":
		return "XBT"
	case "ETH":
		return "XETH"
	case "XETH":
		return "ETH"
	}

	return ""
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

		balance.wallet = wallet
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

		balance.wallet = kraken.NewBalanceFromMap(resp.Result)

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

	if tradeBalance == nil || balance.wallet == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[balance] trade balance or wallet unavailable",
			nil,
		))
	}

	now := time.Now()
	epoch := system.Epoch.Load()

	if epoch <= 0 {
		epoch = now.UnixNano()
	}

	measurement := data.NewMeasurement(
		epoch,
		"balance",
		"balance",
		now,
		now,
	)
	measurement.Write(balance.metrics(tradeBalance)...)

	balance.measurement = measurement
	balance.uiTee.Push(balance.measurement)
	return nil
}

func (balance *Balance) metrics(tradeBalance *kraken.TradeBalanceResult) []*data.Metric {
	metrics := make([]*data.Metric, 0, len(balance.wallet.Data)+3)

	if tradeBalance.UnrealizedPnL != nil {
		metrics = append(metrics, data.NewExactMetric(
			"unrealized",
			tradeBalance.UnrealizedPnL,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	if tradeBalance.Equity != nil {
		metrics = append(metrics, data.NewExactMetric(
			"equity",
			tradeBalance.Equity,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	quoteCash := balance.cash(balance.Quote)

	if quoteCash == nil {
		quoteCash = balance.cash("USD")
	}

	if quoteCash == nil && tradeBalance.AvailableCash != nil {
		quoteCash = tradeBalance.AvailableCash
	}

	if quoteCash != nil {
		metrics = append(metrics, data.NewExactMetric(
			"cash",
			quoteCash,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	for _, entry := range balance.wallet.Data {
		available := entry.Available

		if available == nil {
			available = entry.Balance
		}

		if available == nil {
			continue
		}

		if entry.Asset == balance.Quote || entry.Asset == krakenAssetAlias(balance.Quote) {
			continue
		}

		metrics = append(metrics, data.NewExactMetric(
			"balance:"+entry.Asset,
			available,
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	return metrics
}
