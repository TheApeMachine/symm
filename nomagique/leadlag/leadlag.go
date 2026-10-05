package leadlag

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"sort"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive pushes one payload pointer through one primitive and returns the
answer the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
Gate classifies the arrival: it reads the last trade price the feed wrote,
validates it. Anything invalid fails the measurement here and never reaches the paths.
*/
type Gate struct {
	err error
}

func NewGate() core.Primitive {
	return &Gate{}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			metric, traded := m.LookupMetric("last")

			if !traded {
				m.Err = fmt.Errorf("%w: leadlag: ticker requires a last price", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			last := metric.Raw

			if last < 0 {
				m.Err = fmt.Errorf("%w: leadlag: non-negative last price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			center := metric.Center
			scale := metric.Scale
			if scale == 0 {
				scale = math.Max(last, 1.0)
			}

			m.SetMetric("last_price", data.NewMetric(
				"last_price",
				data.UnitCurrency,
				data.TimescaleInstantaneous,
				center,
				scale,
			).Write(last))

			if last == 0 {
				m.SetProvenance("last_trade_price_state", "unobserved")
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

const (
	lagHistory = iota
	gainHistory
	correlationHistory
)

/*
pipeline owns one symbol pair's lead-lag history: the causal baselines over
lag, absolute correlation gain, and best-lag correlation, plus the lag and
gain velocities. Its readings feed the pair diagnostics.
*/
type pipeline struct {
	histories  [3]core.Primitive
	velocities [2]core.Primitive
	lag        adaptive.BaselineReading
	gain       adaptive.BaselineReading
	corr       adaptive.BaselineReading
	lagVel     temporal.VelocityReading
	gainVel    temporal.VelocityReading
}

func newPipeline() *pipeline {
	return &pipeline{
		histories: [3]core.Primitive{
			adaptive.NewBaseline(adaptive.NewWindow()),
			adaptive.NewBaseline(adaptive.NewWindow()),
			adaptive.NewBaseline(adaptive.NewWindow()),
		},
		velocities: [2]core.Primitive{temporal.NewVelocity(), temporal.NewVelocity()},
	}
}

/*
observe folds one measured pair into the history baselines and velocities.
*/
func (pair *pipeline) observe(lag, absoluteGain, correlation float64, at int64) {
	values := [3]float64{lag, absoluteGain, correlation}
	readings := [3]adaptive.BaselineReading{}

	for index, value := range values {
		probe := value
		readings[index] = drive[float64, adaptive.BaselineReading](pair.histories[index], &probe)
	}

	pair.lag = readings[lagHistory]
	pair.gain = readings[gainHistory]
	pair.corr = readings[correlationHistory]

	observations := [2]temporal.Observation{
		{Value: values[lagHistory], At: at},
		{Value: values[gainHistory], At: at},
	}

	pair.lagVel = drive[temporal.Observation, temporal.VelocityReading](pair.velocities[lagHistory], &observations[0])
	pair.gainVel = drive[temporal.Observation, temporal.VelocityReading](pair.velocities[gainHistory], &observations[1])
}

/*
Cross owns every symbol's price path and measures the arrival's path against
every retained peer: one lead-lag search per pair, every measured pair's
history folded, and the lexicographically last defined peer selected, so
selection is deterministic. All selected-pair facts are written where they
are computed.
*/
type Cross struct {
	err       error
	paths     map[string]core.Primitive
	window    func() core.Primitive
	retained  map[string]nmcorrelation.PathReading
	search    core.Primitive
	fisher    core.Primitive
	pipelines map[[2]string]*pipeline
}

/*
NewCross composes the pair stage over the supplied lag-search estimator, so
the algo dependency is injected at composition instead of imported here.
*/
func NewCross(estimator core.Primitive) core.Primitive {
	return &Cross{
		paths:     make(map[string]core.Primitive),
		window:    adaptive.NewWindow,
		retained:  make(map[string]nmcorrelation.PathReading),
		search:    nmcorrelation.NewLeadLag(estimator),
		fisher:    nmcorrelation.NewFisher(),
		pipelines: make(map[[2]string]*pipeline),
	}
}

func (op *Cross) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			last := m.GetMetric("last_price").Raw

			if last == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			path := op.paths[m.Label]

			if path == nil {
				path = nmcorrelation.NewPath(op.window())
				op.paths[m.Label] = path
			}

			price := temporal.Price{At: m.At.UnixNano(), Value: last}
			focal := drive[temporal.Price, nmcorrelation.PathReading](path, &price)

			if err := path.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.SetMetric("observation_count", data.NewMetric(
				"observation_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(focal.Count, 1),
			).Write(focal.Count))

			if !focal.Accepted {
				m.SetProvenance("event_time_state", "regressed")

				if !yield(arriving) {
					return
				}

				continue
			}

			from := time.Unix(0, focal.From)
			if !from.After(m.At) {
				m.From = from
			}
			op.retained[m.Label] = focal

			failed := false

			var (
				selected     *pipeline
				selectedPair nmcorrelation.LeadLagReading
				selection    string
			)

			for _, symbol := range peers(op.retained, m.Label) {
				peer := op.retained[symbol]

				input := nmcorrelation.LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				var (
					pair nmcorrelation.LeadLagReading
					err  error
				)

				if fast, ok := op.search.(interface {
					Search(*nmcorrelation.LagProfileInput) (nmcorrelation.LeadLagReading, error)
				}); ok {
					pair, err = fast.Search(&input)
				} else {
					pair = drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](op.search, &input)
					err = op.search.Error()
				}

				if err != nil {
					m.Err = errors.Join(m.Err, err)
					op.Error(err)
					failed = true

					break
				}

				if !pair.Defined {
					continue
				}

				key := [2]string{m.Label, symbol}
				built := op.pipelines[key]

				if built == nil {
					built = newPipeline()
					op.pipelines[key] = built
				}

				built.observe(pair.X, pair.AbsoluteGain, pair.Correlation, m.At.UnixNano())
				selected, selectedPair, selection = built, pair, symbol
			}

			if failed {
				if !yield(arriving) {
					return
				}

				continue
			}

			if selected == nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			sample := nmcorrelation.FisherSample{
				Correlation: selectedPair.Correlation,
				Support:     selectedPair.Support,
				SearchCount: selectedPair.SearchCount,
			}
			var significance nmcorrelation.FisherReading

			if fast, ok := op.fisher.(interface {
				Compute(*nmcorrelation.FisherSample) nmcorrelation.FisherReading
			}); ok {
				significance = fast.Compute(&sample)
			} else {
				significance = drive[nmcorrelation.FisherSample, nmcorrelation.FisherReading](op.fisher, &sample)

				if err := op.fisher.Error(); err != nil {
					m.Err = errors.Join(m.Err, err)
					op.Error(err)

					if !yield(arriving) {
						return
					}

					continue
				}
			}

			m.SetProvenance("peer", selection)
			m.SetProvenance("pair_diagnostics_selection", "last_defined_peer_lexicographic")

			resolution := selectedPair.Spacing * 1e-9
			spanScale := math.Max(selectedPair.Span*resolution, 1e-6)

			m.SetMetric("contemporaneous_correlation", data.NewMetric(
				"contemporaneous_correlation",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selectedPair.Contemporaneous))

			m.SetMetric("best_lag_correlation", data.NewMetric(
				"best_lag_correlation",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selectedPair.Correlation))

			m.SetMetric("absolute_correlation_gain", data.NewMetric(
				"absolute_correlation_gain",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selectedPair.AbsoluteGain))

			m.SetMetric("lag_fraction", data.NewMetric(
				"lag_fraction",
				data.UnitRatio,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selectedPair.LagFraction))

			m.SetMetric("best_lag_index", data.NewMetric(
				"best_lag_index",
				data.UnitCount,
				data.TimescaleInstantaneous,
				0,
				math.Max(float64(selectedPair.SearchCount), 1),
			).Write(selectedPair.LagIndex))

			m.SetMetric("reference_return_count", data.NewMetric(
				"reference_return_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selectedPair.Observations), 1),
			).Write(selectedPair.Observations))

			m.SetMetric("measured_return_count", data.NewMetric(
				"measured_return_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selectedPair.Observations), 1),
			).Write(selectedPair.Observations))

			m.SetMetric("overlap_pair_count", data.NewMetric(
				"overlap_pair_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selectedPair.Support), 1),
			).Write(selectedPair.Support))

			m.SetMetric("effective_sample_count", data.NewMetric(
				"effective_sample_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selectedPair.Support), 1),
			).Write(selectedPair.Support))

			m.SetMetric("search_count", data.NewMetric(
				"search_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selectedPair.SearchCount), 1),
			).Write(selectedPair.SearchCount))

			m.SetMetric("best_lag_seconds", data.NewMetric(
				"best_lag_seconds",
				data.UnitDuration,
				data.TimescaleInstantaneous,
				0,
				spanScale,
			).Write(selectedPair.X))

			m.SetMetric("lag_search_resolution_seconds", data.NewMetric(
				"lag_search_resolution_seconds",
				data.UnitDuration,
				data.TimescaleRollingWindow,
				resolution,
				resolution,
			).Write(resolution))

			m.SetMetric("lag_search_span", data.NewMetric(
				"lag_search_span",
				data.UnitDuration,
				data.TimescaleRollingWindow,
				0,
				spanScale,
			).Write(selectedPair.Span*resolution))

			if selectedPair.ShapeDefined {
				m.SetMetric("lag_peak_prominence", data.NewMetric(
					"lag_peak_prominence",
					data.UnitCorrelation,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(selectedPair.Prominence))

				m.SetMetric("lag_peak_curvature", data.NewMetric(
					"lag_peak_curvature",
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(selectedPair.Curvature))
			}

			if significance.Defined {
				m.SetMetric("correlation_p_value", data.NewMetric(
					"correlation_p_value",
					data.UnitProbability,
					data.TimescaleRollingWindow,
					0.5,
					0.5,
				).Write(significance.PValue))

				m.SetMetric("search_adjusted_p_value", data.NewMetric(
					"search_adjusted_p_value",
					data.UnitProbability,
					data.TimescaleRollingWindow,
					0.5,
					0.5,
				).Write(significance.SearchAdjustedPValue))
			}

			lagNoiseScale := math.Max(selected.lag.Dispersion, 1e-6)

			m.SetMetric("lag_baseline_seconds", data.NewMetric(
				"lag_baseline_seconds",
				data.UnitDuration,
				data.TimescaleRollingWindow,
				0,
				lagNoiseScale,
			).Write(selected.lag.Baseline))

			m.SetMetric("lag_divergence_seconds", data.NewMetric(
				"lag_divergence_seconds",
				data.UnitDuration,
				data.TimescaleInstantaneous,
				0,
				lagNoiseScale,
			).Write(selected.lag.Residual))

			m.SetMetric("lag_zscore", data.NewMetric(
				"lag_zscore",
				data.UnitZScore,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.lag.ZScore))

			if selected.lag.VarianceDefined {
				m.SetMetric("lag_noise_scale_seconds", data.NewMetric(
					"lag_noise_scale_seconds",
					data.UnitDuration,
					data.TimescaleRollingWindow,
					0,
					lagNoiseScale,
				).Write(selected.lag.Dispersion))
			}

			if selected.lagVel.Defined {
				m.SetMetric("lag_velocity", data.NewMetric(
					"lag_velocity",
					data.UnitVelocity,
					data.TimescalePerSecond,
					0,
					math.Max(math.Abs(selected.lagVel.Rate), 1e-6),
				).Write(selected.lagVel.Rate))
			}

			m.SetMetric("correlation_gain_baseline", data.NewMetric(
				"correlation_gain_baseline",
				data.UnitCorrelation,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.gain.Baseline))

			m.SetMetric("correlation_gain_zscore", data.NewMetric(
				"correlation_gain_zscore",
				data.UnitZScore,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.gain.ZScore))

			if selected.gainVel.Defined {
				m.SetMetric("correlation_gain_velocity", data.NewMetric(
					"correlation_gain_velocity",
					data.UnitVelocity,
					data.TimescalePerSecond,
					0,
					math.Max(math.Abs(selected.gainVel.Rate), 1e-6),
				).Write(selected.gainVel.Rate))
			}

			m.SetMetric("best_lag_correlation_baseline", data.NewMetric(
				"best_lag_correlation_baseline",
				data.UnitCorrelation,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.corr.Baseline))

			m.SetMetric("best_lag_correlation_zscore", data.NewMetric(
				"best_lag_correlation_zscore",
				data.UnitZScore,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.corr.ZScore))

			m.EnsureMetadata()

			m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(selected.corr.Count, 'f', -1, 64))

			if selected.corr.HasPrior {
				m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(selected.corr.Residual, 'f', -1, 64))
			}

			if selected.corr.VarianceDefined {
				m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(selected.corr.Variance, 'f', -1, 64))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Cross) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
peers lists the retained symbols other than the focal one in lexicographic
order, so pair selection stays deterministic.
*/
func peers(retained map[string]nmcorrelation.PathReading, focal string) []string {
	symbols := make([]string, 0, len(retained))

	for symbol := range retained {
		if symbol != focal {
			symbols = append(symbols, symbol)
		}
	}

	sort.Strings(symbols)

	return symbols
}
