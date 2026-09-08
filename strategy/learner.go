package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
	"gocloud.dev/blob"
)

/*
	Learner wires the market environment to the associative population. Its lock

protects the original state while the UI encodes it off the workspace ring.
*/
type Learner struct {
	Population                 *associative.Population[Action]
	Traders                    []*Trader
	Developments               map[string]*Development
	Tape                       hindsight.Tape
	At                         time.Time
	Steps, Decisions, Resolved uint64
	Err                        error
	Restored                   bool
	mutex                      sync.Mutex
	Checkpoint                 Checkpoint
	run                        hindsight.RunID
	price                      *broker.Price
	recorder                   *recording.Session
}

func NewLearner(ctx context.Context, api *websocket.API, price *broker.Price, balance *broker.Balance, count int, archive *blob.Bucket, run hindsight.RunID, recorder *recording.Session) (*Learner, error) {
	if count < 1 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "learning: population must contain an agent", nil))
	}
	learner := &Learner{Developments: make(map[string]*Development), Checkpoint: Checkpoint{Bucket: archive}, run: run, price: price, recorder: recorder}
	environments := make([]agent.Environment[Action], 0, count)

	for range count {
		trader, err := NewTrader(api, price, balance.Quote, balance.Cash())

		if err != nil {
			return nil, err
		}
		trader.ID, trader.Recorder = len(learner.Traders), recorder
		learner.Traders = append(learner.Traders, trader)
		environments = append(environments, trader)
	}
	population, err := associative.NewPopulation(environments...)

	if err != nil {
		return nil, err
	}
	learner.Population = population
	learner.Restored, err = population.Restore(ctx, learner.Checkpoint)

	if err != nil {
		return nil, err
	}
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
	}

	if err := learner.Population.Step(agent.Observation{At: learner.At, Measurements: readings, History: history}); err != nil {
		return err
	}
	learner.Steps++
	learner.Decisions = 0

	for _, member := range learner.Population.Agents {
		learner.Decisions += member.Decisions
	}
	development.Decisions += learner.Decisions - previousDecisions
	return development.Advance(learner.At, learner.Population.Grid)
}

/*
	Review consumes only durable tape. Each completed decision is resolved once

on its issuing model; Population owns positive-experience consolidation.
*/
func (learner *Learner) Review(ctx context.Context) error {
	return learner.Tape.Read(ctx, learner.Checkpoint.Bucket, learner.run, func(leg hindsight.Leg) error {
		evaluations, err := learner.resolve(leg)

		if err != nil || len(evaluations) == 0 {
			return err
		}
		first := evaluations[0]
		key := fmt.Sprintf("%s%d-%020d.json", learner.run.Prefix("outcomes"), first.Trader, first.ID)
		return store.Write(ctx, learner.Checkpoint.Bucket, key, evaluations)
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
