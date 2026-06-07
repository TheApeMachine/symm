package cvd

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
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

const (
	cvdWindow          = 15 * time.Minute
	minCVDFusedSamples = 12
	rawSubscriberID    = "signal/cvd:raw"
)

var cvdDefaultBandEdges = []float64{-0.10, 0.50, 2.00}

/*
Signal measuring executed-flow absorption (cumulative volume delta).

Every threshold is self-scaling on the input axes; classification bands are pooled
across the universe via BandCalibrator so illiquid pairs inherit a coherent
distribution instead of hyper-sensitive per-symbol quartiles.
*/
type cvdState struct {
	signed    *adaptive.Window
	gross     *adaptive.Window
	count     *adaptive.Window
	convBase  *adaptive.EMA
	volBase   *adaptive.EMA
	actBase   *adaptive.EMA
	driftBase *adaptive.EMA
	pipe      *numeric.Classed
	last      float64
}

type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	categories    map[string]types.CategoryType
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		cvdDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{
			"volume_starvation",
			"stochastic_balance",
			"hidden_absorption",
			"aggressive_drive",
		},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("fused"),
		"cvd",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryVolumeStarvation,
		types.CategoryStochasticBalance,
		types.CategoryHiddenAbsorption,
		types.CategoryAggressiveDrive,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/cvd")
		return nil
	}

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.BroadcastConsumer),
		categories: map[string]types.CategoryType{
			"volume_starvation":  types.CategoryVolumeStarvation,
			"stochastic_balance": types.CategoryStochasticBalance,
			"hidden_absorption":  types.CategoryHiddenAbsorption,
			"aggressive_drive":   types.CategoryAggressiveDrive,
		},
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("cvd"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, viper.GetDuration("system.queue.ttl"))
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", viper.GetDuration("system.queue.ttl"))
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", viper.GetDuration("system.queue.ttl"))

	errnie.Info("signal/cvd ready", "signal/cvd")

	return signal
}

func newCVDState(classifier *adaptive.Classifier) *cvdState {
	return &cvdState{
		signed:    adaptive.NewWindow(cvdWindow),
		gross:     adaptive.NewWindow(cvdWindow),
		count:     adaptive.NewWindow(cvdWindow),
		convBase:  adaptive.NewEMA(0),
		volBase:   adaptive.NewEMA(0),
		actBase:   adaptive.NewEMA(0),
		driftBase: adaptive.NewEMA(0),
		pipe: numeric.NewClassed(
			classifier,
		),
	}
}

func (state *cvdState) scale(value float64, base *adaptive.EMA) float64 {
	norm := base.Value()
	_, _ = base.Next(0, value)

	if norm <= 0 {
		return 1
	}

	return value / norm
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
			if err := signal.observe(trade); err != nil {
				errnie.Error(err, "cvd: observe %s", trade.Symbol)
				continue
			}
		}
		}
	}
}

func (signal *Signal) observe(trade market.TradeUpdate) error {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return nil
	}

	stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newCVDState(signal.classifier))
	state := stored.(*cvdState)

	signed := trade.Qty

	if trade.Side != "buy" {
		signed = -trade.Qty
	}

	nanos := float64(trade.Timestamp.UnixNano())
	state.signed.Next(0, nanos, signed, trade.Price)
	state.gross.Next(0, nanos, trade.Qty)
	state.count.Next(0, nanos, 1)
	state.last = trade.Price

	gross := state.gross.Sum()
	anchor := state.signed.Anchor()

	if gross <= 0 || anchor <= 0 {
		return nil
	}

	conviction := state.scale(math.Abs(state.signed.Sum()/gross), state.convBase)
	tempo := state.scale(state.count.Sum(), state.actBase)
	volume := state.scale(gross, state.volBase)
	activity := math.Sqrt(tempo * volume)
	drift := state.scale(math.Abs((state.last-anchor)/anchor), state.driftBase)
	fused := activity * conviction * (1 + drift)

	code, err := state.pipe.Push(fused)

	if err != nil {
		return err
	}

	signal.calibrator.Observe(state.pipe.Observation(), signal.classifier)

	telemetry := signal.calibrator.Snapshot(signal.classifier)
	telemetry.Observation = state.pipe.Observation()

	if telemetry.Samples < minCVDFusedSamples {
		return nil
	}

	category := signal.categories[state.pipe.Label(code)]
	clarity := state.pipe.Confidence()
	categoryStandout := state.pipe.Standout()

	measurement := types.Measurement{
		Symbol:     trade.Symbol,
		Source:     types.SourceCVD,
		Category:   category,
		Last:       trade.Price,
		Strength:   fused,
		Confidence: clarity,
	}

	if err := types.AssignCategorySurpriseSNR(&measurement, signal.surpriseField, category); err != nil {
		return err
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: trade.Timestamp.UnixNano(),
		Symbol:            trade.Symbol,
		Price:             trade.Price,
		Category:          measurement.Category,
		Signed:            state.signed.Sum(),
		Gross:             gross,
		Count:             state.count.Sum(),
		Conviction:        conviction,
		Tempo:             tempo,
		Volume:            volume,
		Activity:          activity,
		Drift:             drift,
		Fused:             fused,
		Standout:          categoryStandout,
		Confidence:        clarity,
		SNR:               measurement.SNR,
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
