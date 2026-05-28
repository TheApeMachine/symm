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
bucketKey identifies one perspective accumulator. Measurements belong to the
same Perspective only when they share a symbol and a perspective lens
(microstructure / flow / cross-asset / sentiment); otherwise they live in
different buckets and produce independent predictions.
*/
type bucketKey struct {
	symbol string
	ptype  engine.PerspectiveType
}

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
	perspectives map[bucketKey]*Perspective
	predictions  []*engine.Prediction
	kellySizer   *KellySizer
	risk         *riskAccount
	gaugeAvg     *confidenceAverages
	calibrator   *sourceCalibrator
	pumpPeak     map[string]float64
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
		perspectives: make(map[bucketKey]*Perspective),
		predictions:  make([]*engine.Prediction, 0),
		kellySizer:   NewKellySizer(engine.DefaultCalibrationParams()),
		risk:         newRiskAccount(tradingWallet),
		gaugeAvg:     newConfidenceAverages(),
		calibrator:   newSourceCalibrator(),
		pumpPeak:     make(map[string]float64),
	}

	for _, channel := range []string{"measurements", "feedback", "ui"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe("crypto:"+channel, 128)
	}

	crypto.subscribers["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond).
		Subscribe("crypto:exits", 128)

	// executions carries live fills emitted by the private WS client. The
	// trader is the only system that owns the wallet, so the executions
	// consumer applies each fill through wallet.ApplyFill (which dedupes
	// on ExecKey). Paper fills, which already mutate the wallet inline in
	// Buy.FillPaper / Sell.FillPaper, will land here too as informational
	// frames and be a no-op because the ExecKey ring will report them as
	// already-seen.
	crypto.subscribers["executions"] = pool.CreateBroadcastGroup("executions", 10*time.Millisecond).
		Subscribe("crypto:executions", 128)
	crypto.subscribers["order_acks"] = pool.CreateBroadcastGroup("order_acks", 10*time.Millisecond).
		Subscribe("crypto:order_acks", 128)

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
		// run_stats is the offline-analysis seam. Every 10 seconds the
		// trader dumps a cumulative counter snapshot plus the live wallet
		// and risk numbers, so a post-run jq can compute per-window
		// throughput, gate hit rates, slot decisions, and PnL trajectory
		// without having to reconstruct counts from per-event lines.
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-crypto.ctx.Done():
				return
			case <-ticker.C:
				crypto.emitRunStats()
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

				// Top-down feedback loop, step 2: every signal source
				// that contributed to this prediction gets its
				// calibrator updated with the predicted-vs-actual
				// return. feedback.Sources is the multi-source list
				// from perspectiveSources; if it's empty (single-
				// source path), fall back to feedback.Source. The next
				// raw measurement from each of those sources will be
				// multiplied by the updated trust factor at intake.
				sources := feedback.Sources

				if len(sources) == 0 && feedback.Source != "" {
					sources = []string{feedback.Source}
				}

				for _, source := range sources {
					crypto.calibrator.ApplyFeedback(
						source, feedback.PredictedReturn, feedback.ActualReturn,
					)
				}

				audit("prediction_settled", map[string]any{
					"source":           feedback.Source,
					"sources":          sources,
					"symbol":           feedback.Symbol,
					"predicted_return": feedback.PredictedReturn,
					"actual_return":    feedback.ActualReturn,
					"error":            feedback.Error,
					"confidence":       feedback.Confidence,
					"regime":           feedback.Regime,
					"trust":            crypto.calibratorTrust(feedback.Source),
				})
			case raw, ok := <-crypto.subscribers["executions"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto executions channel closed"))
					return
				}

				crypto.applyFill(raw.Value)
			case raw, ok := <-crypto.subscribers["order_acks"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto order_acks channel closed"))
					return
				}

				crypto.handleOrderAck(raw.Value)
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

	if len(measurement.Pairs) == 0 || measurement.Pairs[0].Wsname == "" {
		return nil
	}

	symbol := measurement.Pairs[0].Wsname
	crypto.risk.ObserveMark(symbol, measurement.AnchorPrice(), time.Time{})

	// Track the running peak price during a fast pump so the entry anti-chase
	// guard (§15.5) can measure retrace from the high. Accessed only from this
	// single measurements/exits goroutine, so the plain map needs no lock.
	if pumpRegimeOf(measurement) == "pump_fast" {
		if price := measurement.AnchorPrice(); price > crypto.pumpPeak[symbol] {
			crypto.pumpPeak[symbol] = price
		}
	}

	// Top-down feedback loop, step 1: apply the per-source calibrator's
	// trust score to the raw confidence. The signal's own measurement is
	// honest about its current strength; the calibrator's job is to tell
	// the trader "this source has been accurate / inaccurate lately" so
	// the perspective layer, the gauge, and any downstream consumer all
	// see a track-record-adjusted number. Step 2 happens in
	// applyPredictionFeedback below, which feeds the prediction error
	// back into the calibrator so the next measurement from this source
	// is weighted by its post-error trust.
	rawConfidence := measurement.Confidence
	measurement.Confidence = crypto.calibrator.CalibrateConfidence(
		measurement.Source, rawConfidence,
	)

	audit("measurement_ingest", map[string]any{
		"source":         measurement.Source,
		"symbol":         symbol,
		"confidence":     measurement.Confidence,
		"raw_confidence": rawConfidence,
		"regime":         measurement.Regime,
		"reason":         measurement.Reason,
		"type":           measurement.Type,
		"last":           measurement.Last,
		"bid":            measurement.Bid,
		"ask":            measurement.Ask,
	})

	// The gauge shows the running EMA of CALIBRATED confidence per
	// source, not raw per-measurement reading. EMA smooths anomalies
	// and the calibration multiplier is what makes the gauge
	// self-tuning to feedback.
	smoothed := crypto.gaugeAvg.Observe(measurement.Source, measurement.Confidence)

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":          "confidence",
		"source":         measurement.Source,
		"confidence":     smoothed,
		"raw_confidence": rawConfidence,
		"trust":          crypto.calibratorTrust(measurement.Source),
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
	}})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event": "tick",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}})

	crypto.settlePredictions()

	key := bucketKey{symbol: symbol, ptype: perspectiveType(measurement)}
	bucket := crypto.perspectives[key]

	if bucket == nil {
		bucket = NewPerspective([]engine.Measurement{measurement})
		crypto.perspectives[key] = bucket
	} else {
		bucket.AddMeasurement(measurement)
	}

	audit("perspective_accumulate", map[string]any{
		"symbol":                 symbol,
		"perspective_type":       key.ptype,
		"measurement_count":      len(bucket.measurements),
		"source":                 measurement.Source,
		"measurement_confidence": measurement.Confidence,
	})

	return crypto.tryPerspective(key, bucket)
}

/*
calibratorTrust exposes the per-source trust value to callers that need
to surface it in run_stats / UI events. Safe under concurrent access.
*/
func (crypto *Crypto) calibratorTrust(source string) float64 {
	if crypto == nil || crypto.calibrator == nil {
		return 0
	}

	entry := crypto.calibrator.entry(source)

	if entry == nil {
		return 0
	}

	return entry.forecast.Trust()
}

func (crypto *Crypto) tryPerspective(key bucketKey, perspective *Perspective) error {
	prediction, err := perspective.Predict(key.ptype)

	if err != nil {
		audit("perspective_not_ready", map[string]any{
			"symbol":            key.symbol,
			"perspective_type":  key.ptype,
			"measurement_count": len(perspective.measurements),
			"error":             err.Error(),
		})

		return nil
	}

	return crypto.actOnPrediction(prediction)
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
