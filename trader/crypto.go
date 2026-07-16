package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
	done       chan struct{}
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	api        *websocket.API
	instrument *broker.Instrument
	tree       *dmt.Tree
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
	instrument *broker.Instrument,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	tree *dmt.Tree,
	thesis *types.Thesis,
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
		tree:       tree,
		tick:       &atomic.Int64{},
		tickBudget: tickBudget,
		analyzer:   analyzer,
		planner:    planner,
		postMortem: &strategy.PostMortem{},
	}
	crypto.tick.Store(thesis.Tick)

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
	crypto.done = make(chan struct{})

	go func() {
		defer close(crypto.done)
		errnie.Info("crypto runtime started")

		for crypto.Status() != types.ERROR {
			select {
			case <-crypto.ctx.Done():
				return
			default:
			}

			if crypto.booter.Ready(system.StageWarmup) {
				if crypto.instrument != nil {
					if _, err := crypto.instrument.Activate(); err != nil {
						crypto.status = types.ERROR
						errnie.Error(err)

						continue
					}
				}

				thesis := crypto.planner.Update()
				thesis.Tick = crypto.tick.Add(1)
				crypto.trade(thesis)
				encoded, err := sonic.Marshal(thesis)

				if err != nil {
					crypto.status = types.ERROR
					errnie.Error(err)
					continue
				}

				if _, _, err := crypto.tree.Insert([]byte(types.ThesisKey), encoded); err != nil {
					crypto.status = types.ERROR
					errnie.Error(err)
					continue
				}

				thesis.Publish()
			}

			time.Sleep(crypto.tickBudget)
		}
	}()

	return nil
}

/*
trade supplies current broker constraints to strategy, then submits each
selected order through Desk for exchange validation and execution.
*/
func (crypto *Crypto) trade(thesis *types.Thesis) {
	thesis.Positions = thesis.Positions[:0]

	for _, position := range crypto.desk.Positions() {
		if position.Stop == nil {
			continue
		}

		holding, err := crypto.balance.Holding(position.Stop.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		thesis.Positions = append(thesis.Positions, holding)
	}

	if len(thesis.Forecasts) == 0 {
		return
	}

	fees := make(map[string]float64, len(thesis.Forecasts))

	for index := range thesis.Forecasts {
		forecast := &thesis.Forecasts[index]
		fee, err := crypto.price.FeeRate(forecast.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if fee.Fee == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"taker fee is missing for "+forecast.Symbol,
				nil,
			))
			continue
		}

		fraction := fee.Fee.Div(decimal.NewFromInt64(100))
		forecast.ExpectedFees = fraction.Mul(decimal.NewFromInt64(2)).Float64()
		forecast.FrictionReady = true
		fees[forecast.Symbol] = fraction.Float64()
	}

	available, err := crypto.balance.AvailableQuote()

	if err != nil {
		errnie.Error(err)
		available = 0
	}

	thesis = crypto.planner.Decide(
		thesis,
		fees,
		available,
		viper.GetViper().GetInt("trading.slots.normal"),
	)

	for _, order := range thesis.Orders {
		if order.Description == nil {
			continue
		}

		switch order.Description.Type {
		case "enter":
			pair, err := crypto.instrument.Pair(order.Description.Pair)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if err := crypto.desk.Buy(order, &pair); err != nil {
				errnie.Error(err)
			}
		case "exit", "reduce":
			if err := crypto.desk.Sell(order); err != nil {
				errnie.Error(err)
			}
		default:
			errnie.Error(errnie.Err(
				errnie.Validation,
				"unexpected order type "+order.Description.Type,
				nil,
			))
		}
	}
}

/*
Close stops the trader and its composed resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.done != nil {
		<-crypto.done
	}

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
