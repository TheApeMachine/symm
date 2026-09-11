package strategy

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Agent encapsulates an autonomous cognitive unit: a private grid space for multi-signal
feature fusion, an associative context for token sequence extraction, an evaluator
for post-hoc reinforcement, and an associative learner writing to cognitive memory.
*/
type Agent struct {
	mu         sync.RWMutex
	answersMu  sync.RWMutex
	id         int
	isLive     bool
	space      *grid.Space
	learner    *associative.Agent
	context    *associative.Context
	evaluator  *FragmentEvaluator
	price      *broker.Price
	ring       *store.Ring[[]*data.Measurement[float64]]
	replays    *store.Ring[types.ReplayFragment]
	answers    []*telemetry.LearningAnswerT
	rng        *rand.Rand
	lastOffset int
	lastEntry  int
	lastExit   int
	lastMarks  []*telemetry.LearningMarkT
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
		replays:   store.NewRing[types.ReplayFragment](),
		answers:   make([]*telemetry.LearningAnswerT, 0, 256),
		rng:       randomSource,
		lastEntry: -1,
		lastExit:  -1,
		lastMarks: make([]*telemetry.LearningMarkT, 0, 64),
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

func (agent *Agent) SetPrice(price *broker.Price) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.price = price
	agent.evaluator.SetPrice(price)
}

func (agent *Agent) Price() *broker.Price {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.price
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

func (agent *Agent) LastEntry() int {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.lastEntry
}

func (agent *Agent) LastExit() int {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.lastExit
}

func (agent *Agent) LastOffset() int {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.lastOffset
}

func (agent *Agent) LastMarks() []*telemetry.LearningMarkT {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if len(agent.lastMarks) == 0 {
		return nil
	}

	marks := make([]*telemetry.LearningMarkT, len(agent.lastMarks))
	copy(marks, agent.lastMarks)

	return marks
}

/* PreseedColumns allocates storage for all expected columns across signal sources upfront. */
func (agent *Agent) PreseedColumns(sources map[string][]string) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.space.PreseedColumns(sources)
}

/*
Reset clears observation-local perception state in Space and temporal history
in Context while preserving the learned grid structure and shared cognition trie.
Must be called before replaying an independent historical fragment or starting at A.
*/
func (agent *Agent) Reset() {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.space.Reset()
	agent.context.Reset()
}

func (agent *Agent) Tree() *iradix.Tree[[]byte] {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.learner.Tree()
}

