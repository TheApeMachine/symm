package hawkes

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the Hawkes arrival-dynamics instrument. It holds no estimation state
of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's
lifetime. The per-symbol arrival paths and fitted models live inside the
pipeline's shared stage registry.
*/
type Trade struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

/*
NewTrade composes the arrival-dynamics pipeline: the gate classifies the
trade's side, the counts stage admits the arrival into the symbol's
observation window, the excitation stage measures the arrival against the
model fitted before it, and the refit stage folds the arrival into the
history and re-estimates for the next one.
*/
func NewTrade(ctx context.Context) *Trade {
	history := nmhawkes.Paths()

	return &Trade{
		System: runtime.NewSystem(ctx, "hawkes:trade"),
		pipeline: nomagique.NewNumber(
			nmhawkes.NewGate(),
			nmhawkes.NewCounts(history),
			nmhawkes.NewExcitation(history),
			nmhawkes.NewRefit(history),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](trade.pipeline.Next(transport.NewOne(unsafe.Pointer(&m)).Next(nil)))
}

/*
Register returns the pre-allocated measurement every trade flows through:
every metric the instrument can produce is declared, none valued.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	return data.NewMeasurement("hawkes", map[string]data.Metric[float64]{
		"event_count": data.NewMetric[float64](
			"event_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"event_count:buy": data.NewMetric[float64](
			"event_count:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"event_count:sell": data.NewMetric[float64](
			"event_count:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"event_fraction:buy": data.NewMetric[float64](
			"event_fraction:buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"event_fraction:sell": data.NewMetric[float64](
			"event_fraction:sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"arrival_rate:buy": data.NewMetric[float64](
			"arrival_rate:buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"arrival_rate:sell": data.NewMetric[float64](
			"arrival_rate:sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"arrival_rate": data.NewMetric[float64](
			"arrival_rate", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"conditional_intensity:buy": data.NewMetric[float64](
			"conditional_intensity:buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"conditional_intensity:sell": data.NewMetric[float64](
			"conditional_intensity:sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"conditional_intensity": data.NewMetric[float64](
			"conditional_intensity", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"background_rate:buy": data.NewMetric[float64](
			"background_rate:buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"background_rate:sell": data.NewMetric[float64](
			"background_rate:sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"background_rate": data.NewMetric[float64](
			"background_rate", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_intensity:buy": data.NewMetric[float64](
			"excitation_intensity:buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_intensity:sell": data.NewMetric[float64](
			"excitation_intensity:sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_fraction:buy": data.NewMetric[float64](
			"excitation_fraction:buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_fraction:sell": data.NewMetric[float64](
			"excitation_fraction:sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_amplitude:buy_from_buy": data.NewMetric[float64](
			"excitation_amplitude:buy_from_buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_amplitude:buy_from_sell": data.NewMetric[float64](
			"excitation_amplitude:buy_from_sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_amplitude:sell_from_buy": data.NewMetric[float64](
			"excitation_amplitude:sell_from_buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_amplitude:sell_from_sell": data.NewMetric[float64](
			"excitation_amplitude:sell_from_sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_decay": data.NewMetric[float64](
			"excitation_decay", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_decay:buy_from_buy": data.NewMetric[float64](
			"excitation_decay:buy_from_buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_decay:buy_from_sell": data.NewMetric[float64](
			"excitation_decay:buy_from_sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_decay:sell_from_buy": data.NewMetric[float64](
			"excitation_decay:sell_from_buy", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_decay:sell_from_sell": data.NewMetric[float64](
			"excitation_decay:sell_from_sell", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"excitation_timescale": data.NewMetric[float64](
			"excitation_timescale", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_timescale:buy_from_buy": data.NewMetric[float64](
			"excitation_timescale:buy_from_buy", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_timescale:buy_from_sell": data.NewMetric[float64](
			"excitation_timescale:buy_from_sell", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_timescale:sell_from_buy": data.NewMetric[float64](
			"excitation_timescale:sell_from_buy", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_timescale:sell_from_sell": data.NewMetric[float64](
			"excitation_timescale:sell_from_sell", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"offspring:buy_from_buy": data.NewMetric[float64](
			"offspring:buy_from_buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"offspring:buy_from_sell": data.NewMetric[float64](
			"offspring:buy_from_sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"offspring:sell_from_buy": data.NewMetric[float64](
			"offspring:sell_from_buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"offspring:sell_from_sell": data.NewMetric[float64](
			"offspring:sell_from_sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"branching_spectral_radius": data.NewMetric[float64](
			"branching_spectral_radius", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"expected_descendants_from_buy": data.NewMetric[float64](
			"expected_descendants_from_buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"expected_descendants_from_sell": data.NewMetric[float64](
			"expected_descendants_from_sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood:hawkes": data.NewMetric[float64](
			"log_likelihood:hawkes", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood:poisson": data.NewMetric[float64](
			"log_likelihood:poisson", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood:self_only": data.NewMetric[float64](
			"log_likelihood:self_only", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood_per_event:hawkes": data.NewMetric[float64](
			"log_likelihood_per_event:hawkes", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood_gain_vs_poisson": data.NewMetric[float64](
			"log_likelihood_gain_vs_poisson", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood_gain_per_event_vs_poisson": data.NewMetric[float64](
			"log_likelihood_gain_per_event_vs_poisson", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood_gain_vs_self_only": data.NewMetric[float64](
			"log_likelihood_gain_vs_self_only", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"log_likelihood_gain_per_event_vs_self_only": data.NewMetric[float64](
			"log_likelihood_gain_per_event_vs_self_only", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"compensator:buy": data.NewMetric[float64](
			"compensator:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"compensator:sell": data.NewMetric[float64](
			"compensator:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"count_innovation:buy": data.NewMetric[float64](
			"count_innovation:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"count_innovation:sell": data.NewMetric[float64](
			"count_innovation:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"standardized_innovation:buy": data.NewMetric[float64](
			"standardized_innovation:buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"standardized_innovation:sell": data.NewMetric[float64](
			"standardized_innovation:sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_mass:buy": data.NewMetric[float64](
			"excitation_mass:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_mass:sell": data.NewMetric[float64](
			"excitation_mass:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_share:buy": data.NewMetric[float64](
			"excitation_share:buy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_share:sell": data.NewMetric[float64](
			"excitation_share:sell", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"excitation_share": data.NewMetric[float64](
			"excitation_share", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"snr": data.NewMetric[float64](
			"snr", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
