package manifold

import (
	"errors"
	"fmt"
	"sort"
	"time"

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
domain step.
*/
type intensityCandidate struct {
	symbol   string
	outcome  excitation.Outcome
	orders   []physicalOrder
	midPrice float64
}

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	*PhaseCorpus
	config    pfluid.Config
	domain    *pfluid.Domain
	particles []pfluid.Particle
	symbols   map[string]*symbolSlot
	active    map[string]struct{}
	books     *bookSampler
	recorder  *audit.Recorder
}

/*
symbolSlot remembers a symbol's last market epoch and its most recently
appended particle range so unchanged source epochs can be replayed honestly.
*/
type symbolSlot struct {
	epoch  uint64
	at     time.Time
	events int
	coords *coordinateEpoch
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
	configuredDelta := viper.GetDuration("signals.fluid.integration_interval")

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
Update assembles every book-grounded market population, advances the shared
domain once when any source epoch changes, and publishes symbol views of that
same physical state.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) error {
	if solver == nil || thesis == nil || hawkes == nil {
		return nil
	}

	candidates := solver.candidates(hawkes)
	result := solver.advance(thesis, candidates)
	errnie.Error(audit.Record(solver.recorder, "manifold", map[string]any{
		"candidates": len(candidates),
		"advanced":   result.advanced,
		"replayed":   result.replayed,
		"particles":  len(solver.particles),
		"failed":     len(result.failures),
	}))

	return errors.Join(result.failures...)
}

/*
candidates returns every symbol with observed intensity and a two-sided L3
book. Alphabetical order keeps particle identity independent of map ordering.
*/
func (solver *Solver) candidates(hawkes HawkesSource) []intensityCandidate {
	symbols := append([]string(nil), hawkes.Symbols()...)
	sort.Strings(symbols)
	candidates := make([]intensityCandidate, 0, len(symbols))

	for _, symbol := range symbols {
		outcome, ok := hawkes.Outcome(symbol)

		if !ok || !outcome.Readiness.Intensity {
			continue
		}

		buyIntensity, sellIntensity := intensities(outcome)

		if buyIntensity+sellIntensity <= 0 {
			continue
		}

		orders, midPrice, ready := solver.books.Orders(symbol)

		if !ready {
			continue
		}

		candidates = append(candidates, intensityCandidate{
			symbol:   symbol,
			outcome:  outcome,
			orders:   orders,
			midPrice: midPrice,
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
	if solver == nil || solver.domain == nil {
		return
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil
	solver.particles = nil
	clear(solver.symbols)
	clear(solver.active)
	solver.PhaseCorpus = nil
}
