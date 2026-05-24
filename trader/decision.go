package trader

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/stats"
)

/*
SignalCandidate is one scored signal reading for a symbol at one rescore tick.
*/
type SignalCandidate struct {
	Symbol         string
	Source         string
	Regime         string
	Reason         string
	Confidence     float64
	ExpectedReturn float64
	Runway         time.Duration
	Direction      int
	ObservedAt     time.Time
}

/*
Evaluation is one symbol-level combined decision row.
*/
type Evaluation struct {
	Symbol         string
	CombinedScore  float64
	Support        int
	ExpectedReturn float64
	Runway         time.Duration
	Regime         string
	Reason         string
	Side           string
	Allow          bool
	Why            string
}

/*
Decision is one per-source decision row.
*/
type Decision struct {
	Symbol         string
	Source         string
	Regime         string
	Reason         string
	Score          float64
	ExpectedReturn float64
	Allow          bool
	Why            string
}

/*
DecisionSnapshot is the execution truth for one rescore tick.
*/
type DecisionSnapshot struct {
	Line         float64
	Median       float64
	MAD          float64
	Warming      bool
	MarketRegime string
	Evaluations  []Evaluation
	Decisions    []Decision
}

/*
CandidateStore holds per-tick signal candidates keyed by symbol and source.
*/
type CandidateStore struct {
	bySymbol map[string]map[string]SignalCandidate
}

/*
NewCandidateStore creates an empty candidate store.
*/
func NewCandidateStore() CandidateStore {
	return CandidateStore{
		bySymbol: make(map[string]map[string]SignalCandidate),
	}
}

/*
Reset clears all candidates for the next rescore tick.
*/
func (store *CandidateStore) Reset() {
	store.bySymbol = make(map[string]map[string]SignalCandidate)
}

/*
Symbols returns every symbol with at least one candidate this tick.
*/
func (store *CandidateStore) Symbols() []string {
	symbols := make([]string, 0, len(store.bySymbol))

	for symbol := range store.bySymbol {
		symbols = append(symbols, symbol)
	}

	return symbols
}

/*
Note records or upgrades one candidate for symbol/source.
*/
func (store *CandidateStore) Note(candidate SignalCandidate) {
	if candidate.Symbol == "" || candidate.Source == "" || candidate.Confidence <= 0 {
		return
	}

	if store.bySymbol[candidate.Symbol] == nil {
		store.bySymbol[candidate.Symbol] = make(map[string]SignalCandidate)
	}

	existing := store.bySymbol[candidate.Symbol][candidate.Source]

	if existing.Confidence >= candidate.Confidence &&
		existing.ExpectedReturn >= candidate.ExpectedReturn {
		return
	}

	if candidate.Confidence > existing.Confidence {
		existing.Confidence = candidate.Confidence
		existing.Regime = candidate.Regime
		existing.Reason = candidate.Reason
		existing.Direction = candidate.Direction
	}

	if candidate.ExpectedReturn > existing.ExpectedReturn {
		existing.ExpectedReturn = candidate.ExpectedReturn
	}

	if candidate.Runway > existing.Runway {
		existing.Runway = candidate.Runway
	}

	existing.Symbol = candidate.Symbol
	existing.Source = candidate.Source
	existing.ObservedAt = candidate.ObservedAt

	store.bySymbol[candidate.Symbol][candidate.Source] = existing
}

/*
Len returns the number of symbol/source candidates recorded this tick.
*/
func (store *CandidateStore) Len() int {
	total := 0

	for _, sources := range store.bySymbol {
		total += len(sources)
	}

	return total
}

/*
DecisionEngine builds allow/deny snapshots from candidates and quotes.
*/
type DecisionEngine struct{}

