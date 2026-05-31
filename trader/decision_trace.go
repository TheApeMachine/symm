package trader

import (
	"time"

	"github.com/theapemachine/qpool"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

const decisionTraceCapacity = 64

type decisionTraceRow struct {
	Symbol     string  `json:"symbol"`
	Regime     string  `json:"regime"`
	Reason     string  `json:"reason"`
	Score      float64 `json:"score"`
	Allow      bool    `json:"allow"`
	Why        string  `json:"why"`
	Confidence float64 `json:"confidence"`
	InPlay     bool    `json:"in_play"`
}

/*
recordEntryVerdicts stores playbook outcomes and publishes gate rejects for denies.
*/
func (crypto *Crypto) recordEntryVerdicts(
	symbol string,
	measurements []perspectives.Measurement,
	verdicts []decision.EntryVerdict,
) {
	score := thesisScore(measurements, entryNamesFromVerdicts(verdicts))
	snapshot := crypto.ensureCrossSection()
	inPlay := score > snapshot.Baseline

	for _, verdict := range verdicts {
		row := decisionTraceRow{
			Symbol:     symbol,
			Regime:     string(verdict.Regime),
			Reason:     verdict.Name,
			Score:      score,
			Allow:      verdict.Action == perspectives.ActionEnter,
			Why:        traceWhy(verdict),
			Confidence: traceConfidence(verdict),
			InPlay:     inPlay,
		}

		crypto.appendDecisionTrace(row)

		if perspectives.IsEntryBlocked(verdict.Action) {
			crypto.publishGateReject(symbol, verdict)
		}
	}
}

func (crypto *Crypto) appendDecisionTrace(row decisionTraceRow) {
	crypto.decisionTraceMu.Lock()
	defer crypto.decisionTraceMu.Unlock()

	crypto.decisionTraceRows = append(crypto.decisionTraceRows, row)

	if len(crypto.decisionTraceRows) > decisionTraceCapacity {
		crypto.decisionTraceRows = crypto.decisionTraceRows[len(crypto.decisionTraceRows)-decisionTraceCapacity:]
	}
}

func (crypto *Crypto) publishDecisionTrace() {
	if crypto.ui == nil {
		return
	}

	crypto.decisionTraceMu.Lock()
	rows := crypto.decisionTraceRows
	crypto.decisionTraceRows = nil
	crypto.decisionTraceMu.Unlock()

	if len(rows) == 0 {
		return
	}

	payload := make([]map[string]any, len(rows))

	for index, row := range rows {
		payload[index] = map[string]any{
			"symbol":     row.Symbol,
			"regime":     row.Regime,
			"reason":     row.Reason,
			"score":      row.Score,
			"allow":      row.Allow,
			"why":        row.Why,
			"confidence": row.Confidence,
			"in_play":    row.InPlay,
		}
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "decision_trace",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"decisions": payload,
	}})
}

func (crypto *Crypto) publishGateReject(symbol string, verdict decision.EntryVerdict) {
	fields := map[string]any{
		"playbook": verdict.Name,
		"action":   perspectives.ActionLabel(verdict.Action),
	}

	if step, ok := verdict.Trace.LastStep(); ok {
		fields["category"] = step.Category.String()
		fields["snr"] = step.SNR
		fields["threshold"] = step.Threshold
	}

	crypto.publishAudit("gate_reject", symbol, traceWhy(verdict), fields)
}

func entryNamesFromVerdicts(verdicts []decision.EntryVerdict) []string {
	names := make([]string, 0, len(verdicts))

	for _, verdict := range verdicts {
		if verdict.Action != perspectives.ActionEnter {
			continue
		}

		names = append(names, verdict.Name)
	}

	return names
}

func traceWhy(verdict decision.EntryVerdict) string {
	if verdict.Trace == nil {
		return perspectives.ActionLabel(verdict.Action)
	}

	step, ok := verdict.Trace.LastStep()

	if !ok {
		return perspectives.ActionLabel(verdict.Action)
	}

	if step.Category != perspectives.CategoryTypeNone {
		return step.Category.String() + "_" + perspectives.ActionLabel(step.Action)
	}

	return perspectives.ActionLabel(verdict.Action)
}

func traceConfidence(verdict decision.EntryVerdict) float64 {
	if verdict.Trace == nil {
		return 0
	}

	step, ok := verdict.Trace.LastStep()

	if !ok || step.Threshold <= 0 {
		return 0
	}

	return step.SNR / step.Threshold
}
