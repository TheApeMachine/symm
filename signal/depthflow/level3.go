package depthflow

import (
	"fmt"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	symbolObservedBid       = nmtypes.MustIntern("depthflow/observed_notional_bid")
	symbolObservedAsk       = nmtypes.MustIntern("depthflow/observed_notional_ask")
	symbolObservedTotal     = nmtypes.MustIntern("depthflow/observed_notional")
	symbolObservedDiff      = nmtypes.MustIntern("depthflow/observed_notional_diff")
	symbolObservedImbalance = nmtypes.MustIntern("depthflow/observed_notional_imbalance")
	symbolAddBid            = nmtypes.MustIntern("depthflow/add_notional_bid")
	symbolAddAsk            = nmtypes.MustIntern("depthflow/add_notional_ask")
	symbolModifyBid         = nmtypes.MustIntern("depthflow/modify_remaining_notional_bid")
	symbolModifyAsk         = nmtypes.MustIntern("depthflow/modify_remaining_notional_ask")
	symbolDeleteBid         = nmtypes.MustIntern("depthflow/delete_count_bid")
	symbolDeleteAsk         = nmtypes.MustIntern("depthflow/delete_count_ask")
	symbolMutationBid       = nmtypes.MustIntern("depthflow/mutation_count_bid")
	symbolMutationAsk       = nmtypes.MustIntern("depthflow/mutation_count_ask")
	symbolMutationTotal     = nmtypes.MustIntern("depthflow/mutation_count")
	symbolMutationDiff      = nmtypes.MustIntern("depthflow/mutation_count_diff")
	symbolActivityImbalance = nmtypes.MustIntern("depthflow/mutation_activity_imbalance")
	symbolObservedRate      = nmtypes.MustIntern("depthflow/observed_notional_rate")
	symbolDivergence        = nmtypes.MustIntern("divergence")
	symbolNoiseVariance     = nmtypes.MustIntern("noise_variance")
)

/*
estimatorSlots names one causal estimator's retained functional state.
*/
type estimatorSlots struct {
	prefix     string
	series     temporal.Series
	baseline   nmtypes.Symbol
	residual   nmtypes.Symbol
	dispersion nmtypes.Symbol
	zscore     nmtypes.Symbol
	ready      nmtypes.Symbol
}

func newEstimatorSlots(prefix string) estimatorSlots {
	return estimatorSlots{
		prefix:     prefix,
		series:     temporal.NewSeries(prefix),
		baseline:   nmtypes.MustIntern(temporal.JoinPrefix(prefix, "baseline/value")),
		residual:   nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/residual")),
		dispersion: nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/dispersion")),
		zscore:     nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/value")),
		ready:      nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/ready")),
	}
}

func estimator(slots estimatorSlots, source nmtypes.Symbol) nmtypes.Primitive {
	zscore := statistic.ZScore(slots.prefix)
	baseline := statistic.Baseline(slots.prefix)
	window := temporal.Window(slots.prefix)

	return func(input *nmtypes.Frame) {
		value, found := input.Get(source)

		if !found {
			input.Err = fmt.Errorf("depthflow: estimator source is absent")

			return
		}

		sec, _ := input.Get(nmtypes.EventTimeSec)
		nsec, _ := input.Get(nmtypes.EventTimeNsec)
		input.Put(slots.series.ValueSymbol, value)
		input.Put(slots.series.SecSymbol, sec)
		input.Put(slots.series.NsecSymbol, nsec)
		zscore(input)

		if input.Err != nil {
			return
		}

		ready, _ := input.Get(slots.series.ReadySymbol)
		input.Put(slots.ready, ready)
		baseline(input)

		if input.Err != nil {
			return
		}

		window(input)
	}
}

var (
	imbalanceSlots = newEstimatorSlots("observed_notional_imbalance")
	rateSlots      = newEstimatorSlots("observed_notional_rate")
)

/*
Level3 derives event-local depth-flow observations and retains only the causal
numeric state needed to compare the next observation with prior observations.
It never stores order identities, price levels, snapshots, or a generalized
book representation.
*/
type Level3 struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

func depthFlowPipeline(
	imbalanceEstimator nmtypes.Primitive,
	rateEstimator nmtypes.Primitive,
) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		input.Delete(symbolObservedImbalance)
		input.Delete(symbolObservedRate)
		input.Delete(imbalanceSlots.residual)
		input.Delete(imbalanceSlots.zscore)
		input.Delete(rateSlots.residual)
		input.Delete(rateSlots.zscore)
		input.Delete(symbolDivergence)
		input.Delete(symbolNoiseVariance)

		observedBid, _ := input.Get(symbolObservedBid)
		observedAsk, _ := input.Get(symbolObservedAsk)
		observedTotal := observedBid + observedAsk
		input.Put(symbolObservedTotal, observedTotal)
		input.Put(symbolObservedDiff, observedBid-observedAsk)

		if observedTotal > 0 {
			input.Put(symbolObservedImbalance, (observedBid-observedAsk)/observedTotal)
			imbalanceEstimator(input)

			if input.Err != nil {
				return
			}

			support, _ := input.Get(imbalanceSlots.series.CountSymbol)
			input.Put(nmtypes.SampleCount, support)
			ready, _ := input.Get(imbalanceSlots.ready)

			if ready != 0 {
				residual, _ := input.Get(imbalanceSlots.residual)
				dispersion, _ := input.Get(imbalanceSlots.dispersion)
				input.Put(symbolDivergence, residual)
				input.Put(symbolNoiseVariance, dispersion*dispersion)
			}
		}

		mutationBid, _ := input.Get(symbolMutationBid)
		mutationAsk, _ := input.Get(symbolMutationAsk)
		mutationTotal := mutationBid + mutationAsk
		input.Put(symbolMutationTotal, mutationTotal)
		input.Put(symbolMutationDiff, mutationBid-mutationAsk)

		if mutationTotal > 0 {
			input.Put(symbolActivityImbalance, (mutationBid-mutationAsk)/mutationTotal)
		}

		currentSec, _ := input.Get(temporal.SymbolCurrentSec)
		currentNsec, _ := input.Get(temporal.SymbolCurrentNsec)
		previousSec, _ := input.Get(temporal.SymbolPreviousSec)
		previousNsec, _ := input.Get(temporal.SymbolPreviousNsec)
		elapsed := currentSec - previousSec + (currentNsec-previousNsec)/1e9

		if elapsed < 0 {
			input.Err = fmt.Errorf("depthflow: event time must not regress")

			return
		}

		input.Put(temporal.SymbolDelta, elapsed)

		if elapsed == 0 {
			return
		}

		input.Put(symbolObservedRate, observedTotal/elapsed)
		rateEstimator(input)
	}
}

