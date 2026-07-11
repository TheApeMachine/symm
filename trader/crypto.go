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
	status            types.Status
	ctx               context.Context
	cancel            context.CancelFunc
	pool              *qpool.Q[any]
	tree              *dmt.Tree
	uiHub             *ui.Hub
	desk              *broker.Desk
	price             *broker.Price
	private           websocket.Conn
	public            websocket.Conn
	instrument        *Instrument
	universe          *Universe
	ticker            *Ticker
	feeds             []types.Feed
	chunker           *Chunker
	tick              *atomic.Int64
	lastTick          time.Time
	lastRebalance     time.Time
	rebalanceInterval time.Duration
	tickBudget        time.Duration
	planner           *Planner
	analyzer          *logic.Analyzer
	readyCount        *atomic.Int64
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
	orderBook := NewOrderBook(viper.GetInt("market.book_depth_levels"))
	level3Book := NewLevel3Book(viper.GetInt("market.book_depth_levels"))
	price := broker.NewPrice(private, public)
	desk := broker.NewDesk(private, public, uiHub.Messages)
	desk.SetPrice(price)
	ticker := NewTicker(pool, signal, uiHub)
	trade := NewTrade(pool, signal, uiHub)
	ohlc := NewOHLC(pool, signal, uiHub)
	book := NewBook(pool, signal, uiHub, instrument, orderBook)
	level3Feed := NewLevel3(pool, signal, uiHub, instrument, level3Book)

	crypto := &Crypto{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		public:     public,
		private:    private,
		desk:       desk,
		price:      price,
		instrument: instrument,
		universe:   NewUniverse(instrument, price, desk, orderBook, level3Book),
		ticker:     ticker,
		feeds:      []types.Feed{ticker, trade, ohlc, book, level3Feed},
		chunker: NewChunker(signal.CrossSection, map[string]types.Drainer{
			"ticker": ticker,
			"trade":  trade,
			"ohlc":   ohlc,
			"book":   book,
			"level3": level3Feed,
		}, []string{"ticker", "trade", "ohlc", "book", "level3"}),
		uiHub:             uiHub,
		tick:              &atomic.Int64{},
		rebalanceInterval: viper.GetDuration("market.universe.rebalance_interval"),
		tickBudget:        viper.GetDuration("cognitive.tick_budget"),
		analyzer:          logic.NewAnalyzer(tree, uiHub),
		readyCount:        &atomic.Int64{},
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
			crypto.rebalance()

			for _, feed := range crypto.feeds {
				crypto.ready(feed)
			}

			measurements, err := crypto.measure()

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					err.Error(),
					nil,
				))
			}

			if len(measurements) == 0 {
				crypto.planner.Update(nil)

				if crypto.tickBudget > 0 {
					time.Sleep(crypto.tickBudget)
				} else {
					time.Sleep(time.Millisecond) // Yield to avoid tight spin
				}

				continue
			}

			crypto.publishTick(len(measurements))
			crypto.planner.Update(crypto.analyzer.Update(measurements))
		}
	}()

	return nil
}

/*
measure drains every stream through the Chunker, merging them into
per-symbol EventChunks ordered by event time, stream, and sequence
before any signal runs, then dispatches every event against the one
immutable cross-section snapshot taken for this drain cycle.
*/
func (crypto *Crypto) measure() ([]*types.Measurement, error) {
	chunks, snapshot, err := crypto.chunker.Drain()

	if err != nil {
		return nil, err
	}

	return crypto.chunker.Measure(chunks, snapshot)
}

/*
publishTick forwards a throttled tick summary to the UI hub, gated to at
most 10 updates per second so a burst of measurements does not flood the
websocket channel.
*/
func (crypto *Crypto) publishTick(measurementCount int) {
	tickCount := crypto.tick.Add(1)

	if crypto.uiHub == nil || crypto.uiHub.Messages == nil {
		return
	}

	if time.Since(crypto.lastTick) < 100*time.Millisecond {
		return
	}

	crypto.lastTick = time.Now()

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{
		"tick": datura.Map[any]{
			"count":        tickCount,
			"measurements": measurementCount,
			"open":         crypto.desk.OpenPositions(),
		},
	}.Marshal():
	default:
	}
}

/*
rebalance re-scores the ticker observation tier and re-subscribes the
trade/book/level3 trading tier once the configured interval has elapsed.
Gating on an interval, rather than reacting to every tick, keeps
subscription churn bounded to a fixed cadence.
*/
func (crypto *Crypto) rebalance() {
	if crypto.rebalanceInterval <= 0 || time.Since(crypto.lastRebalance) < crypto.rebalanceInterval {
		return
	}

	crypto.lastRebalance = time.Now()

	if err := crypto.universe.Rebalance(crypto.ticker.Snapshot()); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, err.Error(), nil))
	}
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
