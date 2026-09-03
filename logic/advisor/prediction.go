package advisor

import (
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func (feature *Feature) validatePredictions() error {
	if len(feature.Class.Predictions) == 0 {
		return nil
	}

	if feature.Class.Within == 0 {
		return errnie.Err(
			errnie.Validation,
			"predictive Class requires a positive horizon: "+feature.Class.Label,
			nil,
		)
	}

	for _, prediction := range feature.Class.Predictions {
		if err := prediction.validate(feature.Class.Label); err != nil {
			return err
		}
	}

	return nil
}

func (prediction *Prediction) validate(class string) error {
	if prediction == nil || prediction.Support == nil || prediction.Contradict == nil {
		return errnie.Err(
			errnie.Validation,
			"Prediction requires support and contradiction events for Class: "+class,
			nil,
		)
	}

	if err := prediction.Support.validate(class); err != nil {
		return err
	}

	return prediction.Contradict.validate(class)
}

func (event *Falsifiable) validate(class string) error {
	if event.Label == "" || event.Move == NOMOVE || event.Value < 0 || event.Unit > RAW {
		return errnie.Err(
			errnie.Validation,
			"Prediction event is invalid for Class: "+class,
			nil,
		)
	}

	if event.Type > LEVEL3 {
		return errnie.Err(
			errnie.Validation,
			"Prediction event must be observable: "+event.Label,
			nil,
		)
	}

	return nil
}

/* matches compares one observed value with the value present when issued. */
func (event *Falsifiable) matches(baseline, value float64) (bool, error) {
	movement := value - baseline
	magnitude := math.Abs(value) - math.Abs(baseline)

	if event.Unit == PERCENT {
		if baseline == 0 {
			return false, errnie.Err(
				errnie.UnprocessableContent,
				"percentage Prediction observed a zero baseline: "+event.Label,
				nil,
			)
		}

		movement = 100 * movement / math.Abs(baseline)
		magnitude = 100 * magnitude / math.Abs(baseline)
	}

	switch event.Move {
	case INCREASE:
		return movement > event.Value || event.Value > 0 && movement == event.Value, nil
	case DECREASE:
		return movement < -event.Value || event.Value > 0 && movement == -event.Value, nil
	case STAGNATE:
		return math.Abs(movement) <= event.Value, nil
	case EXPAND:
		return magnitude > event.Value || event.Value > 0 && magnitude == event.Value, nil
	case DISSOLVE:
		return magnitude < -event.Value || event.Value > 0 && magnitude == -event.Value, nil
	default:
		return false, errnie.Err(
			errnie.Validation,
			"Prediction requires a declared move: "+event.Label,
			nil,
		)
	}
}

/* observe reads this event's declared source from one market envelope. */
func (event *Falsifiable) observe(envelope *types.Envelope) (float64, bool, error) {
	if event == nil {
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction event is nil",
			nil,
		)
	}

	switch event.Type {
	case METRIC:
		return event.metricValue(envelope)
	case TICKERS:
		return event.tickerValue(envelope)
	case TRADES:
		return event.tradeValue(envelope)
	case LEVEL3:
		return event.level3Value(envelope)
	default:
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction has an unknown observation type: "+event.Label,
			nil,
		)
	}
}

func (event *Falsifiable) metricValue(
	envelope *types.Envelope,
) (float64, bool, error) {
	source, label, qualified := strings.Cut(event.Label, "/")

	if !qualified || source == "" || label == "" {
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction metric must be source-qualified: "+event.Label,
			nil,
		)
	}

	for _, measurement := range envelope.SignalMeasurements() {
		if measurement == nil || measurement.Source != source {
			continue
		}

		metric, found := measurement.Metrics[label]

		if found {
			return metric.Raw, true, nil
		}
	}

	return 0, false, nil
}

func (event *Falsifiable) tickerValue(
	envelope *types.Envelope,
) (float64, bool, error) {
	if envelope.TypeID != types.EnvelopeTicker {
		return 0, false, nil
	}

	ticker := envelope.TickerData

	switch event.Label {
	case "bid":
		if ticker.Bid == nil {
			return 0, false, nil
		}

		return ticker.Bid.Float64(), true, nil
	case "bid_qty":
		return ticker.BidQty, true, nil
	case "ask":
		if ticker.Ask == nil {
			return 0, false, nil
		}

		return ticker.Ask.Float64(), true, nil
	case "ask_qty":
		return ticker.AskQty, true, nil
	case "last":
		if ticker.Last == nil {
			return 0, false, nil
		}

		return ticker.Last.Float64(), true, nil
	case "volume":
		return ticker.Volume, true, nil
	case "vwap":
		return ticker.Vwap, true, nil
	case "low":
		if ticker.Low == nil {
			return 0, false, nil
		}

		return ticker.Low.Float64(), true, nil
	case "high":
		if ticker.High == nil {
			return 0, false, nil
		}

		return ticker.High.Float64(), true, nil
	case "change":
		if ticker.Change == nil {
			return 0, false, nil
		}

		return ticker.Change.Float64(), true, nil
	case "change_pct":
		return ticker.ChangePct, true, nil
	case "trades":
		if ticker.Trades == nil {
			return 0, false, nil
		}

		return float64(*ticker.Trades), true, nil
	default:
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction names unknown ticker observation: "+event.Label,
			nil,
		)
	}
}

func (event *Falsifiable) tradeValue(
	envelope *types.Envelope,
) (float64, bool, error) {
	if envelope.TypeID != types.EnvelopeTrade {
		return 0, false, nil
	}

	switch event.Label {
	case "price":
		return envelope.TradeData.Price.Float64(), true, nil
	case "qty":
		return envelope.TradeData.Qty, true, nil
	case "trade_id":
		return float64(envelope.TradeData.TradeID), true, nil
	default:
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction names unknown trade observation: "+event.Label,
			nil,
		)
	}
}

func (event *Falsifiable) level3Value(
	envelope *types.Envelope,
) (float64, bool, error) {
	if envelope.TypeID != types.EnvelopeLevel3 {
		return 0, false, nil
	}

	switch event.Label {
	case "bids":
		return float64(len(envelope.Level3Data.Bids)), true, nil
	case "asks":
		return float64(len(envelope.Level3Data.Asks)), true, nil
	case "checksum":
		return float64(envelope.Level3Data.Checksum), true, nil
	default:
		return 0, false, errnie.Err(
			errnie.Validation,
			"Prediction names unknown Level3 observation: "+event.Label,
			nil,
		)
	}
}
