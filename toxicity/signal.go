package toxicity

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Toxicity tracks executed-flow book quality and publishes toxicity perspective
measurements while feeding IsToxic for depthflow and fluid.
*/
type Toxicity struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	subscribers  map[string]*qpool.Subscriber
	tracker      *Tracker
	measurements *qpool.BroadcastGroup
	l3Active     bool
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	tox := &Toxicity{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		tracker:  Default(),
		l3Active: market.Level3Available(),
	}
	tox.measurements = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	tox.subscribers = make(map[string]*qpool.Subscriber)

	for _, channel := range []string{"trade", "ticker", "book"} {
		broadcast := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		tox.subscribers[channel] = broadcast.Subscribe(channel, 128)
	}

	return tox
}

/*
Tick joins the live trade tape, ticker, L2 or L3 book events onto the shared Tracker.
When L3 credentials are configured, per-order events replace the L2 fallback path.
*/
func (tox *Toxicity) Tick() error {
	var level3 <-chan *market.Level3Update

	if tox.l3Active {
		symbols := viper.GetStringSlice("market.symbols")
		depthLevels := viper.GetInt("market.book_depth_levels")

		if depthLevels <= 0 {
			depthLevels = 10
		}

		level3 = market.NewLevel3Subscription(tox.ctx, tox.pool, depthLevels, symbols...)
	}

	for {
		select {
		case <-tox.ctx.Done():
			return tox.ctx.Err()
		case message := <-tox.subscribers["trade"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				trade, err := market.DecodeTrade(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				tox.observeTrade(trade)
			}
		case message := <-tox.subscribers["ticker"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				ticker, err := market.DecodeTicker(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
				tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)
				tox.publishMeasurement(ticker.Symbol)
			}
		case message := <-tox.subscribers["book"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				update, err := market.DecodeBook(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				if !tox.l3Active {
					tox.observeBook(update)
					tox.publishMeasurement(update.Symbol)
				}
			}
		case update, ok := <-level3:
			if !ok {
				level3 = nil

				continue
			}

			if update != nil {
				tox.observeLevel3(update)
			}
		}
	}
}

func (tox *Toxicity) observeTrade(trade market.TradeUpdate) {
	tox.tracker.ObserveTrade(trade.Symbol, market.Pair{}, trade.Price, trade.Qty, trade.Timestamp)
}

func (tox *Toxicity) observeBook(update market.Book) {
	now := time.Now()

	for _, level := range update.Bids {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideBid, level.Price, level.Qty, now)
	}

	for _, level := range update.Asks {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideAsk, level.Price, level.Qty, now)
	}
}

func (tox *Toxicity) publishMeasurement(symbol string) {
	now := time.Now()
	measurement, ok := tox.tracker.Measure(symbol, now)

	if !ok {
		return
	}

	measurement.Symbol = symbol

	tox.measurements.Send(&qpool.QValue[any]{Value: measurement})
}

func (tox *Toxicity) Close() error {
	tox.cancel()
	return nil
}

func midOf(row market.TickerUpdate) float64 {
	if row.Bid > 0 && row.Ask > 0 {
		return (row.Bid + row.Ask) / 2
	}

	return row.Last
}
