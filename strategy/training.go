package strategy

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
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
	queue        *lf.Queue[[][]*data.Measurement[float64]]
	done         atomic.Bool
	runs         atomic.Uint64
	observations atomic.Uint64
	budget       atomic.Uint64
}

func NewTape() *Tape {
	return &Tape{queue: lf.NewQueue[[][]*data.Measurement[float64]]()}
}

func (tape *Tape) Publish(leg [][]*data.Measurement[float64]) {
	tape.queue.Enqueue(leg)
}

func (tape *Tape) Close() {
	tape.done.Store(true)
}

func (tape *Tape) take() ([][]*data.Measurement[float64], bool) {
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
	*runtime.System
	mu     sync.Mutex
	tape   *Tape
	space  *grid.Space
	agent  *associative.Agent
	agents []*Agent
	main   *MainAgent
	legs   [][][]*data.Measurement[float64]
	seen   atomic.Uint64
}

/* replay is the dashboard's reading of this pipeline. */
type replay struct {
	space        *grid.Space
	memories     []*store.Retained[*iradix.Tree[[]byte]]
	learners     []*associative.Agent
	cohort       []*Agent
	mainAgent    *MainAgent
	fragments    int
	tape         [][][]*data.Measurement[float64]
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

	sharedEngine := cognition.NewEngine(cognition.DefaultConfig())
	agents := make([]*Agent, count)

	for idx := 0; idx < count; idx++ {
		isLive := (idx == 0)
		agents[idx] = NewAgent(idx, isLive, sharedEngine, 64)
	}

	var inst *broker.Instrument
	var prc *broker.Price

	for _, dep := range deps {
		switch v := dep.(type) {
		case *broker.Instrument:
			inst = v
		case *broker.Price:
			prc = v
		}
	}

	training := &Training{
		tape:   tape,
		space:  agents[0].Space(),
		agent:  agents[0].learner,
		agents: agents,
		main:   NewMainAgent(nil, "", inst, prc, sharedEngine),
		System: runtime.NewSystem(ctx, "training"),
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
						consensus := training.consensus(impulse)
						seq := PrecursorSequence(impulse)
						training.main.Step(envelope, consensus, seq)
					}
				}
			}
		}
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

		training.mu.Lock()
		training.legs = append(training.legs, fragment)
		training.mu.Unlock()

		for _, worker := range rehearsalWorkers {
			randSlot := 0

			if worker.ring != nil && worker.ring.Len() > 0 {
				randSlot = rand.Intn(worker.ring.Len())
			}

			worker.IngestFragment(fragment, randSlot)

			if worker.Space().UpdatedLabel == "" {
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
		case <-training.Context().Done():
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

func (training *Training) consensus(impulse grid.Impulse) PrecursorConsensus {
	if len(training.agents) == 0 || len(impulse.Regions) == 0 {
		return PrecursorConsensus{Action: "wait", Confidence: 0.5}
	}

	actionCounts := make(map[string]int)
	totalConfidence, totalContrast := 0.0, 0.0
	var totalSupport uint64

	for _, individual := range training.agents {
		action, confidence, contrast, support := individual.EvaluatePrecursor(impulse)
		totalConfidence += confidence
		totalContrast += contrast
		totalSupport += support

		if action != "" {
			actionCounts[action]++
		}
	}

	numAgents := float64(len(training.agents))
	avgConfidence := totalConfidence / numAgents
	avgContrast := totalContrast / numAgents

	waitCount := actionCounts["wait"]
	bestAction := "wait"
	bestCount := waitCount
	minMajority := (len(training.agents) + 1) / 2

	for a, count := range actionCounts {
		if a != "wait" && count > bestCount && count >= minMajority {
			bestAction = a
			bestCount = count
		}
	}

	if bestAction != "wait" {
		if avgContrast <= 0 || avgConfidence <= 0.5 || totalSupport <= 1 {
			bestAction = "wait"
		}
	}

	return PrecursorConsensus{
		Action:     bestAction,
		Confidence: avgConfidence,
		Contrast:   avgContrast,
		Support:    totalSupport,
	}
}

func (training *Training) snapshot() *replay {
	memories := make([]*store.Retained[*iradix.Tree[[]byte]], len(training.agents))
	learners := make([]*associative.Agent, len(training.agents))

	for idx, individual := range training.agents {
		memories[idx] = store.NewRetained(individual.Tree())
		learners[idx] = individual.learner
	}
	activeSpace := training.space

	if activeSpace.UpdatedLabel == "" && len(training.agents) > 1 {
		for _, individual := range training.agents {
			if individual.Space().UpdatedLabel != "" {
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
		learners:     learners,
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
