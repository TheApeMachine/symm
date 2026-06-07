package correlation

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	// gridBars is how many recent bar returns feed each SimHash hyperplane dot product.
	gridBars = 32
	// hashBits is the fingerprint width: one uint64 signature per coin per batch.
	hashBits = 64
	// correlationBatchInterval is the cross-section window. Trades accumulate per
	// symbol and the herd pass runs once per interval — correlation is a property
	// of the cross-section, not of a single print.
	correlationBatchInterval = 250 * time.Millisecond
	// energyFloor excludes coins whose variance is below this fraction of the slow
	// market-energy baseline — dead-flat and illiquid coins cannot vote as herd.
	energyFloor     = 0.0625
	rawSubscriberID = "signal/correlation:raw"

	defaultCalibratorWindow        = 8192
	defaultCalibratorRefitInterval = 256
	defaultCalibratorWarmup        = 512
	defaultCalibratorBlend         = 0.3
)

// correlationBandEdges seed the herd bands; self-calibration adapts them to the
// live pooled distribution from there: divergent_stress | stochastic_noise |
// decoupled_alpha | systemic_herd.
var correlationBandEdges = []float64{-0.30, 0.40, 2.00}

func calibratorConfig() (window, refitInterval, warmup int, blend float64) {
	window = viper.GetInt("signals.correlation.calibrator.window")

	if window <= 0 {
		window = defaultCalibratorWindow
	}

	refitInterval = viper.GetInt("signals.correlation.calibrator.refit_interval")

	if refitInterval <= 0 {
		refitInterval = defaultCalibratorRefitInterval
	}

	warmup = viper.GetInt("signals.correlation.calibrator.warmup")

	if warmup <= 0 {
		warmup = defaultCalibratorWarmup
	}

	blend = viper.GetFloat64("signals.correlation.calibrator.blend")

	if blend <= 0 {
		blend = defaultCalibratorBlend
	}

	return window, refitInterval, warmup, blend
}

/*
Signal measuring cross-asset "herd behavior" via the dominant eigenmode of the
return field — read cheaply as bit-agreement with the market's majority
fingerprint, so the whole universe is classified in one O(n) pass per tick with
no pairwise correlation matrix.

  - Fingerprint: each coin's recent movement is stamped into one 64-bit
    signature through a fixed set of random hyperplanes (SimHash).
  - Market mode: the bit-by-bit majority vote across all signatures is the
    dominant shared direction — "the market."
  - Correlation: a coin's bit-agreement with that majority (a single popcount)
    is how hard it is herding; anti-agreement is divergence.
  - Energy: each coin's exponentially-weighted return variance, normalised by a
    slow market-energy baseline, separates active regimes from quiet noise.

| Category         | Correlation | Variance | Market "Feel"   |
|:-----------------|:------------|:---------|:----------------|
| Systemic Herd    | High >0.85  | High     | Global Beta     |
| Decoupled Alpha  | Low         | High     | Unique Driver   |
| Stochastic Noise | Low         | Low      | Quiet           |
| Divergent Stress | Negative    | High     | Contrarian Move |
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	planes        [hashBits][gridBars]float64
	marketEnergy  *adaptive.EMA
	categories    map[string]types.CategoryType
	activeScratch []live
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

/*
NewSignal wires the measurements broadcast and installs one fixed random
hyperplane set. The seed is fixed so every coin is stamped with the same
projection — otherwise bit agreement would be meaningless.
*/
func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	// One classifier and one calibrator shared by every coin, so the herd bands
	// reflect the whole universe's pooled fused-score distribution with a single
	// sample count, instead of fragmenting into a per-coin state.
	calibratorWindow, calibratorRefitInterval, calibratorWarmup, calibratorBlend := calibratorConfig()
	pooledCalibrator := numeric.NewSignalCalibrator(
		correlationBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{
			"divergent_stress",
			"stochastic_noise",
			"decoupled_alpha",
			"systemic_herd",
		},
		[]float64{0.15, 0.45, 0.25, 0.15},
		numeric.CalibratorConfig{
			Window:     calibratorWindow,
			RefitEvery: calibratorRefitInterval,
			MinSamples: calibratorWarmup,
			Blend:      calibratorBlend,
			SeedField:  "strength",
		},
		"correlation",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryDivergentStress,
		types.CategoryStochasticNoise,
		types.CategoryDecoupledAlpha,
		types.CategorySystemicHerd,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/correlation")
		return nil
	}

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.BroadcastConsumer),
		marketEnergy: adaptive.NewEMA(0),
		categories: map[string]types.CategoryType{
			"divergent_stress": types.CategoryDivergentStress,
			"stochastic_noise": types.CategoryStochasticNoise,
			"decoupled_alpha":  types.CategoryDecoupledAlpha,
			"systemic_herd":    types.CategorySystemicHerd,
		},
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("correlation"),
	}

	// Fixed seed: one shared projection for the whole universe.
	rng := rand.New(rand.NewSource(1))

	for planeIndex := range signal.planes {
		for barIndex := range signal.planes[planeIndex] {
			signal.planes[planeIndex][barIndex] = float64(rng.Intn(2)*2 - 1)
		}
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = bus.Group(pool, "measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = bus.Group(pool, "ui", 10*time.Millisecond)

	errnie.Info("signal/correlation ready", "signal/correlation")

	return signal
}

/*
Tick accumulates the latest trade price per symbol and runs one herd pass per
batch tick. Per-trade processing would restamp fingerprints on every print;
correlationBatchInterval batches enough cross-section activity to be stable.
*/
func (signal *Signal) Tick() error {
	batch := time.NewTicker(correlationBatchInterval)
	defer batch.Stop()

	latest := make(map[string]float64)
	raw := signal.subscribers["raw"]

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case <-batch.C:
			if len(latest) == 0 {
				continue
			}

			if err := signal.process(latest); err != nil {
				errnie.Error(err, "correlation: process")
				continue
			}

			clear(latest)
		default:
		}

		message := raw.Poll()

		if message == nil {
			select {
			case <-signal.ctx.Done():
				return signal.ctx.Err()
			case <-batch.C:
				if len(latest) == 0 {
					continue
				}

				if err := signal.process(latest); err != nil {
					errnie.Error(err, "correlation: process")
					continue
				}

				clear(latest)
			case <-time.After(2 * time.Millisecond):
			}

			continue
		}

		if message.Value == nil {
			continue
		}

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		if sm.Channel != public.TradesChannel {
			continue
		}

		for _, trade := range signalpool.GetTrades(sm) {
			if trade.Price > 0 {
				latest[trade.Symbol] = trade.Price
			}
		}
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
