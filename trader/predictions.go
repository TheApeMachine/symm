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
	predictedReturn := crypto.forecasts.RecordPerspective(symbol, prediction.Perspective, now)

	prediction.ExpectedReturn = predictedReturn
	prediction.PredictedAt = now
	prediction.Runway = runwayForPerspective(prediction.Perspective)
	prediction.DueAt = now.Add(prediction.Runway)

	crypto.predictions = append(crypto.predictions, &prediction)

	audit("perspective_ready", map[string]any{
		"symbol":            symbol,
		"confidence":        prediction.Confidence,
		"predicted_return":  predictedReturn,
		"direction":         prediction.Direction,
		"runway_ms":         prediction.Runway.Milliseconds(),
		"perspective_type":  prediction.Perspective.Type,
		"measurement_count": len(prediction.Perspective.Measurements),
	})

	crypto.tryEnter(prediction, predictedReturn)

	return nil
}
