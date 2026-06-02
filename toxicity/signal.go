package toxicity

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
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
				trades, err := market.DecodeTrades(&envelope)

				if err != nil {
					return fmt.Errorf("toxicity: decode trades: %w", err)
				}

				for _, trade := range trades {
					tox.observeTrade(trade)
				}
			case public.TickerChannel:
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					return fmt.Errorf("toxicity: decode tickers: %w", err)
				}

				for _, ticker := range tickers {
					tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
					tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)

					if err := tox.publishMeasurement(ticker.Symbol); err != nil {
						return fmt.Errorf("toxicity: publish %s: %w", ticker.Symbol, err)
					}
				}
			case public.BookChannel:
				books, err := market.DecodeBooks(&envelope)

				if err != nil {
					return fmt.Errorf("toxicity: decode books: %w", err)
				}

				for _, update := range books {
					if !tox.l3Active {
						tox.observeBook(update)

						if err := tox.publishMeasurement(update.Symbol); err != nil {
							return fmt.Errorf("toxicity: publish %s: %w", update.Symbol, err)
						}
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

func (tox *Toxicity) publishMeasurement(symbol string) error {
	now := time.Now()
	measurement, err := tox.tracker.Measure(symbol, now)

	if err != nil {
		return err
	}

	if measurement.Source == perspectives.SourceNone {
		return nil
	}

	measurement.Symbol = symbol

	activate.Once("toxicity:measurement")
	tox.measurements.Send(&qpool.QValue[any]{Value: measurement})

	return nil
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
