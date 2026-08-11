package manifold

import (
	"fmt"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/system"
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
	waiting   map[string]struct{}
	ui        chan []byte
	binui     chan types.FluidFrame
	closing   bool
	settling  bool
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
	var bookSource websocket.BookSource

	if api != nil {
		bookSource = api
	}

	solver := &Solver{
		api:       bookSource,
		config:    config,
		domain:    domain,
		recorder:  recorder,
		tokenizer: NewTokenizer(config, symbols),
		residency: residency,
		corpus:    corpus,
		angles:    angles,
		pending:   make(map[string][]pendingDial),
		waiting:   make(map[string]struct{}),
		ui:        ui,
		binui:     binui,
	}

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
Update appends tokenized book samples for every changed Hawkes epoch, advances
the shared domain by its regulated relaxation budget, and stamps the result.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	clear(solver.waiting)
	measurements := make(map[string]*types.Measurement)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, nameOK := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !nameOK || symbolName == "" || !ok || symbol == nil {
			return true
		}

		evidence := symbol.MeasurementsSnapshot()

		if len(evidence) == 0 {
			return true
		}

		measurements[symbolName] = nil

		for _, measurement := range evidence {
			if measurement != nil && measurement.Source == types.SourceHawkes {
				measurements[symbolName] = measurement
				break
			}
		}

		return true
	})

	for symbolName, measurement := range measurements {
		buyExcitation := 0.0
		sellExcitation := 0.0

		if measurement != nil {
			buySample, buyFound := measurement.Metrics[types.MetricKey(
				types.MetricExcitationAmplitude, types.SideBuyToBuy,
			)]
			sellSample, sellFound := measurement.Metrics[types.MetricKey(
				types.MetricExcitationAmplitude, types.SideSellToSell,
			)]

			if !buyFound || !sellFound {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"manifold: Hawkes excitation metrics required for "+symbolName,
					nil,
				))
			}

			if (buySample.Normalized == nil) != (sellSample.Normalized == nil) {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"manifold: Hawkes excitation readiness must match for both sides of "+symbolName,
					nil,
				))
			}

			if buySample.Normalized != nil {
				buyExcitation = *buySample.Normalized
				sellExcitation = *sellSample.Normalized
			}
		}

		if solver.api == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: authoritative order book source required",
				nil,
			))
		}

		var particles []pfluid.Particle
		var contentIDs []uint32
		var err error
		bookPopulated := false
		solver.api.Book(symbolName, func(managed *mgrbook.Book) {
			if managed == nil {
				return
			}

			bidOrders := make([]*mgrbook.Order, 0)
			askOrders := make([]*mgrbook.Order, 0)

			for _, level := range managed.Bids.Levels {
				bidOrders = append(bidOrders, level.Queue()...)
			}

			for _, level := range managed.Asks.Levels {
				askOrders = append(askOrders, level.Queue()...)
			}

			if len(bidOrders)+len(askOrders) == 0 {
				return
			}

			bookPopulated = true
			particles, contentIDs, err = solver.tokenizer.NewBatch(
				bidOrders,
				askOrders,
				managed.Midpoint().Float64(),
				buyExcitation,
				sellExcitation,
				symbolName,
			)
		})

		if !bookPopulated {
			solver.waiting[symbolName] = struct{}{}
			continue
		}

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("failed to tokenize manifold particles for %s, %s", symbolName, err.Error()),
				err,
			))
		}

		if len(particles) == 0 || len(contentIDs) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: populated authoritative order book produced no particles for "+symbolName,
				nil,
			))
		}

		_, err = solver.domain.Append(particles, contentIDs)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to append %d manifold particles for %s: %v",
					len(particles), symbolName, err,
				),
				err,
			))
		}

		if err := solver.Step(
			thesis, symbolName, thesis.At, particles,
		); err != nil {
			return err
		}
	}

	return nil
}

/*
WaitingForBook answers whether Manifold has explicitly deferred any symbol
until its authoritative Level 3 manager publishes another update.
*/
func (solver *Solver) WaitingForBook() bool {
	return len(solver.waiting) > 0
}

func (solver *Solver) Step(
	thesis *types.Thesis,
	symbol string,
	at time.Time,
	particles []pfluid.Particle,
) error {
	if system.Cfg == nil || system.Cfg.Manifold == nil ||
		system.Cfg.Manifold.MinSteps <= 0 ||
		system.Cfg.Manifold.MaxSteps < system.Cfg.Manifold.MinSteps ||
		system.Cfg.Manifold.RelaxationSteps < system.Cfg.Manifold.MinSteps ||
		system.Cfg.Manifold.RelaxationSteps > system.Cfg.Manifold.MaxSteps {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: regulated relaxation step count must be within its configured bounds",
			nil,
		))
	}

	relaxationSteps := system.Cfg.Manifold.RelaxationSteps

	for step := 0; step < relaxationSteps; step++ {
		_, err := solver.domain.Advance()

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to advance manifold at relaxation step %d of %d: %v",
					step+1, relaxationSteps, err,
				),
				err,
			))
		}

		solver.stepped = true
	}

	reading, err := solver.domain.Reading()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to read manifold: "+err.Error(),
			err,
		))
	}

	thesis.Manifold = reading

	if err := solver.publish(thesis, symbol, at, particles); err != nil {
		return err
	}

	return nil
}

func (solver *Solver) publish(
	thesis *types.Thesis,
	symbol string,
	at time.Time,
	particles []pfluid.Particle,
) error {
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
					"failed to read manifold fields with %d resident particles: %v",
					solver.domain.ParticleCount(), fieldsErr,
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
					"failed to read manifold particles with %d resident particles: %v",
					solver.domain.ParticleCount(), err,
				),
				err,
			))
		}

		utils.PublishFluid(
			solver.binui, types.FluidParticlesChannel,
			datura.NewMap("particles", resident),
		)
	}

	row := datura.NewMap(
		"source", "manifold",
		"symbol", symbol,
		"at", at.Format(time.RFC3339),
	)
	solver.stampPhase(thesis, row, symbol, at, particles)

	if solver.ui != nil {
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
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	if solver.domain == nil {
		return nil
	}

	solver.closing = true
	err := solver.domain.Close()
	solver.domain = nil

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to close manifold domain: "+err.Error(),
			err,
		))
	}

	return nil
}
