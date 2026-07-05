package trader

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	dashboard "github.com/theapemachine/symm/ui"
)

type Publisher interface {
	Publish(dashboard.Message) error
}

/*
UI publishes typed trader updates to the dashboard hub.
*/
type UI struct {
	publisher Publisher
	ticks     atomic.Int64
}

func NewUI(publisher Publisher) *UI {
	return &UI{
		publisher: publisher,
	}
}

func (ui *UI) Publish(
	role string,
	at time.Time,
	regime market.RegimeReading,
	measurements []*logic.Measurement,
	actions []*logic.Action,
	readings map[string]market.CognitiveReading,
	snapshots []SignalSnapshot,
) error {
	if role == channelTicker {
		if err := ui.regime(regime); err != nil {
			return err
		}
	}

	if len(readings) > 0 {
		if err := ui.cognitive(readings, at); err != nil {
			return err
		}
	}

	for _, snapshot := range snapshots {
		message := dashboard.Message{}

		switch snapshot.Source {
		case logic.SourceManifold:
			message.Manifold = snapshot.Payload
		case logic.SourceResonance:
			message.Resonance = snapshot.Payload
		default:
			return errnie.Err(
				errnie.Validation,
				"trader: unsupported ui snapshot source "+string(snapshot.Source),
				nil,
			)
		}

		if err := ui.publisher.Publish(message); err != nil {
			return err
		}
	}

	for _, measurement := range measurements {
		if err := ui.measurement(measurement); err != nil {
			return err
		}
	}

	return ui.tick(ui.ticks.Add(1), role, at, len(measurements), len(actions))
}

func (ui *UI) Decisions(actions []*logic.Action) error {
	tick := ui.ticks.Load()

	for _, action := range actions {
		if action == nil {
			return errnie.Err(errnie.Validation, "trader: nil ui decision", nil)
		}

		if err := ui.publisher.Publish(dashboard.Message{
			Decision: ui.decision(action, tick),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (ui *UI) Positions(
	readings []map[string]any,
	quote string,
	at time.Time,
) error {
	net := 0.0
	for _, reading := range readings {
		value, ok := reading["unrealizedPnl"].(float64)
		if ok {
			net += value
		}
	}

	return ui.publisher.Publish(dashboard.Message{
		Positions: &dashboard.Positions{
			Positions: readings,
			Count:     len(readings),
			Quote:     quote,
			Net:       net,
			At:        at.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (ui *UI) tick(
	count int64,
	phase string,
	at time.Time,
	measurementCount int,
	candidateCount int,
) error {
	return ui.publisher.Publish(dashboard.Message{
		Tick: &dashboard.Tick{
			Count:        count,
			Phase:        phase,
			Measurements: measurementCount,
			Candidates:   candidateCount,
			At:           at.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (ui *UI) regime(reading market.RegimeReading) error {
	return ui.publisher.Publish(dashboard.Message{
		Regime: &dashboard.Regime{
			Volatility: reading.Volatility,
			Trend:      reading.Trend,
			Bullish:    reading.Bullish,
			Bearish:    reading.Bearish,
			Choppiness: reading.Choppiness,
			Observed:   reading.Observed,
			Confidence: reading.Confidence,
			Strength:   reading.Strength,
			At:         reading.At.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (ui *UI) cognitive(
	readings map[string]market.CognitiveReading,
	at time.Time,
) error {
	return ui.publisher.Publish(dashboard.Message{
		Cognitive: &dashboard.Cognitive{
			Readings:  readings,
			At:        at.UTC().Format(time.RFC3339Nano),
			UpdatedAt: at.UnixNano(),
		},
	})
}

func (ui *UI) measurement(measurement *logic.Measurement) error {
	if measurement == nil {
		return errnie.Err(errnie.Validation, "trader: nil ui measurement", nil)
	}

	return ui.publisher.Publish(dashboard.Message{
		Measurement: &dashboard.Measurement{
			Source:        string(measurement.Source),
			Symbol:        measurement.Symbol,
			At:            measurement.At.UTC().Format(time.RFC3339Nano),
			Category:      string(measurement.DominantCategory()),
			Confidence:    measurement.Confidence,
			Strength:      measurement.Strength,
			Surprise:      measurement.Surprise,
			EntryBaseline: measurement.EntryBaseline,
			ExitBaseline:  measurement.ExitBaseline,
			Status:        measurement.Status,
			Distribution:  ui.distribution(measurement),
			Metrics:       measurement.Metrics,
		},
	})
}

func (ui *UI) decision(action *logic.Action, tick int64) *dashboard.Decision {
	score := action.EntryScore

	if score == 0 && action.EntryConfidence > 0 {
		score = action.EntryConfidence
	}

	if score == 0 {
		score = action.Fraction
	}

	return &dashboard.Decision{
		ID:              ui.id(action),
		Tick:            tick,
		Symbol:          action.Symbol,
		Type:            string(action.Type),
		Side:            string(action.Side),
		Verdict:         action.Verdict,
		Reason:          action.Reason,
		Score:           score,
		EntryScore:      action.EntryScore,
		EntryConfidence: action.EntryConfidence,
		Fraction:        action.Fraction,
		Quantity:        action.Quantity,
		Notional:        action.Notional,
		ActionID:        action.ActionID,
		DecisionID:      action.DecisionID,
		BranchKey:       action.BranchKey,
		ReasonSource:    string(action.ReasonSource),
		ReasonCategory:  string(action.ReasonCategory),
		DecisionAt:      action.DecisionAt,
	}
}

func (ui *UI) id(action *logic.Action) string {
	for _, id := range []string{action.DecisionID, action.ActionID} {
		if strings.TrimSpace(id) != "" {
			return id
		}
	}

	return uuid.NewString()
}

func (ui *UI) distribution(measurement *logic.Measurement) map[string]float64 {
	distribution := make(map[string]float64, len(measurement.Distribution))

	for category, mass := range measurement.Distribution {
		if category == logic.CategoryTypeNone || mass <= 0 {
			continue
		}

		distribution[string(category)] = mass
	}

	return distribution
}
