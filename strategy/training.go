package strategy

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
Tape is the lock-free handoff from the archive walk to the learner.

The walk is a long object-store read; Step is on the disruptor. A channel would
block one of them. The queue is the same shape capture already uses: enqueue
never waits, dequeue is empty or a fragment.
*/
type Tape struct {
	queue        *lf.Queue[types.ReplayFragment]
	done         atomic.Bool
	runs         atomic.Uint64
	observations atomic.Uint64
	budget       atomic.Uint64
}

func NewTape() *Tape {
	return &Tape{queue: lf.NewQueue[types.ReplayFragment]()}
}

func (tape *Tape) Publish(fragment types.ReplayFragment) {
	if len(fragment.Frames) == 0 || fragment.AnchorIndex <= 0 || fragment.AnchorIndex >= len(fragment.Frames) {
		return
	}

	tape.queue.Enqueue(fragment)
}

func (tape *Tape) Close() {
	tape.done.Store(true)
}

func (tape *Tape) take() (types.ReplayFragment, bool) {
	return tape.queue.Dequeue()
}

func (tape *Tape) loading() bool {
	return !tape.done.Load()
}

func (tape *Tape) SetBudget(budget uint64) {
	tape.budget.Store(budget)
}

func (tape *Tape) AddObservations(count uint64) {
	tape.observations.Add(count)
}

func (tape *Tape) AddRuns(count uint64) {
	tape.runs.Add(count)
}

func (tape *Tape) Budget() uint64 {
	return tape.budget.Load()
}

func (tape *Tape) Observations() uint64 {
	return tape.observations.Load()
}

func (tape *Tape) Runs() uint64 {
	return tape.runs.Load()
}

/*
Training is the composition root for the learning cohort.

Agent 0 operates as the live canary stepping incoming market envelopes, while
Agents 1..N-1 operate as rehearsal workers practicing on historical tape
excursions. All agents share the underlying cognition memory trie.
*/
type Training struct {
	ctx    context.Context
	mu     sync.Mutex
	tape   *Tape
	space  core.Primitive
	agents []*Agent
	main   *MainAgent
	legs   []types.ReplayFragment
	seen   atomic.Uint64
}

/* replay is the dashboard's reading of this pipeline. */
type replay struct {
	space        core.Primitive
	memories     []core.Primitive
	cohort       []*Agent
	mainAgent    *MainAgent
	fragments    int
	tape         []types.ReplayFragment
	loading      bool
	runs         uint64
	observations uint64
	budget       uint64
}

/*
NewTraining builds a learning pipeline over a tape that arrives as it is read.
It instantiates the full cohort of agents according to configuration.
*/
func NewTraining(
	ctx context.Context,
	tape *Tape,
	deps ...any,
) *Training {
	count := 8

	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.Traders > 0 {
		count = system.Cfg.Learning.Traders
	}

	var inst *broker.Instrument
	var prc *broker.Price
	var rng *rand.Rand
	var bal *broker.Balance
	var api *websocket.API
	var initialCash *decimal.Decimal

	for _, dep := range deps {
		switch v := dep.(type) {
		case *broker.Instrument:
			inst = v
		case *broker.Price:
			prc = v
		case *broker.Balance:
			bal = v
		case *websocket.API:
			api = v
		case *rand.Rand:
			rng = v
		case *decimal.Decimal:
			initialCash = v
		}
	}

	if initialCash == nil && bal != nil {
		initialCash = bal.Cash()
	}

	targetAccount := viper.GetString("trading.model")

	if targetAccount == "" || targetAccount == "simulated" {
		targetAccount = "paper"
	}

	sharedEngine := cognition.NewEngine(cognition.Config{})
	agents := make([]*Agent, count)

	for idx := 0; idx < count; idx++ {
		isLive := (idx == 0)
		var agentRNG *rand.Rand

		if rng != nil {
			agentRNG = rand.New(rand.NewSource(rng.Int63() + int64(idx)))
		}

		agents[idx] = NewAgent(idx, isLive, sharedEngine, 64, agentRNG)
	}

	training := &Training{
		tape:   tape,
		space:  agents[0].Space(),
		agents: agents,
		main:   NewMainAgent(initialCash, targetAccount, inst, prc, sharedEngine, api, bal),
		ctx:    ctx,
	}

	go training.rehearseLoop()

	return training
}

