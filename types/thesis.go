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
	}
}

/*
LifecycleState returns one symbol's current trade state. Absence means the
Thesis is still observing that symbol and has not crossed another boundary.
*/
func (thesis *Thesis) LifecycleState(symbol string) string {
	state := thesis.Lifecycle[symbol]

	if state == "" {
		return LifecycleObserving
	}

	return state
}

/*
Transition advances one symbol through the explicit trade lifecycle. Invalid
edges fail visibly rather than coercing a Position or Thesis into another state.
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
The trading path owns this sequence, so the Thesis needs no journal wrapper.
*/
func (thesis *Thesis) RecordTrade(observation TradeObservation) {
	thesis.TradeJournal = append(thesis.TradeJournal, observation)
}

/*
Absorb retains the current tick evidence used to manage one open position.
It copies only symbol-relevant derived state rather than raw transport frames.
*/
func (thesis *Thesis) Absorb(current *Thesis, symbol string) {
	for _, measurement := range current.Measurements {
		if measurement.Symbol == symbol {
			thesis.Measurements = append(thesis.Measurements, measurement)
		}
	}

	for _, forecast := range current.Forecasts {
		if forecast.Symbol == symbol {
			thesis.Forecasts = append(thesis.Forecasts, forecast)
		}
	}

	for _, hypothesis := range current.Hypotheses {
		if hypothesis.Symbol == symbol {
			thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		}
	}

	for _, category := range current.Categories {
		if category.Symbol == symbol {
			thesis.Categories = append(thesis.Categories, category)
		}
	}

	if graph, exists := current.Graphs[symbol]; exists {
		thesis.Graphs[symbol] = graph
	}
}

/*
ObservePostExit retains the forecast epochs required to judge a completed
trade. The tail length comes from the traded Thesis's forecast horizons; the
current RLS forecast consumes only the current field state and has no recurrent
inference memory beyond that explicit horizon.
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
Publish exposes the measurements and current cross-sectional state accumulated
by this tick without delaying the trading path when no UI consumer is ready.
*/
func (thesis *Thesis) Publish() {
	leader, leadershipThreshold := thesis.CrossSection.Leadership()

	select {
	case thesis.uiHub <- datura.Map[any]{
		"diagnostics": []datura.Map[any]{
			{
				"metrics":             thesis.CrossSection.Metrics,
				"leader":              leader,
				"leadershipThreshold": leadershipThreshold,
				"breadth":             thesis.CrossSection.Breadth(),
			},
		},
		"measurements": thesis.Measurements,
		"decisions":    thesis.Decisions,
		"tradeJournal": thesis.TradeJournal,
		"lifecycle":    thesis.Lifecycle,
		"findings":     thesis.Findings,
	}.Marshal():
	default:
	}
}
