package leadlag

import (
	"errors"
	"fmt"
	"iter"
	"sort"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
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
validates it, and stamps the measurement's support baseline. Anything invalid
fails the measurement here and never reaches the paths. The metadata baseline
is rewritten on every arrival, so the measurement carries this arrival's
facts, never the prior one's.
*/
type Gate struct {
	err    error
	finite core.Primitive
}

func NewGate() core.Primitive {
	return &Gate{finite: logic.NewFinite()}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			m.EnsureMetadata()

			m.SetMetadata(data.MetadataSupport, "0")

			metric, traded := m.LookupMetric("last")

			if !traded {
				m.Err = fmt.Errorf("%w: leadlag: ticker requires a last price", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			last := metric.Raw

			if holds := drive[float64, bool](op.finite, &last); !holds || last < 0 {
				m.Err = fmt.Errorf("%w: leadlag: finite non-negative last price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.WriteMetric("last_price", last)

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
			m := *(**data.Measurement[float64])(arriving)

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

			m.WriteMetric("observation_count", focal.Count)

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

			m.WriteMetric("contemporaneous_correlation", selectedPair.Contemporaneous)
			m.WriteMetric("best_lag_correlation", selectedPair.Correlation)
			m.WriteMetric("absolute_correlation_gain", selectedPair.AbsoluteGain)
			m.WriteMetric("lag_fraction", selectedPair.LagFraction)
			m.WriteMetric("best_lag_index", selectedPair.LagIndex)
			m.WriteMetric("reference_return_count", selectedPair.Observations)
			m.WriteMetric("measured_return_count", selectedPair.Observations)
			m.WriteMetric("overlap_pair_count", selectedPair.Support)
			m.WriteMetric("effective_sample_count", selectedPair.Support)
			m.WriteMetric("search_count", selectedPair.SearchCount)
			m.WriteMetric("best_lag_seconds", selectedPair.X)
			m.WriteMetric("lag_search_resolution_seconds", resolution)
			m.WriteMetric("lag_search_span", selectedPair.Span * resolution)

			if selectedPair.ShapeDefined {
				m.WriteMetric("lag_peak_prominence", selectedPair.Prominence)
				m.WriteMetric("lag_peak_curvature", selectedPair.Curvature)
			}

			if significance.Defined {
				m.WriteMetric("correlation_p_value", significance.PValue)
				m.WriteMetric("search_adjusted_p_value", significance.SearchAdjustedPValue)
			}

			m.WriteMetric("lag_baseline_seconds", selected.lag.Baseline)
			m.WriteMetric("lag_divergence_seconds", selected.lag.Residual)
			m.WriteMetric("lag_zscore", selected.lag.ZScore)

			if selected.lag.VarianceDefined {
				m.WriteMetric("lag_noise_scale_seconds", selected.lag.Dispersion)
			}

			if selected.lagVel.Defined {
				m.WriteMetric("lag_velocity", selected.lagVel.Rate)
			}

			m.WriteMetric("correlation_gain_baseline", selected.gain.Baseline)
			m.WriteMetric("correlation_gain_zscore", selected.gain.ZScore)

			if selected.gainVel.Defined {
				m.WriteMetric("correlation_gain_velocity", selected.gainVel.Rate)
			}

			m.WriteMetric("best_lag_correlation_baseline", selected.corr.Baseline)
			m.WriteMetric("best_lag_correlation_zscore", selected.corr.ZScore)

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
