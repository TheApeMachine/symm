package strategy

import (
	"encoding/binary"
	"sync"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Agent encapsulates an autonomous cognitive unit: a private grid space for multi-signal
feature fusion, an associative evaluator for reinforcement, and an associative learner
writing to a shared or private cognition memory trie.
*/
type Agent struct {
	mu        sync.RWMutex
	id        int
	isLive    bool
	space     *grid.Space
	evaluator *associative.EvaluatorOp
	learner   *associative.Agent
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

// Step advances this agent's private perception and memory
func (agent *Agent) Step(measurements []*data.Measurement[float64], symbol string) (grid.Impulse, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

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

	if len(assoc.Class) > 0 && reading.WinnerClass != "" {
		agent.recordAnswer(&telemetry.LearningAnswerT{
			Asked:      string(assoc.Class),
			Answered:   reading.WinnerClass,
			RunnerUp:   reading.RunnerUp,
			Confidence: reading.Confidence,
			Contrast:   reading.Contrast,
			Ambiguity:  reading.Ambiguity,
			Support:    reading.Support,
		})

		if reading.WinnerClass != string(assoc.Class) {
			agent.learner.Engine().Observe(assoc.Context, []byte(reading.WinnerClass), -reading.Confidence)
		}
	}

	_, err = agent.learner.LearnAssociation(assoc)

	return impulse, err
}

func (agent *Agent) recordAnswer(answer *telemetry.LearningAnswerT) {
	const maxAnswers = 256

	if len(agent.answers) >= maxAnswers {
		agent.answers = append(agent.answers[1:], answer)

		return
	}
	agent.answers = append(agent.answers, answer)
}

/* Answers returns a copy of genuine out-of-sample streaming evaluations. */
func (agent *Agent) Answers() []*telemetry.LearningAnswerT {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	answers := make([]*telemetry.LearningAnswerT, len(agent.answers))
	copy(answers, agent.answers)

	return answers
}

/*
EvaluatePrecursor reads what this agent's associative memory predicts for
the given active impulse regions: enter_long, hold_long, exit_long, wait, etc.
*/
func (agent *Agent) EvaluatePrecursor(impulse grid.Impulse) (action string, confidence float64, contrast float64, support uint64) {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if len(impulse.Regions) == 0 {
		return "wait", 0, 0, 0
	}
	sequence := make([]byte, 0, len(impulse.Regions)*8)
	var token [8]byte

	for _, region := range impulse.Regions {
		binary.BigEndian.PutUint64(token[:], region.Condition)
		sequence = append(sequence, token[:]...)
	}
	evaluation := agent.learner.Engine().Evaluate(sequence)

	action = evaluation.WinnerClass
	if action == "" {
		action = "wait"
	}

	return action, evaluation.Confidence, evaluation.Contrast, evaluation.Support
}


