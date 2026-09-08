package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/types"
)

/*
	Learner wires the market environment to the associative population. Its lock

protects the original state while the UI encodes it off the workspace ring.
*/
type Learner struct {
	Population                 *associative.Population[Action]
	Traders                    []*Trader
	Rehearsal                  *Rehearsal
	Developments               map[string]*Development
	Tape                       hindsight.Tape
	At                         time.Time
	Steps, Decisions, Resolved uint64
	Forced                     uint64
	Err                        error
	Restored                   bool
	mutex                      sync.Mutex
	Checkpoint                 Checkpoint
	// catalog reads the durable tape and receives graded decisions. The model
	// checkpoint is a blob and stays in Checkpoint; everything else is a table.
	catalog  *tables.Catalog
	Episodes uint64
	funding  *decimal.Decimal
	run      hindsight.RunID
	price    *broker.Price
	recorder *recording.Session
}

func NewLearner(ctx context.Context, api *websocket.API, price *broker.Price, balance *broker.Balance, count int, catalog *tables.Catalog, archive Blobs, run hindsight.RunID, recorder *recording.Session) (*Learner, error) {
	if count < 1 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "learning: population must contain an agent", nil))
	}
	learner := &Learner{Developments: make(map[string]*Development), Checkpoint: Checkpoint{Store: archive}, catalog: catalog, run: run, price: price, recorder: recorder, funding: balance.Cash()}

	// Agent zero is the live forward canary. It is the only member of the live
	// population; every remaining worker replays balanced historical episodes
	// instead of opening another wall-clock-throttled live wallet.
	trader, err := NewTrader(api, price, balance.Quote, balance.Cash())

	if err != nil {
		return nil, err
	}
	trader.ID, trader.Recorder = 0, recorder
	learner.Traders = append(learner.Traders, trader)
	population, err := associative.NewPopulation(trader)

	if err != nil {
		return nil, err
	}
	learner.Population = population
	learner.Restored, err = population.Restore(ctx, learner.Checkpoint)

	if err != nil {
		return nil, err
	}
	if count == 1 {
		return learner, nil
	}

	learner.Rehearsal = NewRehearsal(
		count-1,
		catalog,
		run,
		hindsight.DefaultDiscoveryPolicy(),
		price,
		population,
	)
	return learner, nil
}

/*
	Step consumes all published signal observations, regardless of transport kind.

Future tape grades and S3 I/O never run on this path.
*/
func (learner *Learner) Step(envelope *types.Envelope) *types.Envelope {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	envelope.Learning = learner
	learner.At = time.Now().UTC()

	if envelope.TypeID == types.EnvelopeTicker {
		learner.price.Update(&envelope.TickerData)
	}

	if learner.Err != nil {
		return envelope
	}
	measurements := envelope.SignalMeasurements()
	grouped := make(map[string][]*data.Measurement[float64])

	for _, measurement := range measurements {
		if measurement != nil {
			grouped[measurement.Label] = append(grouped[measurement.Label], measurement)
		}
	}

	for symbol, readings := range grouped {
		if err := learner.observe(symbol, readings); err != nil {
			learner.Err = errnie.Error(err)
			return envelope
		}
	}
	return envelope
}

func (learner *Learner) observe(symbol string, readings []*data.Measurement[float64]) error {
	development := learner.Developments[symbol]

	if development == nil {
		development = &Development{Symbol: symbol}
		learner.Developments[symbol] = development
	}
	history := development.Context(learner.At, readings)
	previousDecisions := learner.Decisions

	for _, trader := range learner.Traders {
		trader.At, trader.Version = learner.At, trader.Version+1
		learner.killStale(trader, symbol)
	}

	if err := learner.Population.Step(agent.Observation{At: learner.At, Measurements: readings, History: history}); err != nil {
		return err
	}
	if err := learner.releaseForced(symbol); err != nil {
		return errnie.Error(err)
	}

	learner.Steps++
	learner.Decisions = 0

	for _, member := range learner.Population.Agents {
		learner.Decisions += member.Decisions
	}
	development.Decisions += learner.Decisions - previousDecisions
	return learner.Population.Read(func(space *grid.Space) error {
		return development.Advance(learner.At, space)
	})
}

