package toxicity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/market/settings"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

var toxicityDefaultBandEdges = []float64{0.5, 1.5}

/*
Toxicity tracks executed-flow book quality and publishes toxicity perspective
measurements while feeding IsToxic and NearTouchToxic for depthflow and fluid.
*/
type Toxicity struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	subscribers  map[string]*qpool.Subscriber
	tracker      *Tracker
	measurements *qpool.BroadcastGroup
	ui           *qpool.BroadcastGroup
	classifier   *adaptive.Classifier
	calibrator   *numeric.BandCalibrator
	l3Active     bool
	rawDump      *rawdump.Writer
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		toxicityDefaultBandEdges,
		[]float64{0, 1, 2},
		[]string{"hard_support", "liquidity_vacuum", "toxic_bluff"},
		[]float64{0.40, 0.35, 0.25},
		numeric.DefaultCalibratorConfig("strength"),
		"toxicity",
	)

	tox := &Toxicity{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tracker:    Default(),
		classifier: pooledCalibrator.Classifier,
		calibrator: pooledCalibrator.Calibrator,
		l3Active:   settings.L3Enabled(),
	}
	queueTTL := viper.GetDuration("system.queue.ttl")
	tox.measurements = bus.Group(pool, "measurements", queueTTL)
	tox.ui = bus.Group(pool, "ui", queueTTL)
	tox.subscribers = make(map[string]*qpool.Subscriber)
	tox.rawDump = rawdump.Open("toxicity")

	raw := bus.Group(pool, "raw", queueTTL)
	tox.subscribers["raw"] = raw.Subscribe("toxicity:raw", 1024)

	level3 := bus.Group(pool, "level3", queueTTL)
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

	frame, ok := signalpool.SocketMessageFromValue(message.Value)

	if !ok {
		return nil
	}

	switch frame.Channel {
	case public.TradesChannel:
		trades := signalpool.GetTrades(frame)

		for _, trade := range trades {
			tox.observeTrade(trade)
		}
	case public.TickerChannel:
		tickers := signalpool.GetTickers(frame)

		for _, ticker := range tickers {
			at, err := toxicityTickerTime(ticker)

			if err != nil {
				return err
			}

			tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
			tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)

			if err := tox.publishMeasurementAt(ticker.Symbol, at); err != nil {
				return fmt.Errorf("toxicity: publish %s: %w", ticker.Symbol, err)
			}
		}
	case public.BookChannel:
		if tox.l3Active {
			return nil
		}

		books := signalpool.GetBooks(frame)

		for _, update := range books {
			at, err := tox.observeBook(update)

			if err != nil {
				return err
			}

			if err := tox.publishMeasurementAt(update.Symbol, at); err != nil {
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

	envelope, ok := signalpool.SocketMessageFromValue(message.Value)

	if !ok {
		return nil
	}

	if envelope.Channel != public.Level3Channel {
		return nil
	}

	tox.l3Active = true

	if len(envelope.Data) == 0 {
		return nil
	}

	var updates []level3Update

	if err := json.Unmarshal(envelope.Data, &updates); err != nil {
		return fmt.Errorf("toxicity: level3 decode: %w", err)
	}

	now := time.Now()

	for _, update := range updates {
		if err := tox.observeLevel3Update(update, now); err != nil {
			return err
		}
	}

	return nil
}

type level3Update struct {
	Symbol string        `json:"symbol"`
	Bids   []level3Order `json:"bids"`
	Asks   []level3Order `json:"asks"`
}

type level3Order struct {
	Event      string    `json:"event"`
	OrderID    string    `json:"order_id"`
	LimitPrice float64   `json:"limit_price"`
	OrderQty   float64   `json:"order_qty"`
	Timestamp  time.Time `json:"timestamp"`
}

func (tox *Toxicity) observeLevel3Update(update level3Update, now time.Time) error {
	if update.Symbol == "" {
		return fmt.Errorf("toxicity: level3 symbol is required")
	}

	for _, order := range update.Bids {
		if err := tox.observeLevel3Order(update.Symbol, SideBid, order, now); err != nil {
			return err
		}
	}

	for _, order := range update.Asks {
		if err := tox.observeLevel3Order(update.Symbol, SideAsk, order, now); err != nil {
			return err
		}
	}

	return nil
}

func (tox *Toxicity) observeLevel3Order(
	symbol string,
	side byte,
	order level3Order,
	now time.Time,
) error {
	if order.Timestamp.IsZero() {
		return fmt.Errorf("toxicity: level3 %s timestamp is required", symbol)
	}

	tox.tracker.ApplyOrder(
		symbol,
		market.Pair{},
		order.Event,
		order.OrderID,
		side,
		order.LimitPrice,
		order.OrderQty,
		order.Timestamp,
		now,
	)

	return nil
}

func (tox *Toxicity) observeTrade(trade market.TradeUpdate) {
	tox.tracker.ObserveTrade(trade.Symbol, market.Pair{}, trade.Price, trade.Qty, trade.Timestamp)
}

func (tox *Toxicity) observeBook(update market.Book) (time.Time, error) {
	now, err := time.Parse(time.RFC3339Nano, update.Timestamp)

	if err != nil {
		return time.Time{}, fmt.Errorf("toxicity: book timestamp %s: %w", update.Symbol, err)
	}

	for _, level := range update.Bids {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideBid, level.Price, level.Qty, now)
	}

	for _, level := range update.Asks {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideAsk, level.Price, level.Qty, now)
	}

	return now, nil
}

func (tox *Toxicity) publishMeasurement(symbol string) error {
	return tox.publishMeasurementAt(symbol, time.Now())
}

func (tox *Toxicity) publishMeasurementAt(symbol string, at time.Time) error {
	measurement, err := tox.tracker.Measure(symbol, at)

	if err != nil {
		return err
	}

	if measurement.Source == types.SourceNone {
		return nil
	}

	measurement.Symbol = symbol

	if err := tox.rawDump.Write(rawRecord{
		Symbol:     measurement.Symbol,
		Category:   measurement.Category,
		Strength:   measurement.Strength,
		Confidence: measurement.Confidence,
		SNR:        measurement.SNR,
		Last:       measurement.Last,
		SpreadBPS:  measurement.SpreadBPS,
	}); err != nil {
		return err
	}

	if err := measurement.Send(tox.pool); err != nil {
		return err
	}

	if tox.ui != nil {
		telemetry, _ := numeric.ObserveGaugeTelemetry(
			tox.calibrator,
			tox.classifier,
			measurement.Strength,
			measurement.SNR,
		)
		tox.ui.Send(&qpool.QValue[any]{
			Value: numeric.GaugePayload(
				measurement.Source.String(),
				measurement.Symbol,
				measurement.Category,
				measurement,
				telemetry,
			),
		})
	}

	return nil
}

func (tox *Toxicity) Close() error {
	tox.cancel()
	return tox.rawDump.Close()
}

func midOf(row market.TickerUpdate) float64 {
	if row.Bid > 0 && row.Ask > 0 {
		return (row.Bid + row.Ask) / 2
	}

	return row.Last
}

func toxicityTickerTime(row market.TickerUpdate) (time.Time, error) {
	if row.Timestamp == "" {
		return time.Time{}, fmt.Errorf("toxicity: ticker timestamp is required for %s", row.Symbol)
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("toxicity: ticker timestamp %s is invalid for %s", row.Timestamp, row.Symbol)
}
