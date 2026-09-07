package correlation

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type pipeline struct {
	pairwise, cohort, progress core.Primitive
	projection                 *data.Projection
}

// newPipeline has one stateless asynchronous pair calculation, one per-run
// peer fold, and one retained cohort history. None replays a stateful producer
// to read a side-channel statistic.
func newPipeline() *pipeline {
	path := func(names ...string) core.Primitive {
		nodes := make([]core.Primitive, len(names))
		for i, name := range names {
			nodes[i] = store.NewGet(name)
		}
		return transport.NewPipe(nodes...)
	}
	field := func(name string, node core.Primitive) core.Primitive {
		return transport.NewPipe(node, store.NewKey(name))
	}
	history := nmcorrelation.NewFisherEstimator(adaptive.NewBaseline(adaptive.NewWindow()))
	return &pipeline{
		pairwise: transport.NewPipe(nmcorrelation.NewDependence(algo.NewHayashiYoshida()), store.NewRecord(transport.NewPipe(), field("fisher", nmcorrelation.NewFisher()))),
		cohort:   transport.NewPipe(transport.NewSpread[core.Primitive](), nmcorrelation.NewCohort(calculus.NewAtanh(transport.NewIO(core.From(0.0))))),
		progress: transport.NewPipe(
			store.NewRecord(transport.NewPipe(), field("relative", logic.NewGate(equation.NewGreater[float64](path("cohort", "peer_energy_rate"), store.NewConstant(core.From(0.0))), equation.NewRatio[float64](path("pair", "left_energy_rate"), path("cohort", "peer_energy_rate")), store.NewConstant(core.From(0.0))))),
			store.NewRecord(transport.NewPipe(),
				field("history", transport.NewPipe(path("cohort", "signed_correlation"), history)),
				field("relative_history", transport.NewPipe(path("relative"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())))),
				field("correlation_velocity", temporal.NewVelocity(path("cohort", "signed_correlation"), path("at"))),
				field("energy_velocity", temporal.NewVelocity(path("relative"), path("at")))),
			store.NewRecord(transport.NewPipe(),
				field("support", logic.NewGate(path("history", "defined"), path("history", "count"), store.NewConstant(core.From(0.0)))),
				field("prior_available", logic.NewGate(path("history", "defined"), path("history", "has_prior"), store.NewConstant(core.From(false)))),
				field("noise_available", logic.NewGate(path("history", "defined"), path("history", "variance_defined"), store.NewConstant(core.From(false))))),
		), projection: correlationProjection(),
	}
}

func correlationProjection() *data.Projection {
	p := &data.Projection{Source: "correlation"}
	add := func(label string, path []string, unit data.Unit, defined ...string) {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: label, Path: path, Defined: defined, Unit: unit, Timescale: data.TimescaleInstantaneous})
	}
	for _, item := range [][2]string{{"signed_correlation", "signed_correlation"}, {"absolute_correlation", "absolute_correlation"}, {"cohort_signed_correlation", "signed_correlation"}, {"cohort_absolute_correlation", "absolute_correlation"}} {
		add(item[0], []string{"cohort", item[1]}, data.UnitDimensionless)
	}
	for _, item := range [][2]string{{"covariance", "covariance"}, {"return_energy:reference", "right_energy"}, {"return_energy:measured", "left_energy"}} {
		add(item[0], []string{"pair", item[1]}, data.UnitNat)
	}
	for _, item := range [][2]string{{"return_energy_rate:reference", "right_energy_rate"}, {"return_energy_rate:measured", "left_energy_rate"}, {"focal_return_energy_rate", "left_energy_rate"}, {"overlap_density", "overlap_density"}} {
		add(item[0], []string{"pair", item[1]}, data.UnitPerSecond)
	}
	add("peer_return_energy_rate", []string{"cohort", "peer_energy_rate"}, data.UnitPerSecond)
	for _, item := range [][2]string{{"supported_return_count:measured", "left_returns"}, {"supported_return_count:reference", "right_returns"}, {"overlap_pair_count", "support"}, {"effective_sample_count", "support"}} {
		add(item[0], []string{"pair", item[1]}, data.UnitCount)
	}
	add("shared_time", []string{"pair", "shared_time"}, data.UnitSecond)
	for _, item := range [][2]string{{"correlation_p_value", "p_value"}, {"correlation_standard_error_fisher", "standard_error"}} {
		add(item[0], []string{"pair", "fisher", item[1]}, data.UnitDimensionless, "pair", "fisher", "defined")
	}
	for _, item := range [][2]string{{"cohort_peer_count", "peers"}, {"cohort_effective_peer_count", "effective_peers"}} {
		add(item[0], []string{"cohort", item[1]}, data.UnitCount)
	}
	add("cohort_correlation_dispersion", []string{"cohort", "dispersion"}, data.UnitNat, "cohort", "fisher_defined")
	for _, name := range []string{"relative_return_energy", "relative_cohort_return_energy"} {
		add(name, []string{"relative"}, data.UnitDimensionless)
	}
	add("correlation_baseline", []string{"history", "baseline"}, data.UnitDimensionless, "history", "defined")
	add("correlation_divergence", []string{"history", "divergence"}, data.UnitNat, "prior_available")
	add("correlation_zscore", []string{"history", "zscore"}, data.UnitDimensionless, "prior_available")
	add("correlation_velocity", []string{"correlation_velocity", "rate"}, data.UnitPerSecond, "correlation_velocity", "defined")
	for _, item := range [][2]string{{"relative_return_energy_baseline", "baseline"}, {"relative_return_energy_divergence", "residual"}, {"relative_return_energy_zscore", "zscore"}} {
		add(item[0], []string{"relative_history", item[1]}, data.UnitDimensionless)
	}
	add("relative_return_energy_velocity", []string{"energy_velocity", "rate"}, data.UnitPerSecond, "energy_velocity", "defined")
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"support"}},
		{Name: data.MetadataDivergence, Path: []string{"history", "divergence"}, Defined: []string{"prior_available"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"history", "variance"}, Defined: []string{"noise_available"}},
	}
	return p
}
