package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

const (
	channelInstrument = "instrument"
	channelTicker     = "ticker"
	channelTrade      = "trade"
	channelBook       = "book"
	channelLevel3     = "level3"
)

/*
Crypto is the simple trading runtime.
It consumes market and private frames, publishes UI frames,
and delegates measurement to Signal.
*/
type Crypto struct {
	booter     *system.Booter
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	uiHub      chan []byte
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	api        *websocket.API
	instrument *Instrument
	tick       *atomic.Int64
	tickBudget time.Duration
	planner    *strategy.Planner
	postMortem *strategy.PostMortem
	analyzer   *logic.Analyzer
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	booter *system.Booter,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
	desk *broker.Desk,
	uiHub chan []byte,
	instrument *Instrument,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	tickBudget := viper.GetViper().GetDuration("cognitive.tick_budget")

	if tickBudget <= 0 {
		tickBudget = 10 * time.Millisecond
	}

	crypto := &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		booter:     booter,
		status:     types.INITIALIZING,
		api:        api,
		desk:       desk,
		price:      price,
		balance:    balance,
		instrument: instrument,
		uiHub:      uiHub,
		tick:       &atomic.Int64{},
		tickBudget: tickBudget,
		analyzer:   analyzer,
		planner:    planner,
		postMortem: &strategy.PostMortem{},
	}

	return crypto, nil
}

func (crypto *Crypto) Initialize() error {
	errnie.Info("initializing crypto")

	crypto.status = types.READY
	return nil
}

/*
Status returns the current status of the crypto runtime.
*/
func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) Run() error {
	go func() {
		errnie.Info("crypto runtime started")

		for crypto.Status() != types.ERROR {
			if crypto.booter.Ready(system.StageWarmup) {
				if crypto.instrument != nil {
					if _, err := crypto.instrument.Activate(); err != nil {
						crypto.status = types.ERROR
						errnie.Error(err)

						continue
					}
				}

				thesis := crypto.planner.Update()
				crypto.trade(thesis)
				thesis.Publish()
			}

			crypto.publish(datura.Map[any]{
				"tick": datura.Map[any]{
					"count": crypto.tick.Add(1),
				},
			})

			time.Sleep(crypto.tickBudget)
		}
	}()

	return nil
}

/*
trade supplies current broker constraints to strategy, then submits each
selected Intent unchanged through Desk for exchange validation and execution.
*/
func (crypto *Crypto) trade(thesis *types.Thesis) {
	for symbol, lifecycle := range crypto.desk.PostExit(thesis) {
		if err := crypto.postMortem.Evaluate(lifecycle, symbol); err != nil {
			errnie.Error(err)

			continue
		}

		if err := crypto.desk.Finalize(symbol, lifecycle); err != nil {
			errnie.Error(err)
		}
	}

	if len(thesis.Forecasts) == 0 {
		return
	}

	fees := make(map[string]float64, len(thesis.Forecasts))

	for _, forecast := range thesis.Forecasts {
		fee, err := crypto.price.FeeFraction(forecast.Symbol)

		if err != nil {
			errnie.Error(err)

			continue
		}

		fees[forecast.Symbol] = fee.Float64()
	}

	available, err := crypto.balance.AvailableQuote()

	if err != nil {
		available = 0
	}

	intents := crypto.planner.Decide(
		thesis,
		crypto.desk.Exposures(),
		fees,
		available,
		crypto.desk.Slots(),
	)

	for _, intent := range intents {
		decision := intent.Selected()
		pair, err := crypto.instrument.Pair(decision.Symbol)

		if err != nil {
			errnie.Error(err)

			continue
		}

		if err := crypto.desk.Execute(intent, pair); err != nil {
			errnie.Error(err)
		}
	}
}

func (crypto *Crypto) publish(mapping datura.Map[any]) {
	select {
	case crypto.uiHub <- mapping.Marshal():
	default:
	}
}

/*
Close stops the trader and its composed resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
