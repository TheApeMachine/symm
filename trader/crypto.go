package trader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/wallet"
)

/*
Crypto combines measurements into perspectives, records predictions, and enters trades.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	wallet       *wallet.Wallet
	forecasts    *price.Prediction
	perspectives []*Perspective
	predictions  []*engine.Prediction
	kellySizer   *KellySizer
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	tradingWallet *wallet.Wallet,
	forecasts *price.Prediction,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		wallet:       tradingWallet,
		forecasts:    forecasts,
		perspectives: make([]*Perspective, 0),
		predictions:  make([]*engine.Prediction, 0),
		kellySizer:   NewKellySizer(engine.DefaultCalibrationParams()),
	}

	for _, channel := range []string{"measurements", "feedback", "ui"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe("crypto:"+channel, 128)
	}

	crypto.subscribers["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond).
		Subscribe("crypto:exits", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":       ctx,
		"cancel":    cancel,
		"pool":      pool,
		"wallet":    tradingWallet,
		"forecasts": forecasts,
	})) != nil {
		return nil
	}

	return crypto
}

func (crypto *Crypto) Start() error {
	crypto.sendWallet()
	return nil
}

func (crypto *Crypto) State() engine.State {
	return engine.READY
}

func (crypto *Crypto) Tick() error {
	errnie.Info("starting crypto tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-crypto.ctx.Done():
				return
			case <-ticker.C:
				crypto.attachWalletMarks()
				crypto.sendWallet()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case raw, ok := <-crypto.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto feedback channel closed"))
					return
				}

				feedback, ok := raw.Value.(engine.PredictionFeedback)

				if !ok {
					errnie.Error(fmt.Errorf("invalid prediction feedback: %v", raw.Value))
					continue
				}

				crypto.kellySizer.ApplyFeedback(feedback)
				audit("prediction_feedback", map[string]any{
					"source":           feedback.Source,
					"symbol":           feedback.Symbol,
					"predicted_return": feedback.PredictedReturn,
					"actual_return":    feedback.ActualReturn,
					"error":            feedback.Error,
					"confidence":       feedback.Confidence,
					"regime":           feedback.Regime,
				})
			case raw, ok := <-crypto.subscribers["exits"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto exits channel closed"))
					return
				}

				exitSignal, ok := raw.Value.(engine.Exit)

				if !ok {
					errnie.Error(fmt.Errorf("invalid exit signal: %v", raw.Value))
					continue
				}

				if err := crypto.handleExit(exitSignal); err != nil {
					errnie.Error(err)
				}
			case raw, ok := <-crypto.subscribers["measurements"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto measurements channel closed"))
					return
				}

				if err := crypto.ingestMeasurement(raw.Value); err != nil {
					errnie.Error(err)
				}
			}
		}
	})

	wg.Wait()
	return nil
}

func (crypto *Crypto) ingestMeasurement(raw any) error {
	measurement, ok := raw.(engine.Measurement)

	if !ok {
		return fmt.Errorf("invalid measurement: %v", raw)
	}

	symbol := ""

	if len(measurement.Pairs) > 0 {
		symbol = measurement.Pairs[0].Wsname
	}

	audit("measurement_ingest", map[string]any{
		"source":     measurement.Source,
		"symbol":     symbol,
		"confidence": measurement.Confidence,
		"regime":     measurement.Regime,
		"reason":     measurement.Reason,
		"type":       measurement.Type,
		"last":       measurement.Last,
		"bid":        measurement.Bid,
		"ask":        measurement.Ask,
	})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":      "confidence",
		"source":     measurement.Source,
		"confidence": measurement.Confidence,
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
	}})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event": "tick",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}})

	crypto.settlePredictions(measurement)

	if len(crypto.perspectives) > 0 {
		lastPerspective := crypto.perspectives[len(crypto.perspectives)-1]

		if !lastPerspective.Ready {
			lastPerspective.AddMeasurement(measurement)
			audit("perspective_accumulate", map[string]any{
				"symbol":                 symbol,
				"measurement_count":      len(lastPerspective.measurements),
				"source":                 measurement.Source,
				"measurement_confidence": measurement.Confidence,
			})

			return crypto.tryPerspective(lastPerspective)
		}
	}

	crypto.perspectives = append(
		crypto.perspectives,
		NewPerspective([]engine.Measurement{measurement}),
	)

	return crypto.tryPerspective(crypto.perspectives[len(crypto.perspectives)-1])
}

func (crypto *Crypto) tryPerspective(perspective *Perspective) error {
	prediction, err := perspective.Predict()

	if err != nil {
		symbol := ""

		if len(perspective.measurements) > 0 && len(perspective.measurements[0].Pairs) > 0 {
			symbol = perspective.measurements[0].Pairs[0].Wsname
		}

		audit("perspective_not_ready", map[string]any{
			"symbol":            symbol,
			"measurement_count": len(perspective.measurements),
			"error":             err.Error(),
		})

		return nil
	}

	return crypto.actOnPrediction(prediction)
}

func (crypto *Crypto) settlePredictions(measurement engine.Measurement) {
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

		if _, ok := due.Error(measurement); !ok {
			remaining = append(remaining, due)
			continue
		}

		lead, _ := due.LeadMeasurement()

		feedback := engine.PredictionFeedback{
			Source:          engine.PerspectiveSource(due.Perspective.Type),
			Symbol:          lead.Pairs[0].Wsname,
			PerspectiveType: due.Perspective.Type,
			Regime:          engine.FeedbackRegime(due.Perspective, lead),
			Reason:          lead.Reason,
			Confidence:      due.Confidence,
			PredictedReturn: due.ExpectedReturn,
			ActualReturn:    due.ActualReturn,
			Error:           due.Err,
			Runway:          due.Runway,
			PredictedAt:     due.PredictedAt,
			DueAt:           due.DueAt,
			SettledAt:       now,
		}

		crypto.kellySizer.ApplyFeedback(feedback)
		crypto.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: feedback})

		audit("prediction_settled", map[string]any{
			"source":           feedback.Source,
			"symbol":           feedback.Symbol,
			"predicted_return": feedback.PredictedReturn,
			"actual_return":    feedback.ActualReturn,
			"error":            feedback.Error,
			"confidence":       feedback.Confidence,
		})

		if crypto.holdsSymbol(crypto.wallet, feedback.Symbol) {
			if err := crypto.handleExit(engine.Exit{
				Symbol:  feedback.Symbol,
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

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
