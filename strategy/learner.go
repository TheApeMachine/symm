package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/impulse"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/types"
)

/*
	Learner consumes ready grid impulses and owns the live decision lifecycle. Its lock

protects the original state while the UI encodes it off the workspace ring.
*/
type Learner struct {
	Grid                       *impulse.Solver
	Agent                      *agent.Agent[Action]
	Traders                    []*Trader
	Rehearsal                  *Rehearsal
	Tape                       hindsight.Tape
	At                         time.Time
	Steps, Decisions, Resolved uint64
	Err                        error
	Restored                   bool
	mutex                      sync.Mutex
	archive                    Blobs
	// catalog reads the durable tape and receives graded decisions. The model
	// checkpoint is a blob and stays in archive; everything else is a table.
	catalog  *tables.Catalog
	funding  *decimal.Decimal
	run      hindsight.RunID
	price    *broker.Price
	recorder *recording.Session
}

func NewLearner(ctx context.Context, api *websocket.API, price *broker.Price, balance *broker.Balance, count int, catalog *tables.Catalog, archive Blobs, run hindsight.RunID, recorder *recording.Session) (*Learner, error) {
	if count < 1 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "learning: population must contain an agent", nil))
	}
	learner := &Learner{Grid: impulse.NewSolver(), archive: archive, catalog: catalog, run: run, price: price, recorder: recorder, funding: balance.Cash()}

	// Agent zero is the live forward canary. It is the only member of the live
	// population; every remaining worker replays balanced historical episodes
	// instead of opening another wall-clock-throttled live wallet.
	trader, err := NewTrader(api, price, balance.Quote, balance.Cash())

	if err != nil {
		return nil, err
	}
	trader.ID, trader.Recorder = 0, recorder
	learner.Traders = append(learner.Traders, trader)
	learner.Agent, err = agent.New(ctx, trader, cognition.NewEngine(cognition.DefaultConfig()), false)

	if err != nil {
		return nil, err
	}
	learner.Restored, err = learner.Restore(ctx)

	if err != nil {
		return nil, err
	}

	if count == 1 {
		return learner, nil
	}

	learner.Rehearsal = NewRehearsal(
		ctx,
		count-1,
		catalog,
		run,
		hindsight.DefaultDiscoveryPolicy(),
		price,
		learner,
	)
	return learner, nil
}

// Ready supplies the agent workload's event-local dependency gate. Invalid
// market data never becomes an idle choice or a learning sample.
func (learner *Learner) Ready(envelope *types.Envelope) bool {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	envelope.Learning = learner

	if envelope.TypeID == types.EnvelopeTicker {
		learner.price.Update(&envelope.TickerData)
	}
	trader := learner.Traders[0]
	for symbol, through := range trader.Ending {
		trader.At = time.Now()

		if err := trader.End(symbol, through); err != nil {
			learner.Err = errnie.Error(err)
			return true
		}
	}
	for _, impulse := range envelope.Impulses {
		trader.Status = "waiting for regions"

		if !impulse.Ready {
			continue
		}
		trader.Status = "waiting for book or fees"

		if trader.Ready(impulse.Label) {
			trader.Status = "learning"
			return true
		}
	}
	return false
}

// Step consumes reduced grid output after the workload barrier. It never
// rebuilds the grid or grades against a future observation.
func (learner *Learner) Step(envelope *types.Envelope) *types.Envelope {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	envelope.Learning = learner

	if learner.Err != nil {
		return envelope
	}
	for _, impulse := range envelope.Impulses {
		if !impulse.Ready || !learner.Traders[0].Ready(impulse.Label) {
			continue
		}

		learner.At = impulse.At
		trader := learner.Traders[0]
		trader.At, trader.Version = impulse.At, trader.Version+1
		learner.Agent.Step(impulse)

		if err := learner.Agent.Error(); err != nil {
			learner.Err = errnie.Error(err)
			return envelope
		}
		learner.Steps++
		learner.Decisions = learner.Agent.Decisions
	}
	return envelope
}

/*
	Review consumes only durable tape. Each completed decision is resolved once

on its issuing cognition engine.
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

	for _, trader := range learner.Traders {
		for identity, evaluation := range trader.Evaluations[leg.Symbol] {
			if !evaluation.Resolve(leg) {
				continue
			}

			if _, err := learner.Agent.Resolve(identity, evaluation.Value); err != nil {
				return nil, err
			}
			trader.LastEvaluation = evaluation
			learner.Resolved++
			delete(trader.Evaluations[leg.Symbol], identity)
			evaluations = append(evaluations, evaluation)
		}

		if len(evaluations) > 0 {
			if leg.ConfirmedAt.After(trader.At) {
				trader.At = leg.ConfirmedAt
			}

			if err := trader.End(leg.Symbol, leg.Through); err != nil {
				return nil, errnie.Error(err)
			}
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
			if err := learner.Save(ctx); err != nil {
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
