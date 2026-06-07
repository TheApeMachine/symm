package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

/*
Signal tracks book/trade microstructure decay and classifies the dominant
exhaustion mode onto the exhaustion perspective. Exit timing itself is decided
at the perspective layer (ActionStopLoss / ActionExit); this signal only
publishes the classified reading.
*/
const rawSubscriberID = "signal/exhaust:raw"

var exhaustDefaultBandEdges = []float64{0.5, 1.5, 2.5}

type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	history       *historyStore
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		exhaustDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"thermal_exhaustion", "fragile_expansion", "mechanical_collapse", "active_reversal"},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("strength"),
		"exhaust",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryThermalExhaustion,
		types.CategoryFragileExpansion,
		types.CategoryMechanicalCollapse,
		types.CategoryActiveReversal,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/exhaust")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.BroadcastConsumer),
		history:       newHistoryStore(),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("exhaust"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/exhaust ready", "signal/exhaust")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		message, err := signal.subscribers["raw"].Wait(signal.ctx)

		if err != nil {
		return err
		}

		if message == nil || message.Value == nil {
		continue
		}

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
		continue
		}

		switch sm.Channel {
		case public.TradesChannel:
			trades := signalpool.GetTrades(sm)

			for _, trade := range trades {
				sign := -1.0

				if trade.Side == "buy" {
					sign = 1.0
				}

				signal.history.observe(trade.Symbol, 0, 0, 0, 0, sign, 0, trade.Price)

				if err := signal.emit(trade.Symbol); err != nil {
					errnie.Error(err, "exhaust: emit %s", trade.Symbol)
					continue
				}
			}
		case public.TickerChannel:
			tickers := signalpool.GetTickers(sm)

			for _, ticker := range tickers {
				signal.history.observe(ticker.Symbol, 0, 0, 0, 0, 0, 0, ticker.Last)

				if err := signal.emit(ticker.Symbol); err != nil {
					errnie.Error(err, "exhaust: emit %s", ticker.Symbol)
					continue
				}
			}
		case public.BookChannel:
			books := signalpool.GetBooks(sm)

			for _, delta := range books {
				signal.observeBook(delta)

				if err := signal.emit(delta.Symbol); err != nil {
					errnie.Error(err, "exhaust: emit %s", delta.Symbol)
					continue
				}
			}
		}
	}
}

// observeBook folds one book delta's depth, spread, and imbalance into history.
func (signal *Signal) observeBook(delta market.Book) {
	bidDepth := 0.0
	askDepth := 0.0

	for _, level := range delta.Bids {
		bidDepth += level.Qty
	}

	for _, level := range delta.Asks {
		askDepth += level.Qty
	}

	spreadBPS := 0.0
	imbalance := 0.0
	mid := 0.0

	if len(delta.Bids) > 0 && len(delta.Asks) > 0 {
		bid := delta.Bids[0].Price
		ask := delta.Asks[0].Price
		mid = (bid + ask) / 2

		if mid > 0 {
			spreadBPS = (ask - bid) / mid * 10000
		}

		total := delta.Bids[0].Qty + delta.Asks[0].Qty

		if total > 0 {
			imbalance = (delta.Bids[0].Qty - delta.Asks[0].Qty) / total
		}
	}

	signal.history.observe(
		delta.Symbol, bidDepth, askDepth, bidDepth+askDepth, spreadBPS, 0, imbalance, mid,
	)
}

// emit publishes the exhaustion reading for the one symbol an event touched.
func (signal *Signal) emit(symbol string) error {
	measurement, standout, err := signal.history.measure(symbol)

	if err != nil {
		return err
	}

	if measurement.Source == types.SourceNone {
		return nil
	}

	measurement.Symbol = symbol

	telemetry, _ := numeric.ObserveGaugeTelemetry(
		signal.calibrator,
		signal.classifier,
		measurement.Strength,
		standout,
	)

	if err := types.AssignCategorySurpriseSNR(&measurement, signal.surpriseField, measurement.Category); err != nil {
		return err
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: time.Now().UTC().UnixNano(),
		Symbol:            measurement.Symbol,
		Category:          measurement.Category,
		Strength:          measurement.Strength,
		Confidence:        measurement.Confidence,
		SNR:               measurement.SNR,
		Standout:          standout,
		Last:              measurement.Last,
		SpreadBPS:         measurement.SpreadBPS,
	}); err != nil {
		return err
	}

	if err := measurement.Send(signal.pool); err != nil {
		return err
	}

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
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

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
