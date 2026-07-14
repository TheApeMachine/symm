package types

import (
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

const (
	LifecycleObserving           = "observing"
	LifecycleShaped              = "shaped"
	LifecycleEntrySelected       = "entry_selected"
	LifecycleEntrySubmitted      = "entry_submitted"
	LifecyclePartiallyEntered    = "partially_entered"
	LifecycleEntered             = "entered"
	LifecycleManaging            = "managing"
	LifecycleExitSelected        = "exit_selected"
	LifecycleExitSubmitted       = "exit_submitted"
	LifecyclePartiallyExited     = "partially_exited"
	LifecycleClosed              = "closed"
	LifecyclePostExitObservation = "post_exit_observation"
	LifecyclePostMortemReady     = "postmortem_ready"
	LifecycleEvaluated           = "evaluated"
	LifecycleExpired             = "expired"
	LifecycleRejected            = "rejected"
	LifecycleInvalid             = "invalid"
)

/*
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	uiHub        chan<- []byte
	Tick         int64
	Signals      *sync.Map
	CrossSection *CrossSection
	Measurements []*Measurement
	Graphs       map[string]*Graph
	Forecasts    []Forecasts
	Decisions    []Decision
	TradeJournal []TradeObservation
	Lifecycle    map[string]string
	Findings     []Finding
	Hypotheses   []Hypothesis
	Categories   []Category
	Manifold     []any
	Resonance    []any
	Causal       []any
}

/*
NewThesis creates an empty in-process lifecycle carrier for one tick.
*/
func NewThesis(uiHub chan<- []byte) *Thesis {
	return &Thesis{
		uiHub:        uiHub,
		Signals:      &sync.Map{},
		CrossSection: NewCrossSection(),
		Measurements: make([]*Measurement, 0),
		Graphs:       make(map[string]*Graph),
		Forecasts:    make([]Forecasts, 0),
		Decisions:    make([]Decision, 0),
		TradeJournal: make([]TradeObservation, 0),
		Lifecycle:    make(map[string]string),
		Findings:     make([]Finding, 0),
		Hypotheses:   make([]Hypothesis, 0),
		Categories:   make([]Category, 0),
		Manifold:     make([]any, 0),
		Resonance:    make([]any, 0),
		Causal:       make([]any, 0),
	}
}

/*
LifecycleState returns one symbol's state, defaulting to observation before a boundary is crossed.
*/
func (thesis *Thesis) LifecycleState(symbol string) string {
	state := thesis.Lifecycle[symbol]

	if state == "" {
		return LifecycleObserving
	}

	return state
}

/*
Transition advances one symbol through the trade lifecycle and rejects invalid edges.
*/
func (thesis *Thesis) Transition(symbol, next string, at time.Time) error {
	current := thesis.LifecycleState(symbol)

	if current == next {
		return nil
	}

	allowed := false

	switch current {
	case LifecycleObserving:
		allowed = next == LifecycleShaped || next == LifecycleManaging ||
			next == LifecycleExpired || next == LifecycleInvalid
	case LifecycleShaped:
		allowed = next == LifecycleEntrySelected || next == LifecycleExpired ||
			next == LifecycleInvalid
	case LifecycleEntrySelected:
		allowed = next == LifecycleEntrySubmitted || next == LifecycleRejected ||
			next == LifecycleInvalid
	case LifecycleEntrySubmitted:
		allowed = next == LifecyclePartiallyEntered || next == LifecycleEntered ||
			next == LifecycleRejected || next == LifecycleInvalid
	case LifecyclePartiallyEntered:
		allowed = next == LifecycleEntered || next == LifecycleManaging ||
			next == LifecycleRejected || next == LifecycleInvalid
	case LifecycleEntered:
		allowed = next == LifecycleManaging || next == LifecycleExitSelected ||
			next == LifecycleInvalid
	case LifecycleManaging:
		allowed = next == LifecycleExitSelected || next == LifecycleInvalid
	case LifecycleExitSelected:
		allowed = next == LifecycleExitSubmitted || next == LifecycleManaging ||
			next == LifecycleInvalid
	case LifecycleExitSubmitted:
		allowed = next == LifecyclePartiallyExited || next == LifecycleClosed ||
			next == LifecycleManaging || next == LifecycleInvalid
	case LifecyclePartiallyExited:
		allowed = next == LifecycleExitSubmitted || next == LifecycleClosed ||
			next == LifecycleManaging || next == LifecycleInvalid
	case LifecycleClosed:
		allowed = next == LifecyclePostExitObservation || next == LifecycleInvalid
	case LifecyclePostExitObservation:
		allowed = next == LifecyclePostMortemReady || next == LifecycleInvalid
	case LifecyclePostMortemReady:
		allowed = next == LifecycleEvaluated || next == LifecycleInvalid
	}

	if !allowed {
		return errnie.Err(
			errnie.Validation,
			"invalid lifecycle transition for "+symbol+": "+current+" -> "+next,
			nil,
		)
	}

	thesis.Lifecycle[symbol] = next
	thesis.RecordTrade(TradeObservation{
		Kind: "lifecycle_transition", Symbol: symbol, Status: next, At: at,
	})

	return nil
}

/*
RecordTrade appends an immutable broker or position fact in lifecycle order.
*/
func (thesis *Thesis) RecordTrade(observation TradeObservation) {
	thesis.TradeJournal = append(thesis.TradeJournal, observation)
}

/*
AbsorbFindings retains evaluated PostMortem findings from one completed lifecycle
on the current tick thesis so Publish can expose them on the UI wire.
*/
func (thesis *Thesis) AbsorbFindings(evaluated *Thesis) {
	if evaluated == nil || len(evaluated.Findings) == 0 {
		return
	}

	thesis.Findings = append(thesis.Findings, evaluated.Findings...)
}

/*
Absorb idempotently retains the current tick evidence used to manage one open position.
*/
func (thesis *Thesis) Absorb(current *Thesis, symbol string) {
	absorbedMeasurements := make(map[*Measurement]struct{})

	for _, measurement := range thesis.Measurements {
		if measurement.Symbol == symbol {
			absorbedMeasurements[measurement] = struct{}{}
		}
	}

	for _, measurement := range current.Measurements {
		if measurement.Symbol != symbol {
			continue
		}

		if _, exists := absorbedMeasurements[measurement]; exists {
			continue
		}

		absorbedMeasurements[measurement] = struct{}{}
		thesis.Measurements = append(thesis.Measurements, measurement)
	}

	absorbedForecasts := make(map[forecastAbsorbKey]struct{})

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol == symbol {
			absorbedForecasts[forecastAbsorbKey{
				sourceEpoch: forecast.SourceEpoch,
				at:          forecast.At,
			}] = struct{}{}
		}
	}

	for _, forecast := range current.Forecasts {
		if forecast.Symbol != symbol {
			continue
		}

		key := forecastAbsorbKey{
			sourceEpoch: forecast.SourceEpoch,
			at:          forecast.At,
		}

		if _, exists := absorbedForecasts[key]; exists {
			continue
		}

		absorbedForecasts[key] = struct{}{}
		thesis.Forecasts = append(thesis.Forecasts, forecast)
	}

	absorbedHypotheses := make(map[hypothesisAbsorbKey]struct{})

	for _, hypothesis := range thesis.Hypotheses {
		if hypothesis.Symbol == symbol {
			absorbedHypotheses[hypothesisAbsorbKey{
				source: hypothesis.Source,
				at:     hypothesis.At,
				claim:  hypothesis.Claim,
			}] = struct{}{}
		}
	}

	for _, hypothesis := range current.Hypotheses {
		if hypothesis.Symbol != symbol {
			continue
		}

		key := hypothesisAbsorbKey{
			source: hypothesis.Source,
			at:     hypothesis.At,
			claim:  hypothesis.Claim,
		}

		if _, exists := absorbedHypotheses[key]; exists {
			continue
		}

		absorbedHypotheses[key] = struct{}{}
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
	}

	absorbedCategories := make(map[CategoryType]struct{})

	for _, category := range thesis.Categories {
		if category.Symbol == symbol {
			absorbedCategories[category.Type] = struct{}{}
		}
	}

	for _, category := range current.Categories {
		if category.Symbol != symbol {
			continue
		}

		if _, exists := absorbedCategories[category.Type]; exists {
			continue
		}

		absorbedCategories[category.Type] = struct{}{}
		thesis.Categories = append(thesis.Categories, category)
	}

	if graph, exists := current.Graphs[symbol]; exists {
		thesis.Graphs[symbol] = graph
	}
}

