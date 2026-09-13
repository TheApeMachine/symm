package leadlag

import (
	"errors"
	"fmt"
	"iter"
	"sort"
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

			metric, traded := m.Metrics["last"]

			m.Metadata = map[string]float64{data.MetadataSupport: 0}

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

			m.Metrics["last_price"] = m.Metrics["last_price"].Write(last)

			if last == 0 {
				m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
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

			last := m.Metrics["last_price"].Raw

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

			m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(focal.Count)

			if !focal.Accepted {
				m.Provenance = map[string]string{"event_time_state": "regressed"}

				if !yield(arriving) {
					return
				}

				continue
			}

			m.From = time.Unix(0, focal.From)
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
				pair := drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](op.search, &input)

				if err := op.search.Error(); err != nil {
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
			significance := drive[nmcorrelation.FisherSample, nmcorrelation.FisherReading](op.fisher, &sample)

			if err := op.fisher.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Provenance = map[string]string{
				"peer":                       selection,
				"pair_diagnostics_selection": "last_defined_peer_lexicographic",
			}

			resolution := selectedPair.Spacing * 1e-9

			m.Metrics["contemporaneous_correlation"] = m.Metrics["contemporaneous_correlation"].Write(selectedPair.Contemporaneous)
			m.Metrics["best_lag_correlation"] = m.Metrics["best_lag_correlation"].Write(selectedPair.Correlation)
			m.Metrics["absolute_correlation_gain"] = m.Metrics["absolute_correlation_gain"].Write(selectedPair.AbsoluteGain)
			m.Metrics["lag_fraction"] = m.Metrics["lag_fraction"].Write(selectedPair.LagFraction)
			m.Metrics["best_lag_index"] = m.Metrics["best_lag_index"].Write(selectedPair.LagIndex)
			m.Metrics["reference_return_count"] = m.Metrics["reference_return_count"].Write(selectedPair.Observations)
			m.Metrics["measured_return_count"] = m.Metrics["measured_return_count"].Write(selectedPair.Observations)
			m.Metrics["overlap_pair_count"] = m.Metrics["overlap_pair_count"].Write(selectedPair.Support)
			m.Metrics["effective_sample_count"] = m.Metrics["effective_sample_count"].Write(selectedPair.Support)
			m.Metrics["search_count"] = m.Metrics["search_count"].Write(selectedPair.SearchCount)
			m.Metrics["best_lag_seconds"] = m.Metrics["best_lag_seconds"].Write(selectedPair.X)
			m.Metrics["lag_search_resolution_seconds"] = m.Metrics["lag_search_resolution_seconds"].Write(resolution)
			m.Metrics["lag_search_span"] = m.Metrics["lag_search_span"].Write(selectedPair.Span * resolution)

			if selectedPair.ShapeDefined {
				m.Metrics["lag_peak_prominence"] = m.Metrics["lag_peak_prominence"].Write(selectedPair.Prominence)
				m.Metrics["lag_peak_curvature"] = m.Metrics["lag_peak_curvature"].Write(selectedPair.Curvature)
			}

			if significance.Defined {
				m.Metrics["correlation_p_value"] = m.Metrics["correlation_p_value"].Write(significance.PValue)
				m.Metrics["search_adjusted_p_value"] = m.Metrics["search_adjusted_p_value"].Write(significance.SearchAdjustedPValue)
			}

			m.Metrics["lag_baseline_seconds"] = m.Metrics["lag_baseline_seconds"].Write(selected.lag.Baseline)
			m.Metrics["lag_divergence_seconds"] = m.Metrics["lag_divergence_seconds"].Write(selected.lag.Residual)
			m.Metrics["lag_zscore"] = m.Metrics["lag_zscore"].Write(selected.lag.ZScore)

			if selected.lag.VarianceDefined {
				m.Metrics["lag_noise_scale_seconds"] = m.Metrics["lag_noise_scale_seconds"].Write(selected.lag.Dispersion)
			}

			if selected.lagVel.Defined {
				m.Metrics["lag_velocity"] = m.Metrics["lag_velocity"].Write(selected.lagVel.Rate)
			}

			m.Metrics["correlation_gain_baseline"] = m.Metrics["correlation_gain_baseline"].Write(selected.gain.Baseline)
			m.Metrics["correlation_gain_zscore"] = m.Metrics["correlation_gain_zscore"].Write(selected.gain.ZScore)

			if selected.gainVel.Defined {
				m.Metrics["correlation_gain_velocity"] = m.Metrics["correlation_gain_velocity"].Write(selected.gainVel.Rate)
			}

			m.Metrics["best_lag_correlation_baseline"] = m.Metrics["best_lag_correlation_baseline"].Write(selected.corr.Baseline)
			m.Metrics["best_lag_correlation_zscore"] = m.Metrics["best_lag_correlation_zscore"].Write(selected.corr.ZScore)

			m.Metadata[data.MetadataSupport] = selected.corr.Count

			if selected.corr.HasPrior {
				m.Metadata[data.MetadataDivergence] = selected.corr.Residual
			}

			if selected.corr.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = selected.corr.Variance
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
