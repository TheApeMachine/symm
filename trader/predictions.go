package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func (crypto *Crypto) settlePredictions() {
	if len(crypto.predictions) == 0 {
		return
	}

	now := time.Now()
	remaining := crypto.predictions[:0]

	for _, due := range crypto.predictions {
		if !due.DueAt.Before(now) {
			remaining = append(remaining, due)
			continue
		}

		// Feedback emission is owned solely by price.Prediction.settleDue.
		// Emitting here too double-fed every signal forecast and the
		// Kelly/calibrator. This path now only flattens a position whose
		// runway has expired.
		lead, ok := due.LeadMeasurement()

		if !ok {
			continue
		}

		symbol := lead.Pairs[0].Wsname

		if crypto.holdsPrediction(
			crypto.wallet,
			symbol,
			engine.PerspectiveSource(due.Perspective.Type),
			due.DueAt,
		) {
			if err := crypto.handleExit(engine.Exit{
				Symbol:  symbol,
				Urgency: 1,
				Reason:  engine.ExitReasonRunwayExpired,
			}); err != nil {
				errnie.Error(err)
			}
		}
	}

	crypto.predictions = remaining
}

func (crypto *Crypto) actOnPrediction(prediction engine.Prediction) error {
	lead, ok := prediction.LeadMeasurement()

	if !ok {
		return fmt.Errorf("prediction missing lead measurement")
	}

	symbol := lead.Pairs[0].Wsname
	now := time.Now()
	prediction.Perspective.Regime = crypto.risk.MarketRegime(symbol)
	record := crypto.forecasts.RecordPerspective(symbol, prediction.Perspective, now)
	requirement := crypto.entryReturnRequirement(symbol, lead)

	prediction.ExpectedReturn = record.PredictedReturn
	prediction.PredictedAt = record.PredictedAt
	prediction.Runway = record.Runway
	prediction.DueAt = record.DueAt

	if record.Source == "" {
		crypto.recordEntrySkip(prediction, "prediction_record_empty", map[string]any{
			"symbol": symbol,
		})

		return nil
	}

	fields := requirement.auditFields(symbol, record.PredictedReturn)
	fields["source"] = record.Source
	fields["confidence"] = prediction.Confidence
	fields["direction"] = prediction.Direction
	fields["runway_ms"] = record.Runway.Milliseconds()
	fields["perspective_type"] = prediction.Perspective.Type
	fields["measurement_count"] = len(prediction.Perspective.Measurements)
	fields["fresh"] = record.Fresh
	fields["tradable"] = record.Tradable
	fields["contributions"] = record.Contributions

	audit("perspective_ready", fields)

	// If we already hold this symbol, this fresh perspective is not a new entry
	// candidate -- it is a live re-read of the thesis that opened the position.
	// Ask "is my edge still here?" and exit if it has decayed, instead of
	// waiting passively for the runway timer.
	if crypto.holdsSymbol(crypto.wallet, symbol) {
		crypto.reviewOpenPosition(symbol, prediction, record.PredictedReturn)

		return nil
	}

	// The decision tree -- not a break-even friction gate -- decides entry. It
	// reads the whole market story (every signal's verdict for this symbol) and
	// only authorizes a long when those verdicts tell a coherent bullish story.
	// A weak or contradictory story yields ActionNone and we simply pass.
	verdict := crypto.storyVerdict(symbol, false)

	if verdict.Action != engine.ActionEnter {
		fields := requirement.auditFields(symbol, record.PredictedReturn)
		fields["source"] = record.Source
		fields["verdict"] = verdict.Action.String()
		fields["node"] = verdict.Node
		fields["story_reason"] = verdict.Reason
		crypto.recordEntrySkip(prediction, "verdict_"+verdict.Action.String(), fields)

		return nil
	}

	crypto.tryEnter(prediction, record.PredictedReturn, verdict)

	return nil
}

/*
reviewOpenPosition is the "does the thesis still persist?" loop -- the exit half
of the same decision tree that authorized entry. On each fresh perspective for a
symbol we hold, we re-read the whole market story through Decide(holding=true):
a decay/reversal verdict (book collapsing, tape turning, driver downgraded to
beta, momentum fading) closes the position now rather than waiting for the
runway timer; a Hold verdict leaves the runway and stops as the backstop.

A position is given at least config.MinExhaustHold to clear its entry friction
before this thesis-decay exit can fire, so a momentary post-entry wobble does
not immediately churn the position back out at a loss.
*/
func (crypto *Crypto) reviewOpenPosition(
	symbol string,
	prediction engine.Prediction,
	predictedReturn float64,
) {
	if binding, ok := crypto.wallet.PositionBindingFor(symbolBase(symbol)); ok {
		if time.Since(binding.PredictedAt) < config.System.MinExhaustHold {
			return
		}
	}

	verdict := crypto.storyVerdict(symbol, true)

	if verdict.Action != engine.ActionExit {
		return
	}

	audit("edge_faded_exit", map[string]any{
		"symbol":           symbol,
		"direction":        prediction.Direction,
		"predicted_return": predictedReturn,
		"node":             verdict.Node,
		"story_reason":     verdict.Reason,
		"urgency":          verdict.Urgency,
	})

	if err := crypto.handleExit(engine.Exit{
		Symbol:  symbol,
		Urgency: verdict.Urgency,
		Reason:  engine.ExitReasonEdgeFaded,
	}); err != nil {
		errnie.Error(err)
	}
}