/*
Step is one delivery of the composition.

The live envelope asks. The ring answers. Number threads that answer through
the grid and the agent. Learning is stamped on the clock so the dashboard can
read it; the measurements never are.
*/
func (training *Training) Step(envelope *types.Envelope) *types.Envelope {
	training.mount()

	if envelope != nil {
		envelope.Learning = training

		if len(training.agents) > 0 {
			liveMeasurements := envelope.Measurements()

			if len(liveMeasurements) > 0 {
				symbol := envelope.Symbol()

				if symbol == "" {
					for _, measurement := range liveMeasurements {
						if measurement != nil && measurement.Label != "" {
							symbol = measurement.Label
							break
						}
					}
				}

				if symbol != "" {
					impulse, err := training.agents[0].Step(liveMeasurements, symbol)

					if err != nil {
						errnie.Error(err)
					}

					if training.main != nil {
						if !impulse.Ready {
							training.main.Step(envelope, ActionDecision{
								Action: ActionWait,
							})
						}

						if impulse.Ready {
							holding := training.main.IsHolding(symbol)
							decision, err := training.agents[0].ChooseAction(impulse, holding)

							if err != nil {
								errnie.Error(errnie.Err(errnie.Internal, "training: cognition evaluation failed", err))
								decision = ActionDecision{Action: ActionWait}
							}

							training.main.Step(envelope, decision)
						}
					}
				}
			}
		}
	}

	if training.main != nil && envelope.Positions == nil {
		envelope.Positions = training.main.exportPositions()
		envelope.Equity = training.main.exportEquity()
	}

	training.seen.Add(1)

	return envelope
}

/*
mount takes up fragments recovered by the tape reader, appends them to legs,
and randomly inserts them into the parent ring slots of the rehearsal workers.
*/
func (training *Training) mount() {
	if training.tape == nil {
		return
	}

	rehearsalWorkers := training.agents

	if len(training.agents) > 1 {
		rehearsalWorkers = training.agents[1:]
	}

	for count := 0; count < 4; count++ {
		fragment, ok := training.tape.take()

		if !ok {
			return
		}

		if len(fragment.Frames) == 0 || fragment.AnchorIndex <= 0 || fragment.AnchorIndex >= len(fragment.Frames) {
			continue
		}

		training.mu.Lock()
		training.legs = append(training.legs, fragment)
		training.mu.Unlock()

		for _, worker := range rehearsalWorkers {
			randSlot := 0

			if worker.ring != nil && ringLen[[]*data.Measurement[float64]](worker.ring) > 0 {
				randSlot = rand.Intn(ringLen[[]*data.Measurement[float64]](worker.ring))
			}

			worker.IngestReplay(fragment, randSlot)

			if worker.IsUnprimed() {
				if _, err := worker.RehearseChild(); err != nil {
					errnie.Error(err)
				}
			}
		}
	}
}

func (training *Training) rehearseLoop() {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-training.ctx.Done():
			return
		case <-ticker.C:
			training.rehearseCycle()
		}
	}
}

func (training *Training) rehearseCycle() {
	training.mount()

	rehearsalWorkers := training.agents

	if len(training.agents) > 1 {
		rehearsalWorkers = training.agents[1:]
	}

	for _, worker := range rehearsalWorkers {
		if _, err := worker.RehearseChild(); err != nil {
			errnie.Error(err)
		}
	}
}

func (training *Training) snapshot() *replay {
	memories := make([]core.Primitive, len(training.agents))

	for idx, individual := range training.agents {
		memories[idx] = store.NewRetained(individual.Tree())
	}
	activeSpace := training.space

	if spaceState(activeSpace).Updated == "" && len(training.agents) > 1 {
		for _, individual := range training.agents {
			if spaceState(individual.Space()).Updated != "" {
				activeSpace = individual.Space()
				break
			}
		}
	}

	var runs, observations, budget uint64
	loading := false

	if training.tape != nil {
		runs = training.tape.Runs()
		observations = training.tape.Observations()
		budget = training.tape.Budget()
		loading = training.tape.loading()
	}

	if budget == 0 {
		budget = uint64(max(len(training.legs), 1))
	}

	return &replay{
		space:        activeSpace,
		memories:     memories,
		cohort:       training.agents,
		mainAgent:    training.main,
		fragments:    len(training.legs),
		tape:         training.legs,
		loading:      loading,
		runs:         runs,
		observations: observations,
		budget:       budget,
	}
}

/* Error joins the pipeline's failure for the runtime node protocol. */
func (training *Training) Error() error {
	training.mu.Lock()
	defer training.mu.Unlock()

	var errs []error

	for _, individual := range training.agents {
		errs = append(errs, individual.space.Error(), individual.learner.Error())
	}

	return errors.Join(errs...)
}

/*
RecentTrades supplies completed trades in reverse chronological order for the
Trade Journal surface and GET /trades endpoint.
*/
func (training *Training) RecentTrades(limit int) ([]*telemetry.PositionT, error) {
	if training == nil || training.main == nil {
		return []*telemetry.PositionT{}, nil
	}

	return training.main.RecentTrades(limit)
}

/*
RequestExit forwards a manual position exit request from the UI to the main agent.
*/
func (training *Training) RequestExit(symbol string) {
	if training == nil || training.main == nil {
		return
	}

	training.main.RequestExit(symbol)
}
