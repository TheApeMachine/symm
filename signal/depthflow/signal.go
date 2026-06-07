package depthflow

import (
	"context"
	"sync"
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
Signal detects multi-level order-book imbalance and depth-weighted flow pressure,
mapping book shape onto the weight-of-the-book perspective (LoadedImbalance /
SpoofTrap / BookThinning / DenseNeutrality). Toxic near-touch walls are excluded
via the shared toxicity tracker before distance-decay weighting.
*/
const rawSubscriberID = "signal/depthflow:raw"

var depthflowDefaultBandEdges = []float64{0.5, 1.5, 2.5}

type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		depthflowDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"dense_neutrality", "loaded_imbalance", "book_thinning", "spoof_trap"},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("strength"),
		"depthflow",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryDenseNeutrality,
		types.CategoryLoadedImbalance,
		types.CategoryBookThinning,
		types.CategorySpoofTrap,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/depthflow")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.BroadcastConsumer),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("depthflow"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/depthflow ready", "signal/depthflow")

	return signal
}

func (signal *Signal) state(symbol string) (*DepthSymbol, error) {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*DepthSymbol), nil
	}

	created, err := NewDepthSymbol(symbol)

	if err != nil {
		return nil, err
	}

	stored, _ := signal.symbols.LoadOrStore(symbol, created)

	return stored.(*DepthSymbol), nil
}

func (signal *Signal) Tick() error {
	for {
		message, err := signal.subscribers["raw"].Wait(signal.ctx)
		if err != nil {
			return err
		}

		if message == nil {
			continue
		}

		errnie.Debug("signal/depthflow: Tick()", "type", message.Type)

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		switch sm.Channel {
		case public.TradesChannel:
			trades := signalpool.GetTrades(sm)

			for _, trade := range trades {
				if err := signal.observeTrade(trade); err != nil {
					errnie.Error(err, "depthflow: observe trade %s", trade.Symbol)
					continue
				}
			}
		case public.TickerChannel:
			tickers := signalpool.GetTickers(sm)

			for _, ticker := range tickers {
				state, err := signal.state(ticker.Symbol)

				if err != nil {
					errnie.Error(err, "depthflow: state %s", ticker.Symbol)
					continue
				}

				state.FeedTicker(ticker)
			}
		case public.BookChannel:
			books := signalpool.GetBooks(sm)

			for _, delta := range books {
				state, err := signal.state(delta.Symbol)

				if err != nil {
					errnie.Error(err, "depthflow: state %s", delta.Symbol)
					continue
				}

				state.ApplyBook(delta)

				if err := signal.emit(delta.Symbol, time.Now().UTC()); err != nil {
					errnie.Error(err, "depthflow: emit %s", delta.Symbol)
					continue
				}
			}
		}
	}
}

// observeTrade folds one trade's aggressor side into depth-weighted pressure and
// emits the symbol's reading.
func (signal *Signal) observeTrade(trade market.TradeUpdate) error {
	sign := -1.0

	if trade.Side == "buy" {
		sign = 1.0
	}

	state, err := signal.state(trade.Symbol)

	if err != nil {
		return err
	}

	if _, err := state.PushTradePressure(sign); err != nil {
		return err
	}

	return signal.emit(trade.Symbol, trade.Timestamp)
}

func (signal *Signal) emit(symbol string, at time.Time) error {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	measurement, standout, err := raw.(*DepthSymbol).Measure()

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

	if err := types.AssignCategorySurpriseSNR(
		&measurement, signal.surpriseField, measurement.Category,
	); err != nil {
		return err
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: at.UnixNano(),
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
