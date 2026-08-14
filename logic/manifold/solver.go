package manifold

import (
	"fmt"
	"runtime"
	"sort"
	"sync/atomic"
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
	waiting   atomic.Bool
	ui        chan []byte
	binui     chan types.FluidFrame
	semaphore chan struct{}
	stopping  chan struct{}
	stopped   chan struct{}
	closing   atomic.Bool
	settling  atomic.Bool
	stepped   bool
	thesis    *types.Thesis
	at        time.Time
	cuts      []manifoldCut
}

/*
manifoldCut keeps the particles attributed to one symbol while every cut in the
market update relaxes inside the same resident domain.
*/
type manifoldCut struct {
	symbol      string
	particles   []pfluid.Particle
	measurement *types.Measurement
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
		ui:        ui,
		binui:     binui,
		semaphore: make(chan struct{}, 1),
		stopping:  make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	go solver.run()

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
Update appends tokenized book samples and wakes the resident settlement worker.
Calls received while that worker is stepping leave their measurements queued.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: thesis required",
			nil,
		))
	}

	if solver.closing.Load() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: solver is closing",
			nil,
		))
	}

	if !solver.settling.CompareAndSwap(false, true) {
		return nil
	}

	if solver.closing.Load() {
		solver.settling.Store(false)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: solver is closing",
			nil,
		))
	}

	cuts, err := solver.append(thesis)

	if err != nil {
		solver.settling.Store(false)
		return err
	}

	if len(cuts) == 0 {
		solver.settling.Store(false)
		return nil
	}

	solver.thesis = thesis
	solver.at = thesis.At
	solver.cuts = cuts
	solver.semaphore <- struct{}{}

	return nil
}

func (solver *Solver) run() {
	defer close(solver.stopped)
	defer solver.settling.Store(false)

	for {
		select {
		case <-solver.semaphore:
			err := solver.Step(solver.thesis, solver.at, solver.cuts)
			solver.settling.Store(false)

			if err != nil {
				errnie.Error(err)
			}
		case <-solver.stopping:
			return
		}
	}
}

func (solver *Solver) append(thesis *types.Thesis) ([]manifoldCut, error) {
	solver.waiting.Store(false)
	measurements := make(map[string]*types.Measurement)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, nameOK := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !nameOK || symbolName == "" || !ok || symbol == nil {
			return true
		}

		if symbol.Status == types.BUSY {
			measurements[symbolName] = nil
		}

		for measurement := range symbol.MarketMeasurements("manifold") {
			if measurement != nil && measurement.Source == types.SourceHawkes {
				measurements[symbolName] = measurement
			}
		}

		return true
	})
	symbolNames := make([]string, 0, len(measurements))

	for symbolName := range measurements {
		symbolNames = append(symbolNames, symbolName)
	}

	sort.Strings(symbolNames)
	cuts := make([]manifoldCut, 0, len(symbolNames))
	allParticles := make([]pfluid.Particle, 0)
	allContentIDs := make([]uint32, 0)

	for _, symbolName := range symbolNames {
		measurement := measurements[symbolName]
		if _, known := universeIndex(solver.tokenizer.universe, symbolName); !known {
			solver.tokenizer.universe = sortedUniverse(
				append(solver.tokenizer.universe, symbolName),
			)
		}
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
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"manifold: Hawkes excitation metrics required for "+symbolName,
					nil,
				))
			}

			if (buySample.Normalized == nil) != (sellSample.Normalized == nil) {
				return nil, errnie.Error(errnie.Err(
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
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: authoritative order book source required",
				nil,
			))
		}

		particles, contentIDs, err := solver.particles(
			symbolName, buyExcitation, sellExcitation,
		)

		if err != nil {
			return nil, err
		}

		if len(particles) == 0 || len(contentIDs) == 0 {
			solver.waiting.Store(true)
			continue
		}

		allParticles = append(allParticles, particles...)
		allContentIDs = append(allContentIDs, contentIDs...)
		cuts = append(cuts, manifoldCut{symbol: symbolName, particles: particles})
	}

	if len(cuts) == 0 {
		return nil, nil
	}

	_, err := solver.domain.Append(allParticles, allContentIDs)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to append %d manifold particles across %d symbols: %v",
				len(allParticles), len(cuts), err,
			),
			err,
		))
	}

	return cuts, nil
}

func (solver *Solver) particles(
	symbolName string,
	buyExcitation float64,
	sellExcitation float64,
) ([]pfluid.Particle, []uint32, error) {
	var particles []pfluid.Particle
	var contentIDs []uint32
	var batchErr error
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
		particles, contentIDs, batchErr = solver.tokenizer.NewBatch(
			bidOrders,
			askOrders,
			managed.Midpoint().Float64(),
			buyExcitation,
			sellExcitation,
			symbolName,
		)
	})

	if batchErr != nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"failed to tokenize manifold particles for %s, %s",
				symbolName, batchErr.Error(),
			),
			batchErr,
		))
	}

	if !bookPopulated {
		return nil, nil, nil
	}

	return particles, contentIDs, nil
}

/*
WaitingForBook answers whether Manifold has explicitly deferred any symbol
until its authoritative Level 3 manager publishes another update.
*/
func (solver *Solver) WaitingForBook() bool {
	return solver.waiting.Load()
}

func (solver *Solver) Step(
	thesis *types.Thesis,
	at time.Time,
	cuts []manifoldCut,
) error {
	config := system.Cfg.Snapshot()

	if config == nil || config.Manifold == nil ||
		config.Manifold.MinSteps <= 0 ||
		config.Manifold.MaxSteps < config.Manifold.MinSteps ||
		config.Manifold.RelaxationSteps < config.Manifold.MinSteps ||
		config.Manifold.RelaxationSteps > config.Manifold.MaxSteps {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: regulated relaxation step count must be within its configured bounds",
			nil,
		))
	}

	relaxationSteps := config.Manifold.RelaxationSteps

	for step := range relaxationSteps {
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
		reading, err := solver.domain.Reading()

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to read manifold: "+err.Error(),
				err,
			))
		}

		thesis.StoreManifold(reading)

		for _, cut := range cuts {
			if err := solver.publish(
				thesis, cut.symbol, at, cut.particles,
			); err != nil {
				return err
			}
		}

		if err := solver.publishDomain(); err != nil {
			return err
		}
	}

	return nil
}

func (solver *Solver) publishDomain() error {
	if solver.binui == nil {
		return nil
	}

	fields, err := solver.domain.Fields()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to read manifold fields with %d resident particles: %v",
				solver.domain.ParticleCount(), err,
			),
			err,
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

	return nil
}

func (solver *Solver) publish(
	thesis *types.Thesis,
	symbol string,
	at time.Time,
	particles []pfluid.Particle,
) error {
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

	if !solver.closing.CompareAndSwap(false, true) {
		return nil
	}

	for solver.settling.Load() {
		runtime.Gosched()
	}

	close(solver.stopping)
	<-solver.stopped

	if solver.domain == nil {
		return nil
	}

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
