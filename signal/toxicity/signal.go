package toxicity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	pool         *qpool.Q[any]
	subscribers  map[string]*qpool.BroadcastConsumer
	tracker      *Tracker
	measurements *qpool.BroadcastGroup
	ui           *qpool.BroadcastGroup
	classifier   *adaptive.Classifier
	calibrator   *numeric.BandCalibrator
	l3Active     bool
	rawDump      *rawdump.Writer
}

func NewToxicity(ctx context.Context, pool *qpool.Q[any]) *Toxicity {
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
	tox.measurements = pool.CreateBroadcastGroup("measurements", queueTTL)
	tox.ui = pool.CreateBroadcastGroup("ui", queueTTL)
	tox.subscribers = make(map[string]*qpool.BroadcastConsumer)
	tox.rawDump = rawdump.Open("toxicity")

	raw := pool.CreateBroadcastGroup("raw", queueTTL)
	tox.subscribers["raw"] = raw.Subscribe("toxicity:raw", 1024)

	level3 := pool.CreateBroadcastGroup("level3", queueTTL)
	tox.subscribers["level3"] = level3.Subscribe("toxicity:level3", 4096)

	errnie.Info("toxicity ready l3="+fmt.Sprint(tox.l3Active), "toxicity")

	return tox
}

/*
Tick joins the live trade tape, ticker, L2 or L3 book events onto the shared Tracker.
When L3 credentials are configured, per-order events replace the L2 fallback path.
*/
func (tox *Toxicity) Tick() error {
	// The level3 stream only carries data when authenticated L3 is configured;
	// without credentials NOTHING ever publishes to it. The previous loop
	// chained the two Waits serially, so the first nil from raw dropped it into
	// level3.Wait — a park on a channel with no publisher — and toxicity froze
	// for the rest of the session (zero rows after 2026-06-06 22:57). Each
	// subscription gets its own loop.
	go tox.drainLevel3()

	raw := tox.subscribers["raw"]

	for {
		message, err := raw.Wait(tox.ctx)
		if err != nil {
			return err
		}

		if message == nil {
			continue
		}

		errnie.Debug("toxicity: Tick()", "type", message.Type)

		if err := tox.handleRaw(message); err != nil {
			errnie.Error(err, "toxicity: handle raw")
		}
	}
}

func (tox *Toxicity) drainLevel3() {
	level3 := tox.subscribers["level3"]

	for {
		message, err := level3.Wait(tox.ctx)
		if err != nil {
			return
		}

		if message == nil {
			continue
		}

		errnie.Debug("toxicity: Tick()", "type", message.Type)

		if err := tox.handleLevel3(message); err != nil {
			errnie.Error(err, "toxicity: handle level3")
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

		// Per-frame tolerance: one malformed frame must not starve the rest of
		// the batch (a returned error here used to abort every remaining symbol).
		for _, ticker := range tickers {
			at, err := toxicityTickerTime(ticker)

			if err != nil {
				errnie.Error(err, "toxicity: ticker time")
				continue
			}

			tox.tracker.ObserveMid(ticker.Symbol, market.Pair{}, midOf(ticker))
			tox.tracker.ObserveLast(ticker.Symbol, market.Pair{}, ticker.Last)

			if err := tox.publishMeasurementAt(ticker.Symbol, at); err != nil {
				errnie.Error(fmt.Errorf("toxicity: publish %s: %w", ticker.Symbol, err))
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
				errnie.Error(err, "toxicity: book")
				continue
			}

			if err := tox.publishMeasurementAt(update.Symbol, at); err != nil {
				errnie.Error(fmt.Errorf("toxicity: publish %s: %w", update.Symbol, err))
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
	// Kraken v2 book SNAPSHOTS carry no timestamp field (deltas do). Requiring
	// one rejected every snapshot at the top of the batch; arrival time is the
	// honest stand-in for a frame that just crossed the wire.
	now := time.Now()

	if raw := strings.TrimSpace(update.Timestamp); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)

		if err != nil {
			return time.Time{}, fmt.Errorf("toxicity: book timestamp %s: %w", update.Symbol, err)
		}

		now = parsed
	} else if !update.IsSnapshot() {
		return time.Time{}, fmt.Errorf("toxicity: book timestamp is required for %s update", update.Symbol)
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
