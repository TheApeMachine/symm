package strategy

import (
	"encoding/binary"
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
feature fusion, an associative evaluator for reinforcement, and an associative learner
writing to a shared or private cognition memory trie.
*/
type Agent struct {
	mu        sync.RWMutex
	answersMu sync.RWMutex
	id        int
	isLive    bool
	space     *grid.Space
	evaluator *associative.EvaluatorOp
	learner   *associative.Agent
	ring      *store.Ring[[]*data.Measurement[float64]]
	answers   []*telemetry.LearningAnswerT
}

func NewAgent(id int, isLive bool, engine *cognition.Engine, windowBins int) *Agent {
	space := grid.NewSpaceWithWindow(windowBins)
	learner := associative.NewAgent(engine)
	evaluator := associative.Evaluator(engine)

	return &Agent{
		id:        id,
		isLive:    isLive,
		space:     space,
		evaluator: evaluator,
		learner:   learner,
		ring:      store.NewRing[[]*data.Measurement[float64]](),
		answers:   make([]*telemetry.LearningAnswerT, 0, 256),
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
RehearseChild plays out the current child sequence starting from a random
point between A and B, steps the agent's perception and learning on each frame,
and advances the parent ring.
*/
func (agent *Agent) RehearseChild() (int, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	if agent.ring == nil || agent.ring.Len() == 0 {
		return 0, nil
	}

	childLen := agent.ring.ChildLen()

	if childLen <= 0 {
		return 0, nil
	}

	offset := 0

	if childLen > 2 {
		offset = rand.Intn(childLen - 1)
	}

	stepped := 0

	for primitive := range agent.ring.NextOffset(nil, offset) {
		measurements := primitive.Read()

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

		if _, err := agent.stepLocked(measurements, symbol); err != nil {
			return stepped, err
		}

		stepped++
	}

	return stepped, nil
}

// Step advances this agent's private perception and memory
func (agent *Agent) Step(measurements []*data.Measurement[float64], symbol string) (grid.Impulse, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	return agent.stepLocked(measurements, symbol)
}

func (agent *Agent) stepLocked(measurements []*data.Measurement[float64], symbol string) (grid.Impulse, error) {
	// 1. Step private grid (fuses raw signals + resonance + flow into spatial coordinates)
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

	// 2. Extract active region impulses
	now := time.Now().UTC()
	impulse, err := agent.space.Impulse(label, now, now)

	if err != nil || !impulse.Ready {
		return impulse, err
	}

	// 3. Learn via associative evaluator (reinforces correct precursors, inhibits wrong ones)
	assoc, reading, err := agent.evaluator.Evaluate(impulse)

	if err != nil {
		return impulse, err
	}

	if reading.WinnerClass != "" {
		asked := string(assoc.Class)

		if asked == "" {
			asked = "excursion"
		}
		agent.recordAnswer(&telemetry.LearningAnswerT{
			Asked:      asked,
			Answered:   reading.WinnerClass,
			RunnerUp:   reading.RunnerUp,
			Confidence: reading.Confidence,
			Contrast:   reading.Contrast,
			Ambiguity:  reading.Ambiguity,
			Support:    reading.Support,
		})

		if len(assoc.Class) > 0 && reading.WinnerClass != string(assoc.Class) {
			agent.learner.Engine().Observe(assoc.Context, []byte(reading.WinnerClass), -reading.Confidence)
		}
	}

	_, err = agent.learner.LearnAssociation(assoc)

	return impulse, err
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

/*
PrecursorSequence encodes the active impulse regions into a binary token sequence.
*/
func PrecursorSequence(impulse grid.Impulse) []byte {
	if len(impulse.Regions) == 0 {
		return nil
	}

	sequence := make([]byte, 0, len(impulse.Regions)*8)
	var token [8]byte

	for _, region := range impulse.Regions {
		binary.BigEndian.PutUint64(token[:], region.Condition)
		sequence = append(sequence, token[:]...)
	}

	return sequence
}

/*
EvaluatePrecursor reads what this agent's associative memory predicts for
the given active impulse regions: enter_long, hold_long, exit_long, wait, etc.
*/
func (agent *Agent) EvaluatePrecursor(impulse grid.Impulse) (action string, confidence float64, contrast float64, support uint64) {
	sequence := PrecursorSequence(impulse)

	if len(sequence) == 0 {
		return "wait", 0, 0, 0
	}

	evaluation := agent.learner.Engine().Evaluate(sequence)
	action = evaluation.WinnerClass

	if action == "" {
		action = "wait"
	}

	agent.recordAnswer(&telemetry.LearningAnswerT{
		Asked:      "precursor",
		Answered:   action,
		RunnerUp:   evaluation.RunnerUp,
		Confidence: evaluation.Confidence,
		Contrast:   evaluation.Contrast,
		Ambiguity:  evaluation.Ambiguity,
		Support:    evaluation.Support,
	})

	return action, evaluation.Confidence, evaluation.Contrast, evaluation.Support
}