/*
IngestReplay loads one replay fragment carrying frames, symbol, and objective anchor metadata.
*/
func (agent *Agent) IngestReplay(fragment types.ReplayFragment, slot int) {
	if len(fragment.Frames) == 0 {
		return
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.replays.WriteAt(fragment, slot)

	child := store.NewRing[[]*data.Measurement[float64]]()

	for _, frame := range fragment.Frames {
		child.Write(frame)
	}

	agent.ring.WriteAt(child, slot)
}

/*
IngestFragment wraps one tape fragment as a child ring and inserts it into
the parent ring at the specified slot.
*/
func (agent *Agent) IngestFragment(fragment [][]*data.Measurement[float64], slot int) {
	if len(fragment) == 0 {
		return
	}

	symbol := ""

	for _, frame := range fragment {
		for _, measurement := range frame {
			if measurement != nil && measurement.Label != "" {
				symbol = measurement.Label
				break
			}
		}

		if symbol != "" {
			break
		}
	}

	anchor := max(1, len(fragment)/2)

	agent.IngestReplay(types.ReplayFragment{
		Frames:      fragment,
		Symbol:      symbol,
		AnchorIndex: anchor,
	}, slot)
}

type actionCandidate struct {
	action   Action
	share    float64
	support  uint64
	observed bool
}

func selectLegalAction(
	legal []Action,
	candidates []cognition.ClassCandidate,
	rng *rand.Rand,
) (Action, float64, float64, uint64) {
	if len(legal) == 0 {
		return ActionWait, 0, 0, 0
	}

	stats := make([]actionCandidate, len(legal))
	totalSupport := uint64(0)
	supportedCount := 0

	for index, act := range legal {
		stats[index] = actionCandidate{action: act}

		for _, cand := range candidates {
			if cand.Name != string(act) {
				continue
			}

			stats[index].share = cand.Probability
			stats[index].support = cand.Support
			stats[index].observed = true
			totalSupport += cand.Support

			if cand.Support > 0 {
				supportedCount++
			}

			break
		}
	}

	// Case 1: Fresh or unseen context. No legal action has evidence in cognitive memory.
	// Explore among currently legal actions with symmetric uniform sampling.
	if supportedCount == 0 {
		chosenIdx := 0

		if rng != nil && len(legal) > 1 {
			chosenIdx = rng.Intn(len(legal))
		}

		return legal[chosenIdx], 0.5, 0, 0
	}

	// Case 2: Some actions have been observed, but an alternative legal action remains unseen.
	// If the observed action produced low/inhibited evidence (<= 0.5), explore the unseen alternative.
	if supportedCount < len(legal) {
		for _, stat := range stats {
			if stat.support > 0 && stat.share <= 0.5 {
				for _, unsupp := range stats {
					if unsupp.support == 0 {
						return unsupp.action, 0.5, 0, 0
					}
				}
			}
		}
	}

	// Case 3: Select the legal action with maximum evidence share.
	bestIndex := 0
	bestShare := -1.0
	runnerUpShare := -1.0

	for index, stat := range stats {
		if stat.share > bestShare {
			runnerUpShare = bestShare
			bestShare = stat.share
			bestIndex = index
			continue
		}

		if stat.share > runnerUpShare {
			runnerUpShare = stat.share
		}
	}

	contrast := 0.0

	if runnerUpShare >= 0 {
		contrast = bestShare - runnerUpShare
	}

	return stats[bestIndex].action, bestShare, contrast, totalSupport
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
	action, confidence, contrast, support := selectLegalAction(legal, evaluation.Candidates, agent.rng)

	agent.recordAnswer(&telemetry.LearningAnswerT{
		Asked:      "action",
		Answered:   string(action),
		RunnerUp:   evaluation.RunnerUp,
		Confidence: confidence,
		Contrast:   contrast,
		Ambiguity:  evaluation.Ambiguity,
		Support:    support,
	})

	return action, sequence, confidence, contrast, support
}

/*
RehearseChild plays out the current child sequence starting from a random
point within the precursor (A < B), steps perception on each frame, chooses legal
actions, evaluates their executable consequence and timing, and reinforces the
temporal-context / action pair in cognitive memory.
*/
func (agent *Agent) RehearseChild() (int, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	var replay types.ReplayFragment

	if agent.replays != nil && agent.replays.Len() > 0 {
		if val, ok := agent.replays.CurrentValue(); ok {
			replay = val
		}
	}

	if len(replay.Frames) == 0 && agent.ring != nil && agent.ring.Len() > 0 {
		frames := agent.ring.CurrentChildValues()
		replay = types.ReplayFragment{
			Frames:      frames,
			AnchorIndex: max(1, len(frames)/2),
		}
	}

	childLen := len(replay.Frames)

	if childLen <= 0 {
		return 0, nil
	}

	// 1. Constrain A to strictly precursor development: 0 <= A < B
	anchorIdx := replay.AnchorIndex

	if anchorIdx <= 0 || anchorIdx >= childLen {
		anchorIdx = max(1, childLen-1)
	}

	offset := 0

	if anchorIdx > 1 {
		offset = agent.rng.Intn(anchorIdx)
	}

	// 2. Clear observation-local perception and context history before replay from A
	agent.space.Reset()
	agent.context.Reset()

	// 3. Configure objective evaluation parameters
	agent.evaluator.SetAnchorIndex(replay.AnchorIndex)
	agent.evaluator.SetPrice(agent.price)

	holding := false
	entryIdx := -1
	exitIdx := -1
	stepped := 0
	marks := make([]*telemetry.LearningMarkT, 0, childLen-offset)

	for frameIdx := offset; frameIdx < childLen; frameIdx++ {
		measurements := replay.Frames[frameIdx]

		if len(measurements) == 0 {
			continue
		}

		symbol := replay.Symbol

		if symbol == "" {
			for _, measurement := range measurements {
				if measurement != nil && measurement.Label != "" {
					symbol = measurement.Label
					break
				}
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

		var outcome ActionOutcome
		var evalErr error

		switch action {
		case ActionEnter:
			outcome, evalErr = agent.evaluator.EvaluateEntry(replay.Frames, frameIdx)

			if evalErr != nil {
				return stepped, errnie.Error(evalErr)
			}

			holding = true
			entryIdx = frameIdx
		case ActionExit:
			outcome, evalErr = agent.evaluator.EvaluateExit(replay.Frames, frameIdx, entryIdx)

			if evalErr != nil {
				return stepped, errnie.Error(evalErr)
			}

			holding = false
			exitIdx = frameIdx
		case ActionWait:
			outcome, evalErr = agent.evaluator.EvaluateWait(replay.Frames, frameIdx, holding, entryIdx)

			if evalErr != nil {
				return stepped, errnie.Error(evalErr)
			}
		}

		// Write the timing-modulated reinforcement to shared cognitive memory
		agent.learner.Engine().Observe(context, []byte(action), outcome.Reinforcement)

		marks = append(marks, &telemetry.LearningMarkT{
			Id:      uint64(len(marks)),
			Index:   int32(frameIdx),
			Kind:    string(action),
			Value:   outcome.Reinforcement,
			Graded:  true,
			Reduce:  holding,
			Verdict: fmt.Sprintf("c=%.2f t=%.2f r=%.2f", outcome.Correctness, outcome.Timing, outcome.Reinforcement),
		})
	}

	agent.lastOffset = offset
	agent.lastEntry = entryIdx
	agent.lastExit = exitIdx
	agent.lastMarks = marks

	if agent.replays != nil && agent.replays.Len() > 0 {
		agent.replays.Advance()
	}

	if agent.ring != nil && agent.ring.Len() > 0 {
		agent.ring.Advance()
	}

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
