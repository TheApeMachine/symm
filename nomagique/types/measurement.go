package types

import (
	"fmt"
	"time"
)

/*
Measurement is one source×symbol observation: shared provenance plus a Metrics
map keyed like the UI wire (metric, or metric:side). It is the single boundary
shape that signals project into and solvers consume.

Provenance contract (set correctly at emit; never rewritten downstream):
  - At is the as-of / emit instant and is required.
  - ObservedFrom, when set, is the start of the observation window and must
    not be after At. Horizon is At−ObservedFrom when both ends are known.
  - Bivariate rows set PeerAt and PeerObservedFrom for the peer observation;
    these timestamps are not inferred from the local interval downstream.
*/
type Measurement struct {
	ID               string                      `json:"id"`
	Source           string                      `json:"source"`
	Symbol           string                      `json:"symbol"`
	Tick             int64                       `json:"tick,omitempty"`
	Peer             string                      `json:"peer,omitempty"`
	At               time.Time                   `json:"at"`
	ObservedFrom     time.Time                   `json:"observedFrom,omitempty"`
	Horizon          time.Duration               `json:"horizon,omitempty"`
	PeerAt           time.Time                   `json:"peerAt,omitempty"`
	PeerObservedFrom time.Time                   `json:"peerObservedFrom,omitempty"`
	Maturity         float64                     `json:"maturity"`
	SNR              float64                     `json:"snr"`
	// SNRDefined distinguishes a measured SNR (including a genuine zero
	// departure) from an undefined SNR where no noise model was estimable.
	// Undefined is not zero.
	SNRDefined bool                         `json:"snrDefined"`
	Metrics    map[string]*Metric[float64]  `json:"metrics,omitempty"`
	Metadata   map[string]float64           `json:"metadata,omitempty"`
	Err        error                        `json:"-"`
}

func NewMeasurement(id string, source string, at int64, from int64) *Measurement {
	return &Measurement{
		ID:      id,
		Source:  source,
		At:      time.Unix(0, at),
		Metrics: make(map[string]*Metric[float64]),
	}
}

func (measurement *Measurement) Put(name string, metric *Metric[float64]) {
	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]*Metric[float64])
	}

	measurement.Metrics[name] = metric
}

/*
AddMetrics appends the given boundary metrics in one expression, indexing each
one by the name the metric carries. The projection from a numeric Frame to a
Measurement therefore stays a single chained statement at the hot-path exit.
*/
func (measurement *Measurement) AddMetrics(metrics ...*Metric[float64]) *Measurement {
	if measurement == nil {
		return nil
	}

	for _, metric := range metrics {
		if metric == nil || metric.Name == "" {
			continue
		}

		measurement.Put(metric.Name, metric)
	}

	return measurement
}

/*
StampQuality writes the two universal quality markers every measurement must
carry, however young its estimator is:

  - hypothesis_separation: the normalized margin between the signal's competing
    hypotheses. A fresh estimator that has not yet observed a competitor
    honestly reports zero — no evidence separates nothing — and rises as real
    separation appears. It is the number downstream systems read to decide how
    much (or how little) to trust the current reading.
  - Maturity: support/(support+1), the same self-scaling measure used across
    the nomagique pipeline. It starts at zero for the first observation and
    approaches one as evidence accumulates, so a young, low-quality estimate is
    still emitted and still clearly marked as young.

There is no readiness threshold and no warm-up: signals emit on the first
sample through this boundary, with baseline-and-maturity carrying the honesty
signal instead of a silent skip.
*/
func (measurement *Measurement) StampQuality(separation float64, support float64) {
	if measurement == nil {
		return
	}

	if support < 0 {
		support = 0
	}

	normalized := separation
	descriptor := Descriptor{
		Unit:      UnitDimensionless,
		Timescale: TimescaleInstantaneous,
	}

	measurement.Put("hypothesis_separation", NewNormalizedMetric(
		"hypothesis_separation", separation, normalized, descriptor,
	))
	measurement.Maturity = support / (support + 1)
}

func (measurement *Measurement) Metric(name string) *Metric[float64] {
	metric, found := measurement.Metrics[name]

	if !found {
		measurement.Err = fmt.Errorf("measurement: metric %q is missing", name)
	}

	return metric
}

/*
Interval returns the observation window: ObservedFrom→At when ObservedFrom is
set, otherwise the point [At, At].
*/
func (measurement Measurement) Interval() (time.Time, time.Time) {
	if measurement.At.IsZero() && measurement.ObservedFrom.IsZero() {
		return time.Time{}, time.Time{}
	}

	through := measurement.At
	from := measurement.ObservedFrom

	if from.IsZero() {
		from = through
	}

	return from, through
}

func (measurement *Measurement) Error() string {
	if measurement.Err == nil {
		return ""
	}

	return measurement.Err.Error()
}
