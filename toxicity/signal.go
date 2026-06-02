package toxicity

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
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

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	tox.subscribers["raw"] = raw.Subscribe("toxicity:raw", 1024)

	activate.Boot("toxicity ready l3=" + fmt.Sprint(tox.l3Active))

	return tox
}

/*
Tick joins the live trade tape, ticker, L2 or L3 book events onto the shared Tracker.
When L3 credentials are configured, per-order events replace the L2 fallback path.
*/
func (tox *Toxicity) Tick() error {
	for {
		select {
		case <-tox.ctx.Done():
			return tox.ctx.Err()
		case message := <-tox.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TradesChannel:
				for _, trade := range errnie.Does(func() ([]market.TradeUpdate, error) {
					return market.DecodeTrades(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					tox.observeTrade(trade)
				}
			case public.TickerChannel:
				for _, ticker := range errnie.Does(func() ([]market.TickerUpdate, error) {
					return market.DecodeTickers(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
					tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)
					tox.publishMeasurement(ticker.Symbol)
				}
			case public.BookChannel:
				for _, update := range errnie.Does(func() ([]market.Book, error) {
					return market.DecodeBooks(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					if !tox.l3Active {
						tox.observeBook(update)
						tox.publishMeasurement(update.Symbol)
					}
				}
			}
			// TODO: handle level3 on tox.subscribers["level3"] when L3 feed is wired.
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

	activate.Once("toxicity:measurement")
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
