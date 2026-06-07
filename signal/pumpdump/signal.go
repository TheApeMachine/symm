package pumpdump

import (
	"context"
	"errors"
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

const rawSubscriberID = "signal/pumpdump:raw"

/*
Signal measuring pump and dump ignition from the trade tape.

Per symbol it maintains gross and signed volume over a configured horizon, self-scaled
RVOL and signed price precursor off the window anchor, and tape skew. Ignition lift is
(rvol-1)*(1+precursor)*(1+skew); a pooled BandCalibrator bands that scalar into four
categories by live quantile position. Strength is the banded lift observation.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	window        time.Duration
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	categories    map[string]types.CategoryType
	surpriseField *types.CategorySurpriseField
	rawDump       *rawdump.Writer
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
}

var pumpDefaultBandEdges = []float64{-0.10, 0.50, 2.00}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	window := viper.GetDuration("signals.pumpdump.window")

	if window <= 0 {
		errnie.Error(errors.New("signals.pumpdump.window is required"))
	}

	calibrator := numeric.NewSignalCalibrator(
		pumpDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
		[]float64{0.50, 0.30, 0.15, 0.05},
		numeric.DefaultCalibratorConfig("observation"),
		"pumpdump",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryFadedExhaustion,
		types.CategoryOrganicTrend,
		types.CategoryCoiledCompression,
		types.CategoryVerticalIgnition,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/pumpdump")
		return nil
	}

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		window:      window,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.BroadcastConsumer),
		categories: map[string]types.CategoryType{
			"faded_exhaustion":   types.CategoryFadedExhaustion,
			"organic_trend":      types.CategoryOrganicTrend,
			"coiled_compression": types.CategoryCoiledCompression,
			"vertical_ignition":  types.CategoryVerticalIgnition,
		},
		surpriseField: surpriseField,
		rawDump:       rawdump.Open("pumpdump"),
		classifier:    calibrator.Classifier,
		calibrator:    calibrator.Calibrator,
	}

	queueTTL := viper.GetDuration("system.queue.ttl")

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, queueTTL)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", queueTTL)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", queueTTL)

	errnie.Info("signal/pumpdump ready", "signal/pumpdump")

	return signal
}

func (signal *Signal) stateFor(symbol string) *pumpState {
	stored, _ := signal.symbols.LoadOrStore(
		symbol, newPumpState(signal.classifier, signal.window),
	)

	return stored.(*pumpState)
}

func (signal *Signal) Tick() error {
	for {
		message := signal.subscribers["raw"].Poll()

		if message == nil {
			continue
		}

		errnie.Debug("signal/pumpdump: Tick()", "type", message.Type)

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		if sm.Channel != public.TradesChannel {
			continue
		}

		for _, trade := range signalpool.GetTrades(sm) {
			err := signal.observe(trade)

			if err != nil && !isWarmup(err) {
				errnie.Error(err, "pumpdump: observe %s", trade.Symbol)
			}
		}
	}
}

func (signal *Signal) observe(trade market.TradeUpdate) error {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return errnie.Error(errors.New("pumpdump: price and quantity are required"))
	}

	state := signal.stateFor(trade.Symbol)
	reading, err := state.fold(trade)

	if err != nil {
		if isWarmup(err) {
			return err
		}

		return errnie.Error(err, "pumpdump: fold %s", trade.Symbol)
	}

	return signal.publish(trade, reading)
}

func (signal *Signal) Close() error {
	signal.cancel()

	return signal.rawDump.Close()
}