/*
Build scores candidates with regime and trust weights, applies the entry line,
and gates on post-cost edge.
*/
func (engine *DecisionEngine) Build(
	candidates CandidateStore,
	quotes QuoteReader,
	warming bool,
	ensemble EnsembleContext,
) DecisionSnapshot {
	evaluations, decisions, scores := engine.buildRows(candidates, ensemble)
	line, median, mad := entryLine(scores)
	engine.applyGates(evaluations, decisions, quotes, warming, line)

	return DecisionSnapshot{
		Line:         line,
		Median:       median,
		MAD:          mad,
		Warming:      warming,
		MarketRegime: regimeLabel(ensemble.Regime),
		Evaluations:  evaluations,
		Decisions:    decisions,
	}
}

func (engine *DecisionEngine) buildRows(
	candidates CandidateStore,
	ensemble EnsembleContext,
) ([]Evaluation, []Decision, []float64) {
	evaluations := make([]Evaluation, 0, len(candidates.bySymbol))
	decisions := make([]Decision, 0)
	scores := make([]float64, 0, len(candidates.bySymbol))

	for symbol, sources := range candidates.bySymbol {
		perspectives := scorePerspectives(sources, ensemble)
		combined, activePerspectives := combinePerspectives(perspectives)
		weightedReturn := 0.0
		returnWeight := 0.0
		support := 0
		topRegime := ""
		topReason := ""
		topConfidence := 0.0
		topDirection := 1
		runway := time.Duration(0)

		for _, candidate := range sources {
			regime := RegimeWeight(ensemble.Regime, candidate.Source)

			if regime <= 0 || !regimeAllowsSource(ensemble.Regime, candidate.Source) {
				continue
			}

			effective := ensembleWeight(ensemble, candidate) * regimeGateScale(ensemble.Regime, candidate.Source)

			if effective <= 0 {
				continue
			}

			support++

			if effective > 0 && candidate.ExpectedReturn > 0 {
				weightedReturn += effective * candidate.ExpectedReturn
				returnWeight += effective
			}

			if candidate.Runway > runway {
				runway = candidate.Runway
			}

			if candidate.Confidence >= topConfidence {
				topConfidence = candidate.Confidence
				topRegime = candidate.Regime
				topReason = candidate.Reason
				topDirection = candidate.Direction
			}

			decisions = append(decisions, Decision{
				Symbol:         symbol,
				Source:         candidate.Source,
				Regime:         candidate.Regime,
				Reason:         candidate.Reason,
				Score:          effective,
				ExpectedReturn: candidate.ExpectedReturn,
				Allow:          false,
				Why:            "below_line",
			})
		}

		expectedReturn := 0.0

		if returnWeight > 0 {
			expectedReturn = weightedReturn / returnWeight
		}

		scores = append(scores, combined)

		evaluations = append(evaluations, Evaluation{
			Symbol:         symbol,
			CombinedScore:  combined,
			Support:        support,
			ExpectedReturn: expectedReturn,
			Runway:         runway,
			Regime:         topRegime,
			Reason:         topReason,
			Side:           directionSide(topDirection),
			Allow:          false,
			Why:            perspectiveWhy(activePerspectives, support),
		})
	}

	sort.Slice(evaluations, func(left, right int) bool {
		if evaluations[left].CombinedScore != evaluations[right].CombinedScore {
			return evaluations[left].CombinedScore > evaluations[right].CombinedScore
		}

		if evaluations[left].ExpectedReturn != evaluations[right].ExpectedReturn {
			return evaluations[left].ExpectedReturn > evaluations[right].ExpectedReturn
		}

		return evaluations[left].Symbol < evaluations[right].Symbol
	})

	return evaluations, decisions, scores
}

func ensembleWeight(ensemble EnsembleContext, candidate SignalCandidate) float64 {
	regime := RegimeWeight(ensemble.Regime, candidate.Source)
	trust := 1.0

	if ensemble.Trust != nil {
		trust = ensemble.Trust.Weight(candidate.Source)
	}

	return candidate.Confidence * regime * trust
}

