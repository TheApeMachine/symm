package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
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
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q[any]
	tree       *dmt.Tree
	uiHub      chan []byte
	desk       *broker.Desk
	price      *broker.Price
	api        *websocket.API
	instrument *Instrument
	feeds      []types.Feed
	tick       *atomic.Int64
	tickBudget time.Duration
	planner    *Planner
	analyzer   *logic.Analyzer
	level3Book *Level3Book
	readyCount *atomic.Int64
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
	uiHub chan []byte,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	signal := NewSignal(ctx)
	instrument := NewInstrument(api, uiHub)

	crypto := &Crypto{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		api:        api,
		desk:       broker.NewDesk(api, price, balance, uiHub),
		price:      price,
		instrument: instrument,
		feeds: []types.Feed{
			NewTicker(signal, uiHub),
			NewTrade(signal, uiHub),
			NewOHLC(signal, uiHub),
			NewBook(signal, uiHub, instrument),
			NewLevel3(signal, uiHub),
		},
		uiHub:      uiHub,
		tick:       &atomic.Int64{},
		tickBudget: viper.GetDuration("cognitive.tick_budget"),
		analyzer:   logic.NewAnalyzer(tree, uiHub),
		level3Book: NewLevel3Book(10),
		readyCount: &atomic.Int64{},
	}

	crypto.planner = NewPlanner(crypto.desk, crypto.price, uiHub)

	api.On(channelInstrument, crypto.instrument.On)
	api.On(channelTicker, crypto.feeds[0].On)
	api.On(channelTrade, crypto.feeds[1].On)
	api.On(channelOHLC, crypto.feeds[2].On)
	api.On(channelBook, crypto.feeds[3].On)
	api.On(channelLevel3, crypto.feeds[4].On)

	errnie.Error(api.SubscribeInstruments())
	return crypto, nil
}

/*
Status returns the current status of the crypto runtime.
*/
func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) Run() (err error) {
	go func() {
		errnie.Info("crypto subscribing to instrument")

		for crypto.instrument.Status() != types.READY {
			if err = crypto.instrument.Subscribe(); err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					err.Error(),
					nil,
				))

				return
			}

			time.Sleep(10 * time.Millisecond)
		}

		for crypto.status != types.ERROR {
			measurements := make([]*types.Measurement, 0)

			for _, feed := range crypto.feeds {
				crypto.ready(feed)

				if level3Feed, ok := feed.(*Level3); ok {
					measurements = append(measurements, errnie.Does(func() (
						[]*types.Measurement, error,
					) {
						return level3Feed.Measure()
					}).Or(func(err error) {
						errnie.Error(errnie.Err(
							errnie.Validation,
							err.Error(),
							nil,
						))
					}).Value()...)

					for _, row := range level3Feed.PopulationRows() {
						pricePrecision := 8
						qtyPrecision := 8

						if pair, err := crypto.instrument.Pair(row.Symbol); err == nil {
							pricePrecision = pair.PricePrecision
							qtyPrecision = pair.QtyPrecision
						}

						crypto.planner.Update(
							crypto.analyzer.IngestLevel3(row, pricePrecision, qtyPrecision, crypto.level3Book),
						)
					}

					continue
				}

				measurements = append(measurements, errnie.Does(func() (
					[]*types.Measurement, error,
				) {
					return feed.Measure()
				}).Or(func(err error) {
					errnie.Error(errnie.Err(
						errnie.Validation,
						err.Error(),
						nil,
					))
				}).Value()...)
			}

			crypto.tick.Add(1)

			select {
			case crypto.uiHub <- datura.Map[any]{
				"tick": datura.Map[any]{
					"count":        crypto.tick.Load(),
					"measurements": len(measurements),
					"open":         crypto.desk.OpenPositions(),
				},
			}.Marshal():
			default:
			}

			if crypto.status == types.READY && len(measurements) > 0 {
				crypto.planner.Update(map[string]*strategy.Thesis{})
			}
		}
	}()

	return nil
}

/*
ready returns the number of feeds that are ready.
*/
func (crypto *Crypto) ready(feed types.Feed) {
	readyCount := crypto.readyCount.Load()

	if readyCount < int64(len(crypto.feeds)) && crypto.Status() != types.READY {
		if feed.Status() == types.READY {
			crypto.readyCount.Add(1)
		}

		if crypto.readyCount.Load() == int64(len(crypto.feeds)) {
			crypto.status = types.READY
			errnie.Info("crypto ready")
		}
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
