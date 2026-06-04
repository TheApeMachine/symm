package toxicity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	signalpool "github.com/theapemachine/symm/signal"
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
		l3Active: viper.GetBool("trading.level3.enabled"),
	}
	tox.measurements = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	tox.subscribers = make(map[string]*qpool.Subscriber)

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	tox.subscribers["raw"] = raw.Subscribe("toxicity:raw", 1024)

	level3 := pool.CreateBroadcastGroup("level3", 10*time.Millisecond)
	tox.subscribers["level3"] = level3.Subscribe("toxicity:level3", 4096)

	errnie.Info("toxicity ready l3="+fmt.Sprint(tox.l3Active), "toxicity")

	return tox
}

/*
Tick joins the live trade tape, ticker, L2 or L3 book events onto the shared Tracker.
When L3 credentials are configured, per-order events replace the L2 fallback path.
*/
func (tox *Toxicity) Tick() error {
	level3In := tox.subscribers["level3"]

	for {
		select {
		case <-tox.ctx.Done():
			return tox.ctx.Err()
		case message := <-tox.subscribers["raw"].Incoming:
			if err := tox.handleRaw(message); err != nil {
				errnie.Error(err, "toxicity: handle raw")
				continue
			}
		case message := <-level3In.Incoming:
			if err := tox.handleLevel3(message); err != nil {
				errnie.Error(err, "toxicity: handle level3")
				continue
			}
		}
	}
}

func (tox *Toxicity) handleRaw(message *qpool.QValue[any]) error {
	if message == nil || message.Value == nil {
		return nil
	}

	envelope, ok := message.Value.(map[string]any)

	if !ok {
		return nil
	}

	channel, _ := envelope["channel"].(string)
	rawData, _ := envelope["data"].(json.RawMessage)
	frame := &public.SocketMessage{Channel: channel, Data: rawData}

	switch channel {
	case public.TradesChannel:
		trades := signalpool.GetTrades(frame)

		for _, trade := range trades {
			tox.observeTrade(trade)
		}
	case public.TickerChannel:
		tickers := signalpool.GetTickers(frame)

		for _, ticker := range tickers {
			tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
			tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)

			if err := tox.publishMeasurement(ticker.Symbol); err != nil {
				return fmt.Errorf("toxicity: publish %s: %w", ticker.Symbol, err)
			}
		}
	case public.BookChannel:
		if tox.l3Active {
			return nil
		}

		books := signalpool.GetBooks(frame)

		for _, update := range books {
			tox.observeBook(update)

			if err := tox.publishMeasurement(update.Symbol); err != nil {
				return fmt.Errorf("toxicity: publish %s: %w", update.Symbol, err)
			}
		}
	}

	return nil
}

func (tox *Toxicity) handleLevel3(message *qpool.QValue[any]) error {
	if message == nil || message.Value == nil {
		return nil
	}

	envelope, ok := message.Value.(*public.SocketMessage)

	if !ok {
		return nil
	}

	switch envelope.Type {
	case "orders":
		orders := signalpool.GetOrders(envelope)
		for _, order := range orders {
			fmt.Println(order)
		}
	}

	ch := envelope.Channel
	if ch != public.Level3Channel {
		return nil
	}

	tox.l3Active = true

	orders := signalpool.GetOrders(envelope)

	for _, order := range orders {
		fmt.Println(order)
		//tox.observeLevel3(&order)
	}

	return nil
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