/*
killStale closes a canary position that stopped developing and grades the
decision immediately at the executable bid. This is the live forward loop's
forced-exit gate: instead of leaving an open trade waiting for a retracement
leg that may never print, the wallet returns capital and the model records the
stagnation outcome now.
*/
func (learner *Learner) killStale(trader *Trader, symbol string) {
	if trader == nil {
		return
	}
	regulator := trader.Positions[symbol]

	if regulator == nil || regulator.Holding.Qty.Sign() == 0 || !trader.stale(symbol) {
		return
	}
	// Close through the regulator directly: this is a forced canary exit, not
	// a decision the agent issued, so it must not create an evaluation that
	// later Review would mistake for a model-issued action.
	trader.Execution.At = trader.At

	if err := regulator.Exit(fmt.Sprintf("stale-%d-%s", trader.ID, symbol)); err != nil {
		learner.Err = errnie.Error(err)
		return
	}

	if err := regulator.Apply(trader.Execution.Fill); err != nil {
		learner.Err = errnie.Error(err)
		return
	}
	trader.Fills++
	trader.Fees = trader.Fees.Add(trader.Execution.Fill.FeeUsdEquiv)
	var bid *decimal.Decimal

	trader.api.Book(symbol, func(current *book.Book) {
		if current != nil && current.BestBid() != nil {
			bid = current.BestBid().Price
		}
	})

	if bid == nil {
		return
	}

	for identity, evaluation := range trader.Evaluations[symbol] {
		if evaluation.Complete || evaluation.Reference == nil {
			continue
		}
		leg := hindsight.Leg{
			Symbol: symbol, From: evaluation.At, Through: trader.At, ConfirmedAt: trader.At,
			Start: evaluation.Reference, End: bid,
		}

		if !evaluation.Resolve(leg) {
			continue
		}

		if err := learner.Population.Resolve(0, identity, evaluation.Value); err != nil {
			learner.Err = errnie.Error(err)
			return
		}
		learner.Resolved++
		delete(trader.Evaluations[symbol], identity)
	}
}

/*
	Review consumes only durable tape. Each completed decision is resolved once

on its issuing model; Population owns positive-experience consolidation.
*/
func (learner *Learner) Review(ctx context.Context) error {
	return learner.Tape.Read(ctx, learner.catalog, learner.run, func(leg hindsight.Leg) error {
		evaluations, err := learner.resolve(leg)

		if err != nil || len(evaluations) == 0 {
			return err
		}

		// Graded decisions go to the outcomes table through the recorder, so
		// they batch with everything else rather than each becoming its own
		// snapshot.
		for _, evaluation := range evaluations {
			if err := learner.recorder.WriteOutcome(evaluation.Row(string(learner.run))); err != nil {
				return err
			}
		}

		return nil
	})
}

// resolve completes original decision records under the learning lock. Once
// resolved they are immutable; Review persists them after releasing the lock.
func (learner *Learner) resolve(leg hindsight.Leg) ([]*Evaluation, error) {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	var evaluations []*Evaluation

	for index, trader := range learner.Traders {
		for identity, evaluation := range trader.Evaluations[leg.Symbol] {
			if !evaluation.Resolve(leg) {
				continue
			}

			if err := learner.Population.Resolve(index, identity, evaluation.Value); err != nil {
				return nil, err
			}
			trader.LastEvaluation = evaluation
			learner.Resolved++
			delete(trader.Evaluations[leg.Symbol], identity)
			evaluations = append(evaluations, evaluation)
		}
	}
	return evaluations, nil
}

/*
	Run keeps archive work outside observation processing. Interval controls I/O

cadence only; completed trade legs determine evaluation maturity.
*/
func (learner *Learner) Run(ctx context.Context, interval time.Duration) {
	if learner.Rehearsal != nil {
		go learner.runRehearsal(ctx, interval)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-learner.recorder.Durable:
			if err := learner.Review(ctx); err != nil {
				learner.fail(err)
				return
			}
		case <-ticker.C:
			if err := learner.Population.Save(ctx, learner.Checkpoint); err != nil {
				learner.fail(err)
				return
			}
		}
	}
}

/*
runRehearsal retries the historical replay whenever the archive still has no
replayable episodes. Once episodes exist the replay loop runs until shutdown.
*/
func (learner *Learner) runRehearsal(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := learner.Rehearsal.Run(ctx); err != nil {
			learner.fail(err)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

/* Error exposes the stage failure through the runtime node protocol. */
func (learner *Learner) Error() error {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	return learner.Err
}

func (learner *Learner) fail(err error) {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	learner.Err = errnie.Error(err)
}

// releaseForced discards a non-choice immediately after successful execution.
// It cannot become training evidence, so waiting for a future tape leg only
// retains an unbounded number of useless tickets when cash or a book is absent.
func (learner *Learner) releaseForced(symbol string) error {
	for index, member := range learner.Population.Agents {
		if member.Last == nil {
			continue
		}

		evaluations := learner.Traders[index].Evaluations[symbol]
		evaluation := evaluations[member.Last.ID]

		if evaluation == nil || !evaluation.Forced {
			continue
		}

		if err := learner.Population.Abort(index, evaluation.ID); err != nil {
			return errnie.Error(err)
		}

		delete(evaluations, evaluation.ID)
		learner.Forced++
	}
	return nil
}
