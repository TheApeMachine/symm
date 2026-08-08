package manifold

import (
	"fmt"
	"math"
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

const inspectionParticleLimit = 4096

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
	turnover  int
	fold      foldMeter
	corpus    *geometry.Corpus[types.PhaseOutcome]
	angles    []float64
	pending   map[string][]pendingDial
	ui        chan []byte
	binui     chan types.FluidFrame
	pool      pond.Pool
	group     pond.TaskGroup
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
		fold:      foldMeter{cap: residency},
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
	if !thesis.Readiness.Hawkes {
		return nil
	}

	for _, measurement := range utils.Measurements(thesis, types.SourceHawkes) {
		managed := solver.api.Book(measurement.Symbol)

		if managed == nil {
			continue
		}

		// The conditional-intensity keys are always present: the hawkes signal publishes
		// them with a zero Raw before its fit is ready, so map presence says nothing.
		// Normalized is the discriminator - it is only set once HawkesFit is ready, and
		// it carries the quantity we actually want, (lambda - mu) / mu.
		buyExcitation, buyState := sideExcitation(measurement, types.SideBuy)
		sellExcitation, sellState := sideExcitation(measurement, types.SideSell)

		solver.fold.excite(buyState)
		solver.fold.excite(sellState)

		bidOrders := make([]*mgrbook.Order, 0)
		askOrders := make([]*mgrbook.Order, 0)

		for _, level := range managed.Bids.Levels {
			bidOrders = append(bidOrders, level.Queue()...)
		}

		for _, level := range managed.Asks.Levels {
			askOrders = append(askOrders, level.Queue()...)
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

		particles, contentIDs, _ = solver.filterBatch(particles, contentIDs)

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

		solver.turnover += len(particles)
		solver.fold.inject(len(particles))

		if err := solver.Step(thesis, measurement.Symbol, thesis.At, particles); err != nil {
			continue
		}
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
	return nil
}

func (solver *Solver) Step(
	thesis *types.Thesis,
	symbol string,
	at time.Time,
	particles []pfluid.Particle,
) error {
	_, err := solver.domain.Advance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to advance manifold for %s with %d resident particles: %v",
				symbol, solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	/*
		Fields and particles are the fluid view's raw frames, and the view
		renders one symbol: the one the dashboard is focused on. Reading them
		back for every symbol served no display, but still paid the GPU readback
		and — because Publish marshals — encoded whole float grids into decimal
		text, which profiled at 37% of process CPU and 80GB of allocations per
		run. The resulting GC pressure was the stall, so the frames are read
		only for the symbol something is actually looking at.
	*/
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
		particleCount := min(solver.domain.ParticleCount(), inspectionParticleLimit)
		resident, particlesErr := solver.domain.ReadParticles(0, particleCount)

		if particlesErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to read manifold particles for %s with %d resident particles: %v",
					symbol, solver.domain.ParticleCount(), particlesErr,
				),
				particlesErr,
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

func (solver *Solver) filterBatch(
	particles []pfluid.Particle,
	contentIDs []uint32,
) ([]pfluid.Particle, []uint32, int) {
	if len(particles) == 0 || len(contentIDs) == 0 {
		return nil, nil, 0
	}

	keptParticles := make([]pfluid.Particle, 0, len(particles))
	keptContentIDs := make([]uint32, 0, len(contentIDs))
	dropped := 0

	for index, particle := range particles {
		if !admissibleParticle(particle, solver.config) {
			dropped++
			continue
		}

		keptParticles = append(keptParticles, particle)
		keptContentIDs = append(keptContentIDs, contentIDs[index])
	}

	return keptParticles, keptContentIDs, dropped
}

/*
sideExcitation reads one side's Hawkes self-excitation, (lambda - mu) / mu, from a
measurement row.

Why:
  - The hawkes signal publishes conditional_intensity for both sides on every row, but
    substitutes a zero Raw until its fit converges, so testing the map for the key cannot
    tell a converged zero from an unconverged placeholder. Normalized is only populated
    once HawkesFit is ready, which makes its presence the readiness signal.
  - An unconverged fit means the excitation is unmeasured, and an unmeasured cascade
    forces nothing, so the side enters at unit energy rather than stalling the symbol.
*/
func sideExcitation(
	measurement *types.Measurement,
	side types.MeasurementSide,
) (float64, excitationState) {
	sample, ok := measurement.Metrics[types.MetricKey(
		types.MetricConditionalIntensity, side,
	)]

	if !ok {
		return 0, excitationMissing
	}

	if sample.Normalized == nil {
		// The hawkes signal substitutes a zero Raw for an unconverged fit, so a nil
		// Normalized sitting on a zero Raw is a warm-up and a nil sitting on a positive
		// Raw is a converged fit reporting arrivals below their immigrant baseline.
		if sample.Raw == 0 {
			return 0, excitationUnfit
		}

		return 0, excitationBelowBaseline
	}

	return *sample.Normalized, excitationForced
}

func admissibleParticle(particle pfluid.Particle, config pfluid.Config) bool {
	values := []float32{
		particle.Position.X,
		particle.Position.Y,
		particle.Position.Z,
		particle.Velocity.X,
		particle.Velocity.Y,
		particle.Velocity.Z,
		particle.Mass,
		particle.Heat,
		particle.Energy,
		particle.Phase,
		particle.Omega,
	}

	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}

	if particle.Mass <= pfluid.MinimumPilotWaveMass || particle.Heat < 0 || particle.Energy < 0 {
		return false
	}

	return particle.Omega >= config.OmegaMin && particle.Omega <= config.OmegaMax
}

func (solver *Solver) rejectBatch(
	symbol string,
	batchStart int,
	batchParticles int,
) {
	if solver == nil || solver.domain == nil || batchParticles <= 0 {
		return
	}

	resident := solver.domain.ParticleCount()

	if batchStart < 0 || batchStart >= resident {
		return
	}

	batchEnd := min(batchStart+batchParticles, resident)
	keep := make([]uint32, 0, resident-(batchEnd-batchStart))

	for index := range resident {
		if index >= batchStart && index < batchEnd {
			continue
		}

		keep = append(keep, uint32(index))
	}

	if err := solver.domain.Retain(keep); err != nil {
		solver.resetDomain(symbol)
		return
	}

	solver.turnover = max(solver.turnover-batchParticles, 0)
}

func (solver *Solver) resetDomain(symbol string) {
	solver.recreateDomain("failed_step", time.Time{}, map[string]any{
		"symbol": symbol,
	})
}

func (solver *Solver) recreateDomain(
	reason string,
	at time.Time,
	extra map[string]any,
) {
	if solver == nil {
		return
	}

	if solver.domain != nil {
		errnie.Error(solver.domain.Close())
	}

	domain, err := newDomain(solver.config)

	if err != nil {
		solver.domain = nil
		solver.turnover = 0

		message := "failed to recreate manifold domain"

		if reason == "failed_step" {
			message = fmt.Sprintf("failed to reset manifold domain after %s destabilized it", extra["symbol"])
		}

		errnie.Error(errnie.Err(
			errnie.Internal,
			message+": "+err.Error(),
			err,
		))

		return
	}

	solver.domain = domain
	solver.turnover = 0

	validationAt := at

	if validationAt.IsZero() {
		validationAt = time.Now().UTC()
	}
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
