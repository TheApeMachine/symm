package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
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

	// Do not enter on a reused open forecast. Its anchor price and due time
	// belong to an earlier market state; waiting for settlement keeps feedback
	// flowing without letting stale confidence authorize a fresh position.
	if !record.Fresh {
		fields := requirement.auditFields(symbol, record.PredictedReturn)
		fields["source"] = record.Source
		fields["due_at"] = record.DueAt.UTC().Format(time.RFC3339Nano)
		crypto.recordEntrySkip(prediction, "open_prediction_pending", fields)

		return nil
	}

	crypto.predictions = append(crypto.predictions, &prediction)

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