func (engine *DecisionEngine) applyGates(
	evaluations []Evaluation,
	decisions []Decision,
	quotes QuoteReader,
	warming bool,
	line float64,
) {
	for index := range evaluations {
		evaluation := &evaluations[index]
		allow, why := engine.allowEvaluation(evaluation, quotes, warming, line)
		evaluation.Allow = allow
		evaluation.Why = why
	}

	for index := range decisions {
		decision := &decisions[index]
		allow, why := engine.allowDecision(decision, quotes, warming, line)
		decision.Allow = allow
		decision.Why = why
	}
}

func (engine *DecisionEngine) allowEvaluation(
	evaluation *Evaluation,
	quotes QuoteReader,
	warming bool,
	line float64,
) (bool, string) {
	if warming {
		return false, "field_warming"
	}

	if evaluation.CombinedScore < line {
		return false, "below_line"
	}

	if evaluation.CombinedScore <= 0 {
		return false, "below_line"
	}

	minPerspectives := config.System.MinActivePerspectives

	if minPerspectives <= 0 {
		minPerspectives = 1
	}

	if evaluation.Support < minPerspectives {
		return false, "thin_support"
	}

	requiredEdge := requiredEdgeReturn(quotes, evaluation.Symbol)

	if evaluation.ExpectedReturn <= requiredEdge {
		return false, "negative_edge"
	}

	return true, "ok"
}

func (engine *DecisionEngine) allowDecision(
	decision *Decision,
	quotes QuoteReader,
	warming bool,
	line float64,
) (bool, string) {
	if warming {
		return false, "field_warming"
	}

	if decision.Score < line {
		return false, "below_line"
	}

	if decision.Score <= 0 {
		return false, "below_line"
	}

	requiredEdge := requiredEdgeReturn(quotes, decision.Symbol)

	if decision.ExpectedReturn <= requiredEdge {
		return false, "negative_edge"
	}

	return true, "ok"
}

func requiredEdgeReturn(quotes QuoteReader, symbol string) float64 {
	roundTripFee := 2 * config.System.TakerFeePct / 100
	slippageCost := 2 * config.System.SlippageBPS / 10000
	minEdge := config.System.MinEdgeReturn

	if minEdge <= 0 {
		minEdge = 0
	}

	return roundTripFee + slippageCost + minEdge
}

func entryLine(scores []float64) (line, median, mad float64) {
	if len(scores) == 0 {
		return 0, 0, 0
	}

	sorted := stats.CopySorted(scores)
	median = stats.PercentileSorted(sorted, 0.5)
	mad = stats.MedianAbsoluteDeviation(sorted, median)
	line = median + mad

	return line, median, mad
}

func evaluationToMap(evaluation Evaluation) map[string]any {
	return map[string]any{
		"symbol":          evaluation.Symbol,
		"combined":        evaluation.CombinedScore,
		"support":         evaluation.Support,
		"expected_return": evaluation.ExpectedReturn,
		"runway_ms":       evaluation.Runway.Milliseconds(),
		"regime":          evaluation.Regime,
		"reason":          evaluation.Reason,
		"side":            evaluation.Side,
		"allow":           evaluation.Allow,
		"why":             evaluation.Why,
	}
}

func decisionToMap(decision Decision) map[string]any {
	return map[string]any{
		"symbol":          decision.Symbol,
		"source":          decision.Source,
		"regime":          decision.Regime,
		"reason":          decision.Reason,
		"score":           decision.Score,
		"confidence":      decision.Score,
		"effective_score": decision.Score,
		"expected_return": decision.ExpectedReturn,
		"in_play":         true,
		"allow":           decision.Allow,
		"why":             decision.Why,
	}
}

func directionSide(direction int) string {
	if direction < 0 {
		return "short"
	}

	return "long"
}

func perspectiveWhy(activePerspectives, support int) string {
	if activePerspectives <= 0 {
		return "no_perspective"
	}

	if support <= 0 {
		return "below_line"
	}

	return "below_line"
}
