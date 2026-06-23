package trader

import (
	"math"
	"sort"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

type decisionTraceFrame struct {
	Type                string             `json:"type"`
	StoryTicks          uint64             `json:"story_ticks"`
	PlaybookEvaluations uint64             `json:"playbook_evaluations"`
	Decisions           []decisionTraceRow `json:"decisions"`
}

type decisionTraceRow struct {
	Symbol  string              `json:"symbol"`
	Source  string              `json:"source"`
	Score   float64             `json:"score"`
	Allow   bool                `json:"allow"`
	InPlay  bool                `json:"in_play"`
	Why     string              `json:"why"`
	Signals []decisionSignalRow `json:"signals"`
}

type decisionSignalRow struct {
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type symbolDecision struct {
	symbol       string
	source       string
	score        float64
	bestScore    float64
	allow        bool
	inPlay       bool
	why          string
	measurements []*datura.Artifact
	signals      []decisionSignalRow
}

func (crypto *Crypto) publishDecisionTrace(
	measurements []*datura.Artifact,
) *datura.Artifact {
	frame := crypto.decisionTraceFrame(measurements)

	if frame == nil || len(frame.Decisions) == 0 {
		return nil
	}

	payload, err := sonic.Marshal(frame)

	if err != nil {
		return nil
	}

	artifact := datura.Acquire("trader", datura.APPJSON)
	artifact.WithRole("decision_trace")
	artifact.WithScope("update")
	artifact.WithPayload(payload)

	return artifact
}

func (crypto *Crypto) decisionTraceFrame(
	measurements []*datura.Artifact,
) *decisionTraceFrame {
	decisions := decisionsFromMeasurements(measurements)

	if len(decisions) == 0 {
		return nil
	}

	entryLine := decisionEntryLine(decisions)
	playbookEvaluations := uint64(0)

	for index := range decisions {
		decisions[index].allow = crypto.allows(decisions[index])
		decisions[index].inPlay = decisions[index].allow || decisions[index].score >= entryLine
		decisions[index].why = decisionWhy(decisions[index])

		if decisions[index].allow {
			playbookEvaluations++
		}
	}

	sort.SliceStable(decisions, func(left, right int) bool {
		return decisions[left].score > decisions[right].score
	})

	return &decisionTraceFrame{
		Type:                "decision_trace",
		StoryTicks:          crypto.storyTicks.Add(1),
		PlaybookEvaluations: crypto.playbookEvaluations.Add(playbookEvaluations),
		Decisions:           decisionRows(decisions),
	}
}

func decisionsFromMeasurements(measurements []*datura.Artifact) []symbolDecision {
	bySymbol := map[string]*symbolDecision{}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol := measurementSymbol(measurement)

		if symbol == "" {
			continue
		}

		source, _ := measurement.Origin()
		confidence := datura.Peek[float64](measurement, "output", "confidence")

		if source == "" || !finitePositive(confidence) {
			continue
		}

		decision := bySymbol[symbol]

		if decision == nil {
			decision = &symbolDecision{symbol: symbol}
			bySymbol[symbol] = decision
		}

		decision.measurements = append(decision.measurements, measurement)
		decision.signals = append(decision.signals, decisionSignalRow{
			Source:     source,
			Confidence: confidence,
		})
		decision.score += confidence

		if confidence > decision.bestScore {
			decision.bestScore = confidence
			decision.source = source
		}
	}

	decisions := make([]symbolDecision, 0, len(bySymbol))

	for _, decision := range bySymbol {
		if len(decision.signals) == 0 {
			continue
		}

		decision.score /= float64(len(decision.signals))

		if decision.source == "" {
			decision.source = decision.signals[0].Source
		}

		decisions = append(decisions, *decision)
	}

	return decisions
}

func (crypto *Crypto) allows(decision symbolDecision) bool {
	if crypto == nil || crypto.story == nil {
		return false
	}

	verdict := crypto.story.Update(decision.symbol, decision.measurements)

	if verdict == nil {
		return false
	}

	return len(datura.Peek[[]any](verdict, "actions")) > 0
}

func decisionRows(decisions []symbolDecision) []decisionTraceRow {
	rows := make([]decisionTraceRow, 0, len(decisions))

	for _, decision := range decisions {
		rows = append(rows, decisionTraceRow{
			Symbol:  decision.symbol,
			Source:  decision.source,
			Score:   decision.score,
			Allow:   decision.allow,
			InPlay:  decision.inPlay,
			Why:     decision.why,
			Signals: append([]decisionSignalRow(nil), decision.signals...),
		})
	}

	return rows
}

func decisionEntryLine(decisions []symbolDecision) float64 {
	scores := make([]float64, 0, len(decisions))

	for _, decision := range decisions {
		if finitePositive(decision.score) {
			scores = append(scores, decision.score)
		}
	}

	if len(scores) == 0 {
		return 0
	}

	sort.Float64s(scores)

	median := scores[len(scores)/2]
	deviations := make([]float64, 0, len(scores))

	for _, score := range scores {
		deviations = append(deviations, math.Abs(score-median))
	}

	sort.Float64s(deviations)

	return median + deviations[len(deviations)/2]
}

func decisionWhy(decision symbolDecision) string {
	if decision.allow {
		return "matched_branch"
	}

	if decision.inPlay {
		return "above_entry"
	}

	return "below_edge"
}

func measurementSymbol(measurement *datura.Artifact) string {
	scope, _ := measurement.Scope()

	if scope != "" && scope != "update" && scope != "snapshot" {
		return scope
	}

	if symbol := datura.Peek[string](measurement, "data", 0, "symbol"); symbol != "" {
		return symbol
	}

	return datura.Peek[string](measurement, "symbol")
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