/*
NewLevel3 constructs the streaming Level-3 depth-flow measurement.
*/
func NewLevel3() *Level3 {
	imbalanceEstimator := estimator(imbalanceSlots, symbolObservedImbalance)
	rateEstimator := estimator(rateSlots, symbolObservedRate)

	return &Level3{
		number: nomagique.NewNumber[string](depthFlowPipeline(
			imbalanceEstimator, rateEstimator,
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolObservedBid, Name: "observed_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolObservedAsk, Name: "observed_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolObservedTotal, Name: "observed_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolObservedImbalance, Name: "observed_notional_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAddBid, Name: "add_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAddAsk, Name: "add_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolModifyBid, Name: "modify_remaining_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolModifyAsk, Name: "modify_remaining_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDeleteBid, Name: "delete_count:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDeleteAsk, Name: "delete_count:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMutationBid, Name: "mutation_count:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMutationAsk, Name: "mutation_count:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolActivityImbalance, Name: "mutation_activity_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolObservedRate, Name: "observed_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: imbalanceSlots.baseline, Name: "observed_notional_imbalance_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: imbalanceSlots.residual, Name: "observed_notional_imbalance_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: imbalanceSlots.zscore, Name: "observed_notional_imbalance_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: rateSlots.baseline, Name: "observed_notional_rate_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: rateSlots.residual, Name: "observed_notional_rate_divergence", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: rateSlots.zscore, Name: "observed_notional_rate_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
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

	input := nmtypes.Frame{}
	input.Put(symbolObservedBid, 0)
	input.Put(symbolObservedAsk, 0)
	input.Put(symbolAddBid, 0)
	input.Put(symbolAddAsk, 0)
	input.Put(symbolModifyBid, 0)
	input.Put(symbolModifyAsk, 0)
	input.Put(symbolDeleteBid, 0)
	input.Put(symbolDeleteAsk, 0)
	input.Put(symbolMutationBid, 0)
	input.Put(symbolMutationAsk, 0)

	if err := observeSide(message.Bids, true, &input); err != nil {
		input.Err = err

		return level3.projector.Project(
			message.Symbol, "depthflow", message.Timestamp, message.Timestamp, input,
		)
	}

	if err := observeSide(message.Asks, false, &input); err != nil {
		input.Err = err

		return level3.projector.Project(
			message.Symbol, "depthflow", message.Timestamp, message.Timestamp, input,
		)
	}

	at := message.Timestamp
	previousSec := float64(at.Unix())
	previousNsec := float64(at.Nanosecond())

	if committed, found := level3.number.Project(message.Symbol); found {
		previousSec, _ = committed.Get(nmtypes.EventTimeSec)
		previousNsec, _ = committed.Get(nmtypes.EventTimeNsec)
	}

	input.Put(nmtypes.EventTimeSec, float64(at.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(at.Nanosecond()))
	input.Put(temporal.SymbolCurrentSec, float64(at.Unix()))
	input.Put(temporal.SymbolCurrentNsec, float64(at.Nanosecond()))
	input.Put(temporal.SymbolPreviousSec, previousSec)
	input.Put(temporal.SymbolPreviousNsec, previousNsec)

	frame := level3.number.Step(message.Symbol, input)

	return level3.projector.Project(message.Symbol, "depthflow", at, at, frame)
}

func observeSide(orders []kraken.Level3Order, bid bool, input *nmtypes.Frame) error {
	observed := 0.0
	added := 0.0
	modified := 0.0
	deleted := 0.0

	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			return fmt.Errorf("depthflow: level3 order requires price and quantity")
		}

		price := order.LimitPrice.Float64()
		quantity := order.OrderQty.Float64()

		if price <= 0 || quantity < 0 {
			return fmt.Errorf("depthflow: level3 order requires positive price and non-negative quantity")
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
			return fmt.Errorf("depthflow: unknown level3 event %q", order.Event)
		}
	}

	if bid {
		input.Put(symbolObservedBid, observed)
		input.Put(symbolAddBid, added)
		input.Put(symbolModifyBid, modified)
		input.Put(symbolDeleteBid, deleted)
		input.Put(symbolMutationBid, float64(len(orders)))

		return nil
	}

	input.Put(symbolObservedAsk, observed)
	input.Put(symbolAddAsk, added)
	input.Put(symbolModifyAsk, modified)
	input.Put(symbolDeleteAsk, deleted)
	input.Put(symbolMutationAsk, float64(len(orders)))

	return nil
}

func (level3 *Level3) Close() error { return nil }