type forecastAbsorbKey struct {
	sourceEpoch uint64
	at          time.Time
}

type hypothesisAbsorbKey struct {
	source SourceType
	at     time.Time
	claim  string
}

/*
ObservePostExit retains enough forecast epochs to judge a completed trade.
The traded Thesis's forecast horizons determine the required tail length.
*/
func (thesis *Thesis) ObservePostExit(current *Thesis, symbol string) error {
	state := thesis.LifecycleState(symbol)

	if state != LifecycleClosed && state != LifecyclePostExitObservation {
		return errnie.Err(
			errnie.Validation,
			"post-exit observation requires a closed lifecycle for "+symbol,
			nil,
		)
	}

	closedAt := time.Time{}

	for _, observation := range thesis.TradeJournal {
		if observation.Symbol == symbol && observation.Kind == "lifecycle_transition" &&
			observation.Status == LifecycleClosed {
			closedAt = observation.At
		}
	}

	if closedAt.IsZero() {
		return errnie.Err(errnie.Validation, "closed timestamp required for "+symbol, nil)
	}

	required := uint64(0)

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol == symbol && !forecast.At.After(closedAt) &&
			forecast.HorizonEvents > required {
			required = forecast.HorizonEvents
		}
	}

	if required == 0 {
		return errnie.Err(errnie.Validation, "forecast horizon required for "+symbol, nil)
	}

	thesis.Absorb(current, symbol)
	observed := make(map[uint64]struct{})
	latestAt := time.Time{}

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol != symbol || !forecast.At.After(closedAt) {
			continue
		}

		observed[forecast.SourceEpoch] = struct{}{}

		if forecast.At.After(latestAt) {
			latestAt = forecast.At
		}
	}

	if len(observed) == 0 {
		return nil
	}

	if state == LifecycleClosed {
		if err := thesis.Transition(
			symbol, LifecyclePostExitObservation, latestAt,
		); err != nil {
			return err
		}
	}

	if uint64(len(observed)) >= required {
		return thesis.Transition(symbol, LifecyclePostMortemReady, latestAt)
	}

	return nil
}

