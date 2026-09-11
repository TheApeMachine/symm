package strategy

import (
	"math/rand"
	"sync"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Agent encapsulates an autonomous cognitive unit: a private grid space for multi-signal
feature fusion, an associative context for token sequence extraction, an evaluator
for post-hoc reinforcement, and an associative learner writing to cognitive memory.
*/
type Agent struct {
	mu        sync.RWMutex
	answersMu sync.RWMutex
	id        int
	isLive    bool
	space     *grid.Space
	learner   *associative.Agent
	context   *associative.Context
	evaluator *FragmentEvaluator
	ring      *store.Ring[[]*data.Measurement[float64]]
	answers   []*telemetry.LearningAnswerT
	rng       *rand.Rand
}

/*
NewAgent creates an agent with a private perception grid, associative learner,
and randomized precursor offset generator.
*/
func NewAgent(
	id int,
	isLive bool,
	engine *cognition.Engine,
	windowBins int,
	rng ...*rand.Rand,
) *Agent {
	space := grid.NewSpaceWithWindow(windowBins)
	learner := associative.NewAgent(engine)
	var randomSource *rand.Rand

	if len(rng) > 0 && rng[0] != nil {
		randomSource = rng[0]
	}

	if randomSource == nil {
		randomSource = rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
	}

	return &Agent{
		id:        id,
		isLive:    isLive,
		space:     space,
		learner:   learner,
		context:   associative.NewContext(),
		evaluator: NewFragmentEvaluator(nil),
		ring:      store.NewRing[[]*data.Measurement[float64]](),
		answers:   make([]*telemetry.LearningAnswerT, 0, 256),
		rng:       randomSource,
	}
}

func (agent *Agent) ID() int {
	return agent.id
}

func (agent *Agent) IsLive() bool {
	return agent.isLive
}

func (agent *Agent) Space() *grid.Space {
	return agent.space
}

func (agent *Agent) Engine() *cognition.Engine {
	return agent.learner.Engine()
}

func (agent *Agent) Learner() *associative.Agent {
	return agent.learner
}

func (agent *Agent) Context() *associative.Context {
	return agent.context
}

func (agent *Agent) Evaluator() *FragmentEvaluator {
	return agent.evaluator
}

func (agent *Agent) SetFeeRate(rate float64) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.evaluator.SetFeeRate(rate)
}

func (agent *Agent) SetRNG(source *rand.Rand) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.rng = source
}

/* PreseedColumns allocates storage for all expected columns across signal sources upfront. */
func (agent *Agent) PreseedColumns(sources map[string][]string) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.space.PreseedColumns(sources)
}

func (agent *Agent) Tree() *iradix.Tree[[]byte] {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.learner.Tree()
}

