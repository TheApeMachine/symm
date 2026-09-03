package depthflow

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	types "github.com/theapemachine/symm/nomagique/types"
)

type symbolState struct {
	imbalanceEstimator equation.CausalResidual
	rateEstimator      equation.CausalResidual
	lastTimestamp      time.Time
	hasTime            bool
}

/*
Level3 derives event-local depth-flow observations and retains only the causal
numeric state needed to compare the next observation with prior observations.
It never stores order identities, price levels, snapshots, or a generalized
book representation.
*/
type Level3 struct {
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewLevel3() *Level3 {
	return &Level3{
		states: make(map[string]*symbolState),
	}
}

/*
Step consumes one Level-3 message once. It projects only facts present in that
message. A modify contributes its reported remaining notional, never a guessed
delta; a delete contributes a count because its removed notional is absent
from the mutation and cannot be recovered without retaining order identity.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if level3 == nil || len(message.Bids)+len(message.Asks) == 0 {
		return nil
	}

	var observedBid, observedAsk float64
	var addBid, addAsk float64
	var modifyBid, modifyAsk float64
	var deleteBid, deleteAsk float64
	mutationBid := float64(len(message.Bids))
	mutationAsk := float64(len(message.Asks))

	var err error
	observedBid, addBid, modifyBid, deleteBid, err = observeSide(message.Bids)
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	observedAsk, addAsk, modifyAsk, deleteAsk, err = observeSide(message.Asks)
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	level3.mu.Lock()
	defer level3.mu.Unlock()

	state, found := level3.states[message.Symbol]
	if !found {
		state = &symbolState{}
		level3.states[message.Symbol] = state
	}

	if state.hasTime && message.Timestamp.Before(state.lastTimestamp) {
		return nil
	}

	id := message.Symbol + ":depthflow:" + message.Timestamp.Format(time.RFC3339Nano)
	measurement := data.NewMeasurement[float64](
		id, message.Symbol, "depthflow", message.Timestamp, message.Timestamp,
	)

	observedTotal := observedBid + observedAsk
	observedDiff := observedBid - observedAsk
	mutationTotal := mutationBid + mutationAsk
	mutationDiff := mutationBid - mutationAsk

	putMetric(measurement, "observed_notional:bid", observedBid, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "observed_notional:ask", observedAsk, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "observed_notional", observedTotal, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "observed_notional_diff", observedDiff, data.UnitRate, data.TimescaleInstantaneous)

	putMetric(measurement, "add_notional:bid", addBid, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "add_notional:ask", addAsk, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "modify_remaining_notional:bid", modifyBid, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "modify_remaining_notional:ask", modifyAsk, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "delete_count:bid", deleteBid, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "delete_count:ask", deleteAsk, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "mutation_count:bid", mutationBid, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "mutation_count:ask", mutationAsk, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "mutation_count", mutationTotal, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "mutation_count_diff", mutationDiff, data.UnitDimensionless, data.TimescaleInstantaneous)

	if mutationTotal > 0 {
		putMetric(measurement, "mutation_activity_imbalance", mutationDiff/mutationTotal, data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	if observedTotal > 0 {
		imbalance := observedDiff / observedTotal
		putMetric(measurement, "observed_notional_imbalance", imbalance, data.UnitDimensionless, data.TimescaleInstantaneous)

		// Causal imbalance estimator
		state.imbalanceEstimator.Step(types.Scalar(imbalance))

		if state.imbalanceEstimator.HasPrior() {
			putMetric(measurement, "observed_notional_imbalance_baseline", float64(state.imbalanceEstimator.Baseline()), data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "observed_notional_imbalance_divergence", float64(state.imbalanceEstimator.Residual()), data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "observed_notional_imbalance_zscore", float64(state.imbalanceEstimator.ZScore()), data.UnitDimensionless, data.TimescaleInstantaneous)

			disp := float64(state.imbalanceEstimator.Dispersion())
			if disp > 0 {
				measurement.SNR = float64(state.imbalanceEstimator.Residual()) * float64(state.imbalanceEstimator.Residual()) / (disp * disp)
				measurement.SNRDefined = true
			}
		}
	}

	// Rate estimator uses elapsed time
	var elapsed float64
	if state.hasTime {
		elapsed = message.Timestamp.Sub(state.lastTimestamp).Seconds()

		if elapsed < 0 {
			return nil
		}

		if elapsed > 0 {
			rate := observedTotal / elapsed
			putMetric(measurement, "observed_notional_rate", rate, data.UnitPerSecond, data.TimescalePerSecond)

			state.rateEstimator.Step(types.Scalar(rate))

			if state.rateEstimator.HasPrior() {
				putMetric(measurement, "observed_notional_rate_baseline", float64(state.rateEstimator.Baseline()), data.UnitPerSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "observed_notional_rate_divergence", float64(state.rateEstimator.Residual()), data.UnitPerSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "observed_notional_rate_zscore", float64(state.rateEstimator.ZScore()), data.UnitDimensionless, data.TimescaleInstantaneous)
			}
		}
	}

	state.lastTimestamp = message.Timestamp
	state.hasTime = true

	measurement.Maturity = float64(state.imbalanceEstimator.Maturity())

	return measurement
}

func observeSide(orders []kraken.Level3Order) (observed, added, modified, deleted float64, err error) {
	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires price and quantity")
		}

		price := order.LimitPrice.Float64()
		quantity := order.OrderQty.Float64()

		if price <= 0 || quantity < 0 {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires positive price and non-negative quantity")
		}

		notional := price * quantity

		switch order.Event {
		case "", "add":
			observed += notional
			added += notional
		case "modify":
			observed += notional
			modified += notional
		case "delete":
			deleted++
		default:
			return 0, 0, 0, 0, fmt.Errorf("depthflow: unknown level3 event %q", order.Event)
		}
	}

	return observed, added, modified, deleted, nil
}

func (level3 *Level3) Close() error { return nil }

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, timescale data.Timescale) {
	m.PutMetric(data.Metric[float64]{
		Label:     label,
		Raw:       raw,
		Unit:      unit,
		Timescale: timescale,
	})
}
