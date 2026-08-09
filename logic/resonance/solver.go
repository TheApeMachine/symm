package resonance

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Solver feeds normalized market measurements into one resonance manifold per symbol.
*/
type Solver struct {
	ctx      context.Context
	cancel   context.CancelFunc
	recorder *audit.Recorder
	coders   *sync.Map
	alpha    float64
	ui       chan []byte
}

var coderPool = sync.Pool{
	New: func() any {
		return learning.NewResonanceManifold(
			[]int{6, 12, 6}, 1, viper.GetFloat64("resonance.learning_rate"),
		)
	},
}

/*
NewSolver returns a predictive-coding solver using the configured learning pace.
*/
func NewSolver(
	ctx context.Context,
	ui chan []byte,
	recorder *audit.Recorder,
	initialAlpha float64,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	return &Solver{
		ctx:      ctx,
		cancel:   cancel,
		recorder: recorder,
		coders:   &sync.Map{},
		alpha:    initialAlpha,
		ui:       ui,
	}
}

/*
Update settles one predictive-coding manifold for every symbol carrying finite,
normalized measurements and publishes the resulting hierarchy.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if !(solver.alpha > 0) || solver.alpha > 1 || math.IsNaN(solver.alpha) || math.IsInf(solver.alpha, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: alpha must be finite and in (0, 1]",
			errors.New("invalid resonance alpha"),
		))
	}

	features := make(map[string]map[string]float64)

	thesis.Measurements.Range(func(_, value any) bool {
		measurements, ok := value.([]*types.Measurement)

		if !ok {
			return true
		}

		for _, measurement := range measurements {
			if measurement == nil || measurement.Symbol == "" {
				continue
			}

			for key, sample := range measurement.Metrics {
				if sample.Normalized == nil || math.IsNaN(*sample.Normalized) || math.IsInf(*sample.Normalized, 0) {
					continue
				}

				if features[measurement.Symbol] == nil {
					features[measurement.Symbol] = make(map[string]float64)
				}

				identity := string(measurement.Source) + ":" + measurement.Symbol + ":" + key
				features[measurement.Symbol][identity] = *sample.Normalized
			}
		}

		return true
	})

	if len(features) == 0 {
		thesis.Fanout(types.SourceResonance)
		return nil
	}

	group, _ := errgroup.WithContext(solver.ctx)

	for symbol, readings := range features {
		group.Go(func() error {
			schema := make([]string, 0, len(readings))

			for identity := range readings {
				schema = append(schema, identity)
			}

			sort.Strings(schema)
			input := make([]float64, len(schema))

			for index, identity := range schema {
				input[index] = readings[identity]
			}

			found, ok := solver.coders.Load(symbol)

			if !ok {
				found, ok = coderPool.Get().(*learning.ResonanceManifold)

				if !ok || found == nil {
					return errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"resonance: failed to get predictive coding manifold from pool",
						errors.New("invalid resonance manifold"),
					))
				}

				solver.coders.Store(symbol, found)
			}

			coder, ok := found.(*learning.ResonanceManifold)

			if !ok || coder == nil {
				return errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"resonance: predictive coding manifold has an invalid type",
					errors.New("invalid resonance manifold"),
				))
			}

			if _, err := coder.SettleFromBatch(input, nil); err != nil {
				return err
			}

			layers, surprise, energy := coder.WireSnapshot()

			reading := types.ResonanceReading{
				Stage:    "resonance",
				Source:   types.SourceResonance,
				Symbol:   symbol,
				At:       thesis.At,
				Surprise: surprise,
				Energy:   energy,
				Latent:   coder.LatentState(),
				Layers:   layers,
				Alpha:    solver.alpha,
			}

			thesis.Resonance.Store(symbol, reading)

			utils.Publish(
				solver.ui,
				datura.NewMap("resonance", reading),
			)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: failed to update CPU predictive coding manifold: "+err.Error(),
			err,
		))
	}

	thesis.Readiness.Stamp(types.SourceResonance)
	thesis.Fanout(types.SourceResonance)
	return nil
}

/*
Close stops the solver context.
*/
func (solver *Solver) Close() error {
	if solver.cancel != nil {
		solver.cancel()
	}

	return nil
}
