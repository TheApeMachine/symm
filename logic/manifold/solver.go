package manifold

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
)

/*
HawkesSource exposes the latest empirical arrival process for every symbol.
*/
type HawkesSource interface {
	Symbols() []string
	Outcome(symbol string) (excitation.Outcome, bool)
}

/*
intensityCandidate is one book-grounded market source entering the next shared
domain step. Book fields are owned samples — no live *book.Order pointers.
*/
type intensityCandidate struct {
	symbol       string
	outcome      excitation.Outcome
	midPrice     float64
	orderIDs     []string
	batch        Batch
	reference    *decimal.Decimal
	spread       float64
	buyCapacity  *decimal.Decimal
	sellCapacity *decimal.Decimal
}

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	*PhaseCorpus
	mu       sync.Mutex
	config   pfluid.Config
	domain   *pfluid.Domain
	symbols  map[string]*symbolSlot
	active   map[string]struct{}
	books    *bookSampler
	recorder *audit.Recorder
}

/*
symbolSlot remembers a symbol's last appended Hawkes/book sample. start/end are
append-time bookmarks only; Advance clears them after inelastic merge rewrites
resident indices. Views read the shared post-merge population.
*/
type symbolSlot struct {
	epoch  uint64
	at     time.Time
	events int
	start  int
	end    int
	orders []string
	state  map[string]pfluid.Particle
	last   State
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(books BookSource, historyCapacity int) (*Solver, error) {
	config := pfluid.DefaultConfig()
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 && configuredDelta.Seconds() < float64(config.MaxDelta) {
		config.MaxDelta = float32(configuredDelta.Seconds())
	}

	phaseCorpus, err := NewPhaseCorpus(historyCapacity)

	if err != nil {
		return nil, err
	}

	var domain *pfluid.Domain
	err = compute.WithMetalInit(func() error {
		created, createErr := pfluid.NewDomain(config)

		if createErr != nil {
			return createErr
		}

		domain = created
		return nil
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to create shared Sensorium domain",
			err,
		)
	}

	return &Solver{
		PhaseCorpus: phaseCorpus,
		config:      config,
		domain:      domain,
		symbols:     make(map[string]*symbolSlot),
		active:      make(map[string]struct{}),
		books:       newBookSampler(books),
	}, nil
}

/*
SetRecorder attaches the runtime audit stream to shared-domain advances.
*/
func (solver *Solver) SetRecorder(recorder *audit.Recorder) {
	if solver != nil {
		solver.recorder = recorder
	}
}

/*
Update appends tokenized book samples for every changed Hawkes epoch, then
always advances the shared domain once for this tick and publishes symbol views
of that physical state. Inject is Hawkes-gated; the step is not.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) error {
	if solver == nil || thesis == nil || hawkes == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.domain == nil {
		return nil
	}

	changed := solver.changedOutcomes(hawkes)
	candidates := solver.sampleChanged(hawkes, changed)
	result := solver.advance(thesis, candidates, changed)
	population := solver.Population()

	errnie.Error(audit.Record(solver.recorder, "manifold", map[string]any{
		"candidates": len(candidates),
		"appended":   result.appended,
		"particles":  population,
		"failed":     len(result.failures),
	}))

	return errors.Join(result.failures...)
}

/*
changedOutcomes lists Hawkes epochs that have not yet been appended, without
touching L3 books.
*/
func (solver *Solver) changedOutcomes(
	hawkes HawkesSource,
) map[string]excitation.Outcome {
	symbols := append([]string(nil), hawkes.Symbols()...)
	changed := make(map[string]excitation.Outcome)

	for _, symbol := range symbols {
		outcome, ok := hawkes.Outcome(symbol)

		if !ok || !outcome.Readiness.Intensity {
			continue
		}

		buyIntensity, sellIntensity := intensities(outcome)

		if buyIntensity+sellIntensity <= 0 {
			continue
		}

		candidate := intensityCandidate{symbol: symbol, outcome: outcome}

		if candidate.changed(solver.symbols[symbol]) {
			changed[symbol] = outcome
		}
	}

	return changed
}

/*
sampleChanged tokenizes L3 books only for epochs that must enter the domain.
Universe indices are alphabetical over the full Hawkes symbol set so content
ids stay stable when only a subset of markets change this tick.
*/
func (solver *Solver) sampleChanged(
	hawkes HawkesSource,
	changed map[string]excitation.Outcome,
) []intensityCandidate {
	universe := sortedUniverse(hawkes.Symbols())
	symbols := make([]string, 0, len(changed))

	for symbol := range changed {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	candidates := make([]intensityCandidate, 0, len(symbols))
	tokenizer := NewTokenizer(solver.config)

	for _, symbol := range symbols {
		symbolIndex, ok := universeIndex(universe, symbol)

		if !ok {
			continue
		}

		buyIntensity, sellIntensity := intensities(changed[symbol])
		population, ready := solver.books.Sample(
			symbol, tokenizer, buyIntensity, sellIntensity, symbolIndex,
		)

		if !ready {
			continue
		}

		candidates = append(candidates, intensityCandidate{
			symbol:       symbol,
			outcome:      changed[symbol],
			midPrice:     population.midPrice,
			orderIDs:     population.orderIDs,
			batch:        population.batch,
			reference:    population.reference,
			spread:       population.spread,
			buyCapacity:  population.buyCapacity,
			sellCapacity: population.sellCapacity,
		})
	}

	return candidates
}

/*
noteAdvanceFailure records the root cause of one rejected market population.
*/
func (solver *Solver) noteAdvanceFailure(symbol string, err error) error {
	cause := err

	for errors.Unwrap(cause) != nil {
		cause = errors.Unwrap(cause)
	}

	errnie.Error(audit.Record(solver.recorder, "manifold_advance", map[string]any{
		"symbol": symbol,
		"ok":     false,
		"error":  cause.Error(),
	}))

	return errnie.Err(
		errnie.Internal,
		fmt.Sprintf("manifold: %s failed to enter shared domain", symbol),
		err,
	).With("cause", cause.Error())
}

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() {
	if solver == nil {
		return
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.domain == nil {
		return
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil
	clear(solver.symbols)
	clear(solver.active)
	solver.PhaseCorpus = nil
}

/*
Population returns the Metal-resident particle count.
*/
func (solver *Solver) Population() int {
	if solver == nil || solver.domain == nil {
		return 0
	}

	return solver.domain.ParticleCount()
}
