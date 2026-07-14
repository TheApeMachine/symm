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
	api        *websocket.API
	instrument *Instrument
	tick       *atomic.Int64
	tickBudget time.Duration
	planner    *strategy.Planner
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
		instrument: instrument,
		uiHub:      uiHub,
		tick:       &atomic.Int64{},
		tickBudget: tickBudget,
		analyzer:   analyzer,
		planner:    planner,
		level3:     level3,
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
				thesis := crypto.planner.BeginTick()

				if crypto.level3 != nil {
					crypto.level3.Drain(thesis, crypto.analyzer, crypto.instrument)
				}

				thesis = crypto.planner.CompleteTick(thesis)
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
