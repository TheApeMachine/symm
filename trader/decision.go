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
	Symbol          string
	Source          string
	Regime          string
	Reason          string
	Score           float64
	ExpectedReturn  float64
	Allow           bool
	Why             string
}

/*
DecisionSnapshot is the execution truth for one rescore tick.
*/
type DecisionSnapshot struct {
	Line        float64
	Median      float64
	MAD         float64
	Warming     bool
	Evaluations []Evaluation
	Decisions   []Decision
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
DecisionEngine builds allow/deny snapshots from candidates and quotes.
*/
type DecisionEngine struct{}

/*
Build scores candidates, applies the entry line, and gates on post-cost edge.
*/
func (engine *DecisionEngine) Build(
	candidates CandidateStore,
	quotes QuoteReader,
	warming bool,
) DecisionSnapshot {
	evaluations, decisions, scores := engine.buildRows(candidates)
	line, median, mad := entryLine(scores)
	engine.applyGates(evaluations, decisions, quotes, warming, line)

	return DecisionSnapshot{
		Line:        line,
		Median:      median,
		MAD:         mad,
		Warming:     warming,
		Evaluations: evaluations,
		Decisions:   decisions,
	}
}

func (engine *DecisionEngine) buildRows(
	candidates CandidateStore,
) ([]Evaluation, []Decision, []float64) {
	evaluations := make([]Evaluation, 0, len(candidates.bySymbol))
	decisions := make([]Decision, 0)
	scores := make([]float64, 0, len(candidates.bySymbol))

	for symbol, sources := range candidates.bySymbol {
		combined := 0.0
		support := 0
		topRegime := ""
		topReason := ""
		topConfidence := 0.0
		topDirection := 1
		expectedReturn := 0.0
		runway := time.Duration(0)

		for _, candidate := range sources {
			support++
			combined += candidate.Confidence

			if candidate.ExpectedReturn > expectedReturn {
				expectedReturn = candidate.ExpectedReturn
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
				Score:          candidate.Confidence,
				ExpectedReturn: candidate.ExpectedReturn,
				Allow:          false,
				Why:            "below_line",
			})
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
			Side:           sideLabel(topDirection),
			Allow:          false,
			Why:            "below_line",
		})
	}

	sort.Slice(evaluations, func(left, right int) bool {
		if evaluations[left].CombinedScore != evaluations[right].CombinedScore {
			return evaluations[left].CombinedScore > evaluations[right].CombinedScore
		}

		return evaluations[left].Symbol < evaluations[right].Symbol
	})

	return evaluations, decisions, scores
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
	spreadCost := 0.0
	slippageCost := 2 * config.System.SlippageBPS / 10000

	if quotes != nil && symbol != "" {
		last, bid, ask, _, ok := quotes.Quote(symbol)

		if ok && last > 0 && bid > 0 && ask > 0 && ask >= bid {
			spreadCost = (ask - bid) / last
		}
	}

	minEdge := config.System.MinEdgeReturn

	if minEdge <= 0 {
		minEdge = 0
	}

	return roundTripFee + spreadCost + slippageCost + minEdge
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