/*
Publish exposes the non-empty evidence accumulated by this tick without delaying trading.
*/
func (thesis *Thesis) Publish() {
	if thesis.uiHub == nil {
		return
	}

	if len(thesis.Measurements) == 0 && len(thesis.Decisions) == 0 &&
		len(thesis.TradeJournal) == 0 && len(thesis.Lifecycle) == 0 &&
		len(thesis.Findings) == 0 && len(thesis.CrossSection.Metrics) == 0 &&
		len(thesis.Manifold) == 0 && len(thesis.Resonance) == 0 && len(thesis.Causal) == 0 && thesis.Tick == 0 {
		return
	}

	leader, leadershipThreshold := thesis.CrossSection.Leadership()
	frame := datura.Map[any]{
		"tick": datura.Map[any]{"count": thesis.Tick},
		"diagnostics": []datura.Map[any]{
			{
				"metrics":             thesis.CrossSection.Metrics,
				"leader":              leader,
				"leadershipThreshold": leadershipThreshold,
				"breadth":             thesis.CrossSection.Breadth(),
			},
		},
	}

	if len(thesis.Measurements) > 0 {
		frame["measurements"] = thesis.Measurements
	}

	if len(thesis.Decisions) > 0 {
		frame["decisions"] = thesis.Decisions
	}

	if len(thesis.TradeJournal) > 0 {
		frame["tradeJournal"] = thesis.TradeJournal
	}

	if len(thesis.Lifecycle) > 0 {
		frame["lifecycle"] = thesis.Lifecycle
	}

	if len(thesis.Findings) > 0 {
		frame["findings"] = thesis.Findings
	}

	if len(thesis.Graphs) > 0 {
		graphs := make([]GraphFrame, 0, len(thesis.Graphs))

		for _, graph := range thesis.Graphs {
			graphs = append(graphs, graph.Frame())
		}

		frame["graphs"] = graphs
	}

	if len(thesis.Forecasts) > 0 {
		frame["forecasts"] = thesis.Forecasts
	}

	if len(thesis.Hypotheses) > 0 {
		frame["hypotheses"] = thesis.Hypotheses
	}

	if len(thesis.Categories) > 0 {
		frame["categories"] = thesis.Categories
	}

	if len(thesis.Manifold) > 0 {
		frame["manifold"] = thesis.Manifold
	}

	if len(thesis.Resonance) > 0 {
		frame["resonance"] = thesis.Resonance
	}

	if len(thesis.Causal) > 0 {
		frame["causal"] = thesis.Causal
	}

	select {
	case thesis.uiHub <- frame.Marshal():
	default:
	}
}
