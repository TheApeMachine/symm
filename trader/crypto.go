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
	channelOHLC       = "ohlc"
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
	feeds      []types.Feed
	tick       *atomic.Int64
	tickBudget time.Duration
	planner    *strategy.Planner
	analyzer   *logic.Analyzer
	level3     *Level3
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
	feeds []types.Feed,
	instrument *Instrument,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	if len(feeds) < 5 {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"crypto: expected ticker, trade, ohlc, book, and level3 feeds",
			nil,
		)
	}

	level3, ok := feeds[4].(*Level3)

	if !ok {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"crypto: level3 feed must be a *Level3",
			nil,
		)
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
		feeds:      feeds,
		uiHub:      uiHub,
		tick:       &atomic.Int64{},
		tickBudget: viper.GetViper().GetDuration("cognitive.tick_budget"),
		analyzer:   analyzer,
		planner:    planner,
		level3:     level3,
	}

	api.On(channelInstrument, crypto.instrument.On)
	api.On(channelTicker, crypto.feeds[0].On)
	api.On(channelTrade, crypto.feeds[1].On)
	api.On(channelTrade, crypto.level3.OnTrade)
	api.On(channelOHLC, crypto.feeds[2].On)
	api.On(channelBook, crypto.feeds[3].On)
	api.On(channelLevel3, crypto.level3.On)

	errnie.Error(api.SubscribeInstruments())
	return crypto, nil
}

/*
Status returns the current status of the crypto runtime.
*/
func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) Run() error {
	go func() {
		errnie.Info("crypto subscribing to instrument")

		if err := crypto.instrument.Subscribe(); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				nil,
			))
		}

		for crypto.Status() != types.ERROR {
			tick := datura.Map[any]{
				"count":        0,
				"measurements": 0,
				"open":         0,
			}

			if crypto.booter.Ready(system.StagePreflight) {
				measurements := make([]*types.Measurement, 0)

				crypto.ready()

				for _, feed := range crypto.feeds {
					measures, err := feed.Measure()

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							err.Error(),
							nil,
						))
					}

					measurements = append(measurements, measures...)
				}
				tick["measurements"] = len(measurements)

				crypto.tick.Add(1)

				crypto.publish(datura.Map[any]{
					"measurements": measurements,
				})

				thesis := crypto.analyzer.PendingThesis()

				if err := crypto.analyzer.Measurements.Ingest(measurements); err != nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"crypto: measurement analysis failed",
						err,
					))

					crypto.status = types.ERROR
					return
				}

				if crypto.booter.Ready(system.StageWarmup) {
					intents, err := crypto.planner.Update(thesis)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"crypto: strategy planning failed",
							err,
						))
						crypto.status = types.ERROR

						return
					}

					positions := make([]broker.PositionData, 0)

					for _, p := range crypto.desk.Positions() {
						if p != nil && p.Data != nil {
							positions = append(positions, *p.Data)
						}
					}

					executions := crypto.desk.Executions()

					crypto.publish(datura.Map[any]{
						"intents":    intents,
						"positions":  positions,
						"executions": executions,
					})

					for _, intent := range intents {
						if intent.Action == strategy.ActionBuy {
							crypto.desk.Buy(intent.Symbol, 1.0, intent.Edge, false)
							continue
						}

						if intent.Action == strategy.ActionSell {
							crypto.desk.Sell(intent.Symbol)
						}
					}
				}
			}

			tick["count"] = crypto.tick.Add(1)
			tick["open"] = crypto.desk.OpenPositions()

			crypto.publish(datura.Map[any]{
				"tick": tick,
			})

			if tick["measurements"] == 0 {
				time.Sleep(crypto.tickBudget)
				continue
			}
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
ready promotes crypto to READY once the instrument cache, the price
feed, the desk, and every composed market feed are all ready. It is
idempotent and cheap to call every tick: once crypto is READY it
never re-evaluates, and until then it recomputes every dependency's
current status rather than accumulating a counter that has no way to
notice a feed that was ready and later is not.
*/
func (crypto *Crypto) ready() {
	if crypto.Status() == types.READY {
		return
	}

	if crypto.instrument.Status() != types.READY ||
		crypto.price.Status() != types.READY ||
		crypto.desk.Status() != types.READY {
		return
	}

	for _, feed := range crypto.feeds {
		if feed.Status() != types.READY {
			return
		}
	}

	crypto.status = types.READY
	errnie.Info("crypto ready")
}

/*
Close stops the trader and its composed resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()
	errnie.Error(crypto.level3.Close())

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
