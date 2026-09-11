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

/* IsUnprimed reports whether the agent space has processed any observation. */
func (agent *Agent) IsUnprimed() bool {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.space.UpdatedLabel == ""
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

	clonedFrames := make([][]*data.Measurement[float64], len(fragment.Frames))

	for frameIdx, frame := range fragment.Frames {
		clonedFrame := make([]*data.Measurement[float64], len(frame))

		for measIdx, meas := range frame {
			if meas != nil {
				clonedFrame[measIdx] = meas.Clone()
			}
		}

		clonedFrames[frameIdx] = clonedFrame
	}

	fragCopy := fragment
	fragCopy.Frames = clonedFrames

	agent.replays.WriteAt(fragCopy, slot)

	child := store.NewRing[[]*data.Measurement[float64]]()

	for _, frame := range clonedFrames {
		child.Write(frame)
	}

	agent.ring.WriteAt(child, slot)
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
	isLive bool,
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
	// In live mode (isLive == true or rng == nil), strictly default to ActionWait.
	// In rehearsal exploration (!isLive and rng != nil), explore among currently legal actions with symmetric uniform sampling.
	if supportedCount == 0 {
		if isLive || rng == nil {
			return ActionWait, 0, 0, 0
		}

		chosenIdx := 0

		if len(legal) > 1 {
			chosenIdx = rng.Intn(len(legal))
		}

		return legal[chosenIdx], 0.5, 0, 0
	}

	// Case 2: Some actions have been observed, but an alternative legal action remains unseen.
	// Only rehearsal exploration (!isLive) explores the unseen alternative when observed action has low evidence share.
	if !isLive && supportedCount < len(legal) {
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
) ActionDecision {
	sequence := agent.context.Sequence(impulse)

	if len(sequence) == 0 {
		return ActionDecision{Action: ActionWait}
	}

	evaluation := agent.learner.Engine().Evaluate(sequence)

	if evaluation.IsBreak {
		agent.context.Reset()
	}

	legal := LegalActions(holding)
	action, confidence, contrast, support := selectLegalAction(legal, evaluation.Candidates, agent.isLive, agent.rng)

	return ActionDecision{
		Action:     action,
		Context:    sequence,
		Confidence: confidence,
		Contrast:   contrast,
		Support:    support,
		RunnerUp:   evaluation.RunnerUp,
		Ambiguity:  evaluation.Ambiguity,
	}
}

type rehearsalTransition struct {
	context    []byte
	action     Action
	frameIdx   int
	holding    bool
	entryIdx   int
	runnerUp   string
	confidence float64
	contrast   float64
	ambiguity  float64
	support    uint64
}

/*
RehearseChild plays out the current child sequence starting from a random
point within the precursor (0 <= A < B). During rollout, all decisions are made
strictly using the policy as it existed prior to this replay. Evaluator judgments
and reinforcement are applied post-hoc once the rollout completes, eliminating
intra-fragment hindsight leakage.
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

	childLen := len(replay.Frames)

	if childLen <= 0 {
		if agent.replays != nil && agent.replays.Len() > 0 {
			agent.replays.Advance()
		}

		if agent.ring != nil && agent.ring.Len() > 0 {
			agent.ring.Advance()
		}

		return 0, nil
	}

	// 1. Constrain A to strictly precursor development: 0 <= A < B.
	// Factual anchor B must be valid; missing anchor is an explicit error, never a fallback.
	anchorIdx := replay.AnchorIndex

	if anchorIdx <= 0 || anchorIdx >= childLen {
		if agent.replays != nil && agent.replays.Len() > 0 {
			agent.replays.Advance()
		}

		if agent.ring != nil && agent.ring.Len() > 0 {
			agent.ring.Advance()
		}

		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"rehearsal: invalid or missing anchor index B in replay fragment",
			nil,
		))
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
	agent.evaluator.SetSurfaces(replay.Surfaces)
	agent.evaluator.SetPrices(replay.Prices)
	agent.evaluator.SetPrice(agent.price)

	holding := false
	entryIdx := -1
	exitIdx := -1
	stepped := 0

	transitions := make([]rehearsalTransition, 0, childLen-offset)

	// Phase 1: Forward rollout using current policy. No in-rollout hindsight mutation.
	for frameIdx := offset; frameIdx < childLen; frameIdx++ {
		measurements := replay.Frames[frameIdx]

		if len(measurements) == 0 {
			continue
		}

		symbol := replay.Symbol

		if symbol == "" {
			for _, measurement := range measurements {
				if measurement != nil && measurement.Label != "" && measurement.Label != "price" && measurement.Label != "last" {
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

		decision := agent.ChooseAction(impulse, holding)

		if len(decision.Context) == 0 {
			continue
		}

		transitions = append(transitions, rehearsalTransition{
			context:    decision.Context,
			action:     decision.Action,
			frameIdx:   frameIdx,
			holding:    holding,
			entryIdx:   entryIdx,
			runnerUp:   decision.RunnerUp,
			confidence: decision.Confidence,
			contrast:   decision.Contrast,
			ambiguity:  decision.Ambiguity,
			support:    decision.Support,
		})

		if decision.Action == ActionEnter {
			holding = true
			entryIdx = frameIdx
		}

		if decision.Action == ActionExit {
			holding = false
			exitIdx = frameIdx
		}
	}

	// Phase 2: Post-hoc evaluation and reinforcement after episode has played out.
	marks := make([]*telemetry.LearningMarkT, 0, len(transitions))

	for _, tr := range transitions {
		var outcome ActionOutcome
		var evalErr error

		switch tr.action {
		case ActionEnter:
			outcome, evalErr = agent.evaluator.EvaluateEntry(replay.Frames, tr.frameIdx)
		case ActionExit:
			outcome, evalErr = agent.evaluator.EvaluateExit(replay.Frames, tr.frameIdx, tr.entryIdx)
		case ActionWait:
			outcome, evalErr = agent.evaluator.EvaluateWait(replay.Frames, tr.frameIdx, tr.holding, tr.entryIdx)
		}

		if evalErr != nil {
			return stepped, errnie.Error(evalErr)
		}

		agent.learner.Engine().Observe(tr.context, []byte(tr.action), outcome.Reinforcement)

		marks = append(marks, &telemetry.LearningMarkT{
			Id:      uint64(len(marks)),
			Index:   int32(tr.frameIdx),
			Kind:    string(tr.action),
			Value:   outcome.Reinforcement,
			Graded:  true,
			Reduce:  tr.holding,
			Verdict: fmt.Sprintf("c=%.2f t=%.2f r=%.2f", outcome.Correctness, outcome.Timing, outcome.Reinforcement),
		})

		groundTruth := tr.action

		if outcome.Correctness <= 0 {
			if !tr.holding {
				groundTruth = ActionWait

				if tr.action == ActionWait {
					groundTruth = ActionEnter
				}
			}

			if tr.holding {
				groundTruth = ActionWait

				if tr.action == ActionWait {
					groundTruth = ActionExit
				}
			}
		}

		agent.recordAnswer(&telemetry.LearningAnswerT{
			Asked:      string(groundTruth),
			Answered:   string(tr.action),
			RunnerUp:   tr.runnerUp,
			Confidence: tr.confidence,
			Contrast:   tr.contrast,
			Ambiguity:  tr.ambiguity,
			Support:    tr.support,
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