/*
IngestFragment wraps one tape fragment as a child ring and inserts it into
the parent ring at the specified slot.
*/
func (agent *Agent) IngestFragment(fragment [][]*data.Measurement[float64], slot int) {
	if len(fragment) == 0 {
		return
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	child := store.NewRing[[]*data.Measurement[float64]]()

	for _, frame := range fragment {
		child.Write(frame)
	}

	agent.ring.WriteAt(child, slot)
}

/*
ChooseAction evaluates the cognitive memory for the active impulse regions and
chooses one of the currently legal actions:
When flat:    {ActionEnter, ActionWait}
When holding: {ActionExit, ActionWait}
*/
func (agent *Agent) ChooseAction(
	impulse grid.Impulse,
	holding bool,
) (Action, []byte, float64, float64, uint64) {
	sequence := agent.context.Sequence(impulse)

	if len(sequence) == 0 {
		return ActionWait, nil, 0, 0, 0
	}

	evaluation := agent.learner.Engine().Evaluate(sequence)
	legal := LegalActions(holding)
	action := ActionWait

	for _, candidate := range legal {
		if string(candidate) == evaluation.WinnerClass {
			action = candidate
			break
		}
	}

	agent.recordAnswer(&telemetry.LearningAnswerT{
		Asked:      "action",
		Answered:   string(action),
		RunnerUp:   evaluation.RunnerUp,
		Confidence: evaluation.Confidence,
		Contrast:   evaluation.Contrast,
		Ambiguity:  evaluation.Ambiguity,
		Support:    evaluation.Support,
	})

	return action, sequence, evaluation.Confidence, evaluation.Contrast, evaluation.Support
}

/*
RehearseChild plays out the current child sequence starting from a random
point within the precursor, steps perception on each frame, chooses legal
actions, evaluates their outcomes against objective subsequent facts, and
reinforces or inhibits the chosen action in the cognitive memory trie.
*/
func (agent *Agent) RehearseChild() (int, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	if agent.ring == nil || agent.ring.Len() == 0 {
		return 0, nil
	}

	fragment := agent.ring.CurrentChildValues()
	childLen := len(fragment)

	if childLen <= 0 {
		return 0, nil
	}

	offset := 0

	if childLen > 2 {
		offset = agent.rng.Intn(childLen - 1)
	}

	holding := false
	entryIdx := -1
	stepped := 0

	for frameIdx := offset; frameIdx < childLen; frameIdx++ {
		measurements := fragment[frameIdx]

		if len(measurements) == 0 {
			continue
		}

		symbol := ""

		for _, measurement := range measurements {
			if measurement != nil && measurement.Label != "" {
				symbol = measurement.Label
				break
			}
		}

		if symbol == "" {
			continue
		}

		impulse, err := agent.stepLocked(measurements, symbol)

		if err != nil {
			return stepped, err
		}

		stepped++

		if !impulse.Ready {
			continue
		}

		action, context, _, _, _ := agent.ChooseAction(impulse, holding)

		if len(context) == 0 {
			continue
		}

		switch action {
		case ActionEnter:
			outcome, err := agent.evaluator.EvaluateEntry(fragment, frameIdx)

			if err == nil {
				agent.learner.Engine().Observe(context, []byte(ActionEnter), outcome.Correctness)
				holding = true
				entryIdx = frameIdx
			}
		case ActionExit:
			outcome, err := agent.evaluator.EvaluateExit(fragment, frameIdx, entryIdx)

			if err == nil {
				agent.learner.Engine().Observe(context, []byte(ActionExit), outcome.Correctness)
				holding = false
				entryIdx = -1
			}
		case ActionWait:
			outcome, err := agent.evaluator.EvaluateWait(fragment, frameIdx, holding, entryIdx)

			if err == nil {
				agent.learner.Engine().Observe(context, []byte(ActionWait), outcome.Correctness)
			}
		}
	}

	agent.ring.Advance()

	return stepped, nil
}

// Step advances this agent's private perception grid and extracts impulses
func (agent *Agent) Step(measurements []*data.Measurement[float64], symbol string) (grid.Impulse, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	return agent.stepLocked(measurements, symbol)
}

func (agent *Agent) stepLocked(measurements []*data.Measurement[float64], symbol string) (grid.Impulse, error) {
	if err := agent.space.Step(measurements); err != nil {
		return grid.Impulse{}, err
	}

	label := symbol

	if label == "" {
		label = agent.space.UpdatedLabel
	}

	if label == "" {
		return grid.Impulse{}, nil
	}

	now := time.Now().UTC()

	return agent.space.Impulse(label, now, now)
}

func (agent *Agent) recordAnswer(answer *telemetry.LearningAnswerT) {
	const maxAnswers = 256

	agent.answersMu.Lock()
	defer agent.answersMu.Unlock()

	if len(agent.answers) >= maxAnswers {
		agent.answers = append(agent.answers[1:], answer)

		return
	}

	agent.answers = append(agent.answers, answer)
}

/* Answers returns a copy of genuine out-of-sample streaming evaluations. */
func (agent *Agent) Answers() []*telemetry.LearningAnswerT {
	agent.answersMu.RLock()
	defer agent.answersMu.RUnlock()

	answers := make([]*telemetry.LearningAnswerT, len(agent.answers))
	copy(answers, agent.answers)

	return answers
}
