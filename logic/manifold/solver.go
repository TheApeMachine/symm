package manifold

import (
	"fmt"
	"runtime"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/spf13/viper"
	mgrbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	api       websocket.BookSource
	config    pfluid.Config
	domain    *pfluid.Domain
	recorder  *audit.Recorder
	tokenizer *Tokenizer
	residency int
	corpus    *geometry.Corpus[types.PhaseOutcome]
	angles    []float64
	pending   map[string][]pendingDial
	ui        chan []byte
	binui     chan types.FluidFrame
	pool      pond.Pool
	group     pond.TaskGroup
	stepped   bool
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(
	api *websocket.API,
	ui chan []byte,
	binui chan types.FluidFrame,
	recorder *audit.Recorder,
) *Solver {
	config := pfluid.DefaultConfig()
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 && configuredDelta.Seconds() < float64(config.MaxDelta) {
		config.MaxDelta = float32(configuredDelta.Seconds())
	}

	symbols := make([]string, 0)

	if api != nil {
		api.Books().Range(func(key, _ any) bool {
			name, ok := key.(string)

			if ok {
				symbols = append(symbols, name)
			}

			return true
		})
	}

	domain, err := newDomain(config)
	errnie.Error(err)

	cells := config.Grid.X * config.Grid.Y * config.Grid.Z
	residency := max(cells/32, 1)

	if configuredResidency := viper.GetInt("market.manifold.residency"); configuredResidency > 0 {
		residency = configuredResidency
	}

	corpus, corpusErr := geometry.NewCorpus[types.PhaseOutcome](phaseCorpusCapacity)
	errnie.Error(corpusErr)
	angles, angleErr := geometry.PhasePath(phaseScanAngles)
	errnie.Error(angleErr)

	solver := &Solver{
		api:       api,
		config:    config,
		domain:    domain,
		recorder:  recorder,
		tokenizer: NewTokenizer(config, symbols),
		residency: residency,
		corpus:    corpus,
		angles:    angles,
		pending:   make(map[string][]pendingDial),
		ui:        ui,
		binui:     binui,
		pool:      pond.NewPool(runtime.NumCPU()),
	}

	solver.group = solver.pool.NewGroup()
	return solver
}

func newDomain(config pfluid.Config) (*pfluid.Domain, error) {
	var domain *pfluid.Domain

	err := compute.WithMetalInit(func() error {
		created, err := pfluid.NewDomain(config)

		if err != nil {
			return err
		}

		domain = created
		return nil
	})

	if err != nil {
		return nil, err
	}

	return domain, nil
}

/*
Update appends tokenized book samples for every changed Hawkes epoch, then
advances the shared domain once for the complete tick.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis.Readiness.Hawkes {
		for _, measurement := range utils.Measurements(thesis, types.SourceHawkes) {
			managed := solver.api.Book(measurement.Symbol)

			if managed == nil {
				continue
			}

			bidOrders := make([]*mgrbook.Order, 0)
			askOrders := make([]*mgrbook.Order, 0)

			for _, level := range managed.Bids.Levels {
				bidOrders = append(bidOrders, level.Queue()...)
			}

			for _, level := range managed.Asks.Levels {
				askOrders = append(askOrders, level.Queue()...)
			}

			found, ok := thesis.Measurements.Load(types.SourceHawkes)

			if !ok {
				continue
			}

			measurements, ok := found.([]*types.Measurement)

			if !ok {
				continue
			}

			var buyExcitation, sellExcitation float64

			for _, measurement := range measurements {
				if measurement == nil {
					continue
				}

				buySample, buyFound := measurement.Metrics[types.MetricKey(
					types.MetricExcitationAmplitude, types.SideBuyToBuy,
				)]
				sellSample, sellFound := measurement.Metrics[types.MetricKey(
					types.MetricExcitationAmplitude, types.SideSellToSell,
				)]

				if !buyFound || !sellFound || buySample.Normalized == nil || sellSample.Normalized == nil {
					continue
				}

				buyExcitation = *buySample.Normalized
				sellExcitation = *sellSample.Normalized
			}

			if buyExcitation == 0 && sellExcitation == 0 {
				continue
			}

			particles, contentIDs, err := solver.tokenizer.NewBatch(
				bidOrders,
				askOrders,
				managed.Midpoint().Float64(),
				buyExcitation,
				sellExcitation,
				measurement.Symbol,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("failed to tokenize manifold particles for %s, %s", measurement.Symbol, err.Error()),
					err,
				))
			}

			if len(particles) == 0 || len(contentIDs) == 0 {
				continue
			}

			_, err = solver.domain.Append(particles, contentIDs)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf(
						"failed to append %d manifold particles for %s: %v",
						len(particles), measurement.Symbol, err,
					),
					err,
				))
			}

			if err := solver.Step(
				thesis, measurement.Symbol, thesis.At, particles,
			); err != nil {
				continue
			}
		}

		if !solver.stepped {
			thesis.Stamp(types.SourceManifold)
			return nil
		}

		var err error
		thesis.Manifold, err = solver.domain.Reading()

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to read manifold: "+err.Error(),
				err,
			))
		}

		thesis.Stamp(types.SourceManifold)
	}

	thesis.Fanout(types.SourceManifold)
	return nil
}

func (solver *Solver) Step(
	thesis *types.Thesis,
) error {
	_, err := solver.domain.Advance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to advance manifold with %d resident particles: %v",
				solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	solver.stepped = true

	// Fields and particles are the fluid view's raw frames, and the view
	// renders one symbol: the one the dashboard is focused on. Reading them
	// back for every symbol served no display, but still paid the GPU readback
	// and — because Publish marshals — encoded whole float grids into decimal
	// text, which profiled at 37% of process CPU and 80GB of allocations per
	// run. The resulting GC pressure was the stall, so the frames are read
	// only for the symbol something is actually looking at.
	if solver.binui != nil && symbol == types.Focus() {
		fields, fieldsErr := solver.domain.Fields()

		if fieldsErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to read manifold fields for %s with %d resident particles: %v",
					symbol, solver.domain.ParticleCount(), fieldsErr,
				),
				fieldsErr,
			))
		}

		utils.PublishFluid(
			solver.binui, types.FluidFieldsChannel,
			datura.NewMap("fields", fields),
		)

		resident, err := solver.domain.ReadParticles(
			0, solver.domain.ParticleCount(),
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to read manifold particles for %s with %d resident particles: %v",
					symbol, solver.domain.ParticleCount(), err,
				),
				err,
			))
		}

		utils.PublishFluid(
			solver.binui, types.FluidParticlesChannel,
			datura.NewMap("particles", resident),
		)
	}

	if solver.ui != nil {
		row := datura.NewMap(
			"source", "manifold",
			"symbol", symbol,
			"at", at.Format(time.RFC3339),
		)

		solver.stampPhase(thesis, row, symbol, at, particles)

		select {
		case solver.ui <- datura.NewMap(
			"manifold", []datura.Map[any]{row},
		).MarshalAndFree():
		default:
		}
	}

	return nil
}

/*
residentHeatLocked sums Heat over every resident particle that is not
clamped, matching the Python prototype's total_Q. Callers must hold domainMu.
*/
func (solver *Solver) residentHeatLocked() (float32, error) {
	count := solver.domain.ParticleCount()

	if count == 0 {
		return 0, nil
	}

	particles, err := solver.domain.ReadParticles(0, count)

	if err != nil {
		return 0, err
	}

	clamped, err := solver.domain.ReadClamped(0, count)

	if err != nil {
		return 0, err
	}

	var totalHeat float32

	for index, particle := range particles {
		if index < len(clamped) && clamped[index] {
			continue
		}

		totalHeat += particle.Heat
	}

	return totalHeat, nil
}

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	if solver.domain == nil {
		return nil
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil

	return nil
}
