package broker

import (
	"context"

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
The venue's own request budgets bound how often Update may run.

Live: Kraken's private REST counter decays by 0.33 per second on the Starter
tier, the lowest tier any account has, and Update spends two points (Balance
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

	// Startup needs the venue's account state, so the first read is retried
	// until it succeeds, each attempt spaced by the venue's request budget.
	for {
		if err := balance.limiter.Wait(ctx); err != nil {
			balance.Transition(runtime.ERROR)
			return balance
		}

		err := balance.Update()

		if err == nil {
			break
		}

		errnie.Error(errnie.Err(
			errnie.BadGateway, "[balance] initial account read failed, retrying", err,
		))
	}

	go balance.watch()

	balance.Transition(runtime.READY)
	return balance
}

func (balance *Balance) cash(asset string) *decimal.Decimal {
	for _, wallet := range balance.wallet.Data {
		if wallet.Asset == asset {
			return wallet.Available
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

		if err := balance.Update(); err != nil {
			errnie.Error(err)
		}
	}
}

/*
Update refreshes the holistic account state atomically.
*/
func (balance *Balance) Update() (err error) {
	var tradeBalance *kraken.TradeBalanceResult

	if balance.paper != nil {
		wallet, err := balance.paper.Balances()

		if err != nil {
			return errnie.Error(err)
		}

		if wallet == nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[balance] empty paper balance response",
				nil,
			))
		}

		tradeBalance, err = balance.paper.TradeBalance()

		if err != nil {
			return errnie.Error(err)
		}
	} else {
		client, err := kraken.NewAuthenticatedREST()

		if err != nil {
			return errnie.Error(err)
		}

		resp, err := client.Balances()

		if err != nil {
			return errnie.Error(err)
		}

		if resp == nil || resp.Result == nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[balance] empty balance response",
				nil,
			))
		}

		balance.wallet = kraken.NewBalanceFromMap(resp.Result)
		var result *spot.Response[kraken.TradeBalanceResult]

		// No asset parameter: Kraken values the trade balance in its documented
		// default asset, ZUSD.
		result, err = spot.Call[kraken.TradeBalanceResult](client, spot.RequestOptions{
			Auth:   true,
			Method: "POST",
			Path:   system.Cfg.WebSocket.Endpoints.TradeBalance,
		})

		if err != nil {
			return errnie.Error(err)
		}

		tradeBalance = &result.Result
	}

	metrics := []*data.Metric{
		data.NewMetric(
			"unrealized",
			tradeBalance.UnrealizedPnL.Float64(),
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		),
		data.NewMetric(
			"equity",
			tradeBalance.Equity.Float64(),
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		),
	}

	for _, balance := range balance.wallet.Data {
		metrics = append(metrics, data.NewMetric(
			"cash",
			balance.Available.Float64(),
			data.UnitCurrency,
			data.TimescaleInstantaneous,
		))
	}

	balance.measurement = data.NewMeasurement(
		0,
		"balance",
		"balance",
		system.SeqIdx.Add(1),
		system.Tick.Load(),
	).Write(metrics...)

	balance.uiTee.Push(balance.measurement)

	return nil
}
