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
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

const (
	channelInstrument = "instrument"
	channelTicker     = "ticker"
	channelTrade      = "trade"
	channelOHLC       = "ohlc"
	channelBook       = "book"
	channelLevel3     = "level3"
	channelBalances   = "balances"
	channelExecutions = "executions"
	channelOrders     = "orders"
	channelAddOrder   = "add_order"
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
	uiHub      *ui.Hub
	desk       *broker.Desk
	price      *broker.Price
	private    websocket.Conn
	public     websocket.Conn
	instrument *Instrument
	feeds      []types.Feed
	tick       *atomic.Int64
	lastTick   time.Time
	tickBudget time.Duration
	planner    *Planner
	analyzer   *logic.Analyzer
	readyCount *atomic.Int64
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	private websocket.Conn,
	public websocket.Conn,
	uiHub *ui.Hub,
	level3 websocket.Conn,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	signal := NewSignal(ctx)
	instrument := NewInstrument(pool, public, private, level3, uiHub)

	crypto := &Crypto{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		public:     public,
		private:    private,
		desk:       broker.NewDesk(private, public, uiHub.Messages),
		price:      broker.NewPrice(private, public),
		instrument: instrument,
		feeds: []types.Feed{
			NewTicker(pool, signal, uiHub),
			NewTrade(pool, signal, uiHub),
			NewOHLC(pool, signal, uiHub),
			NewBook(pool, signal, uiHub, instrument),
			NewLevel3(pool, signal, uiHub),
		},
		uiHub:      uiHub,
		tick:       &atomic.Int64{},
		tickBudget: viper.GetDuration("cognitive.tick_budget"),
		analyzer:   logic.NewAnalyzer(tree, uiHub),
		readyCount: &atomic.Int64{},
	}

	crypto.planner = NewPlanner(crypto.desk, crypto.price, uiHub)

	public.On(channelInstrument, crypto.instrument.On)
	public.On(channelTicker, crypto.feeds[0].On)
	public.On(channelTrade, crypto.feeds[1].On)
	public.On(channelOHLC, crypto.feeds[2].On)
	public.On(channelBook, crypto.feeds[3].On)
	level3.On(channelLevel3, crypto.feeds[4].On)

	errnie.Error(public.Client().SubInstruments())
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
			var totalMeasurements int

			for _, feed := range crypto.feeds {
				measurements := make([]*types.Measurement, 0)

				crypto.ready(feed)

				feedMeasurements, err := feed.Measure()
				if err != nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						err.Error(),
						nil,
					))
				} else {
					measurements = append(measurements, feedMeasurements...)
				}

				if len(measurements) > 0 {
					totalMeasurements += len(measurements)
					tickCount := crypto.tick.Add(1)

					if crypto.uiHub != nil && crypto.uiHub.Messages != nil {
						// Time-based throttle for tick updates (max 10 fps) to avoid
						// flooding websocket channel.
						if time.Since(crypto.lastTick) >= 100*time.Millisecond {
							crypto.lastTick = time.Now()
							select {
							case crypto.uiHub.Messages <- datura.Map[any]{
								"tick": datura.Map[any]{
									"count":        tickCount,
									"measurements": len(measurements),
									"open":         crypto.desk.OpenPositions(),
								},
							}.Marshal():
							default:
							}
						}
					}

					// if crypto.status == types.READY {
					crypto.planner.Update(
						crypto.analyzer.Update(measurements),
					)
					// }
				}
			}

			if totalMeasurements == 0 {
				crypto.planner.Update(nil)
				if crypto.tickBudget > 0 {
					time.Sleep(crypto.tickBudget)
				} else {
					time.Sleep(time.Millisecond) // Yield to avoid tight spin
				}
			}
		}
	}()

	return nil
}

/*
ready returns the number of feeds that are ready.
*/
func (crypto *Crypto) ready(feed types.Feed) {
	if crypto.Status() == types.READY {
		return
	}

	allReady := true
	for _, f := range crypto.feeds {
		if f.Status() != types.READY {
			allReady = false
			break
		}
	}

	if allReady {
		crypto.status = types.READY
		errnie.Info("crypto ready")
	}
}

/*
Close stops the trader and its composed signal resources.
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
