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

	if !record.Tradable {
		fields := requirement.auditFields(symbol, record.PredictedReturn)
		fields["source"] = record.Source
		fields["contributions"] = record.Contributions
		crypto.recordEntrySkip(prediction, "forward_model_not_ready", fields)

		return nil
	}

	crypto.tryEnter(prediction, record.PredictedReturn)

	return nil
}

/*
reviewOpenPosition is the "is my edge still here?" loop. Positions in this
system are long, so the entry thesis is always "price advances". On each fresh
perspective for a symbol we already hold, we re-read that thesis: if the live
direction has flipped down, or the (source, regime) forward-return estimate has
gone negative, the reason we entered is gone and we exit now via
ExitReasonEdgeFaded rather than holding to the runway timer.

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

	// Thesis still intact: still pointing up and still a non-negative edge.
	if prediction.Direction >= 0 && predictedReturn >= 0 {
		return
	}

	audit("edge_faded_exit", map[string]any{
		"symbol":           symbol,
		"direction":        prediction.Direction,
		"predicted_return": predictedReturn,
	})

	if err := crypto.handleExit(engine.Exit{
		Symbol:  symbol,
		Urgency: 1,
		Reason:  engine.ExitReasonEdgeFaded,
	}); err != nil {
		errnie.Error(err)
	}
}
