package correlation

import (
	"errors"
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Fold folds the admitted peers into one cohort summary and derives the
focal-to-cohort relative return energy rate. Without admitted peers there is
nothing to fold and the measurement moves through untouched. Every cohort
fact is written where it is computed.
*/
type Fold struct {
	err    error
	cohort core.Primitive
}

func NewFold() core.Primitive {
	return &Fold{cohort: NewCohort()}
}

func (op *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			admitted := make([]Peer, len(m.Peers))

			for index, peer := range m.Peers {
				supportStr, _ := peer.GetMetadata("support")
				support, _ := strconv.ParseFloat(supportStr, 64)
				peerEnergyStr, _ := peer.GetMetadata("peer_energy_rate")
				peerEnergy, _ := strconv.ParseFloat(peerEnergyStr, 64)

				var corr float64
				if metric, ok := peer.LookupMetric("signed_correlation"); ok {
					corr = metric.Raw
				}

				admitted[index] = Peer{
					Correlation: corr,
					Support:     support,
					PeerEnergy:  peerEnergy,
				}
			}

			summary := fold(op.cohort, admitted)

			if err := op.cohort.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if summary.Defined {
				m.SetMetric("cohort_signed_correlation", data.NewMetric(
					"cohort_signed_correlation",
					data.UnitCorrelation,
					data.TimescaleRollingWindow,
					0.0,
					1.0,
				).Write(summary.SignedCorrelation))

				m.SetMetric("cohort_absolute_correlation", data.NewMetric(
					"cohort_absolute_correlation",
					data.UnitCorrelation,
					data.TimescaleRollingWindow,
					0.0,
					1.0,
				).Write(summary.AbsoluteCorrelation))

				m.SetMetric("cohort_effective_peer_count", data.NewMetric(
					"cohort_effective_peer_count",
					data.UnitCount,
					data.TimescaleRollingWindow,
					0.0,
					math.Max(summary.Peers, 1.0),
				).Write(summary.EffectivePeers))
			}
			m.SetMetric("cohort_peer_count", data.NewMetric(
				"cohort_peer_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0.0,
				math.Max(summary.Peers, 1.0),
			).Write(summary.Peers))

			if summary.FisherDefined {
				m.SetMetric("cohort_correlation_dispersion", data.NewMetric(
					"cohort_correlation_dispersion",
					data.UnitVariance,
					data.TimescaleRollingWindow,
					0.0,
					1.0,
				).Write(summary.Dispersion))
			}

			if summary.PeerEnergyRate > 0 {
				m.SetMetric("peer_return_energy_rate", data.NewMetric(
					"peer_return_energy_rate",
					data.UnitRate,
					data.TimescalePerSecond,
					0.0,
					math.Max(summary.PeerEnergyRate, 1e-6),
				).Write(summary.PeerEnergyRate))

				measuredEnergy := m.GetMetric("return_energy_rate:measured").Raw
				relEnergy := measuredEnergy / summary.PeerEnergyRate
				m.SetMetric("relative_return_energy", data.NewMetric(
					"relative_return_energy",
					data.UnitRatio,
					data.TimescalePerSecond,
					1.0,
					1.0,
				).Write(relEnergy))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Fold) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
History feeds the cohort's signed correlation to the Fisher-space causal
estimator, giving the measurement its baseline, divergence, and z-score.
*/
type History struct {
	err        error
	estimators map[string]core.Primitive
}

func NewHistory() core.Primitive {
	return &History{estimators: make(map[string]core.Primitive)}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil || len(m.Peers) == 0 || m.GetMetric("cohort_peer_count").Raw == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			estimator := op.estimators[m.Label]
			if estimator == nil {
				estimator = NewFisherEstimator()
				op.estimators[m.Label] = estimator
			}

			signed := m.GetMetric("cohort_signed_correlation").Raw
			view := drive[float64, FisherView](estimator, &signed)

			var divergenceScale float64 = 1.0
			if view.VarianceDefined && view.Variance > 0 {
				divergenceScale = math.Sqrt(view.Variance)
			}

			m.SetMetric("correlation_baseline", data.NewMetric(
				"correlation_baseline",
				data.UnitCorrelation,
				data.TimescaleRollingWindow,
				0.0,
				1.0,
			).Write(view.Baseline))

			m.SetMetric("correlation_divergence", data.NewMetric(
				"correlation_divergence",
				data.UnitCorrelation,
				data.TimescaleRollingWindow,
				0.0,
				divergenceScale,
			).Write(view.Divergence))

			m.SetMetric("correlation_zscore", data.NewMetric(
				"correlation_zscore",
				data.UnitZScore,
				data.TimescaleRollingWindow,
				0.0,
				1.0,
			).Write(view.ZScore))

			if view.Defined {
				m.EnsureMetadata()

				m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(view.Divergence, 'f', -1, 64))
				m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(view.Count, 'f', -1, 64))

				if view.VarianceDefined {
					m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(view.Variance, 'f', -1, 64))
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *History) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Relative tracks the focal-to-cohort relative return energy rate against its
own adaptive baseline.
*/
type Relative struct {
	err       error
	baselines map[string]core.Primitive
}

func NewRelative() core.Primitive {
	return &Relative{baselines: make(map[string]core.Primitive)}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil || len(m.Peers) == 0 || m.GetMetric("cohort_peer_count").Raw == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			baseline := op.baselines[m.Label]
			if baseline == nil {
				baseline = adaptive.NewBaseline(adaptive.NewWindow())
				op.baselines[m.Label] = baseline
			}

			relative := m.GetMetric("relative_return_energy").Raw
			reading := drive[float64, adaptive.BaselineReading](baseline, &relative)

			energyDispersion := math.Max(reading.Dispersion, 1e-6)
			m.SetMetric("relative_return_energy_baseline", data.NewMetric(
				"relative_return_energy_baseline",
				data.UnitRatio,
				data.TimescaleRollingWindow,
				1.0,
				energyDispersion,
			).Write(reading.Baseline))
			m.SetMetric("relative_return_energy_divergence", data.NewMetric(
				"relative_return_energy_divergence",
				data.UnitRatio,
				data.TimescaleInstantaneous,
				0.0,
				energyDispersion,
			).Write(reading.Residual))
			m.SetMetric("relative_return_energy_zscore", data.NewMetric(
				"relative_return_energy_zscore",
				data.UnitZScore,
				data.TimescaleRollingWindow,
				0.0,
				1.0,
			).Write(reading.ZScore))

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Relative) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
CorrelationVelocity measures how fast the cohort's signed correlation moves.
*/
type CorrelationVelocity struct {
	err        error
	velocities map[string]core.Primitive
}

func NewCorrelationVelocity() core.Primitive {
	return &CorrelationVelocity{velocities: make(map[string]core.Primitive)}
}

func (op *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil || len(m.Peers) == 0 || m.GetMetric("cohort_peer_count").Raw == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			velocity := op.velocities[m.Label]
			if velocity == nil {
				velocity = temporal.NewVelocity()
				op.velocities[m.Label] = velocity
			}

			observation := temporal.Observation{
				Value: m.GetMetric("cohort_signed_correlation").Raw,
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](velocity, &observation)

			if reading.Defined {
				m.SetMetric("correlation_velocity", data.NewMetric(
					"correlation_velocity",
					data.UnitVelocity,
					data.TimescalePerSecond,
					0.0,
					math.Max(math.Abs(reading.Rate), 1e-6),
				).Write(reading.Rate))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *CorrelationVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
EnergyVelocity measures how fast the relative return energy rate moves.
*/
type EnergyVelocity struct {
	err        error
	velocities map[string]core.Primitive
}

func NewEnergyVelocity() core.Primitive {
	return &EnergyVelocity{velocities: make(map[string]core.Primitive)}
}

func (op *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil || len(m.Peers) == 0 || m.GetMetric("cohort_peer_count").Raw == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			velocity := op.velocities[m.Label]
			if velocity == nil {
				velocity = temporal.NewVelocity()
				op.velocities[m.Label] = velocity
			}

			observation := temporal.Observation{
				Value: m.GetMetric("relative_return_energy").Raw,
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](velocity, &observation)

			if reading.Defined {
				m.SetMetric("relative_return_energy_velocity", data.NewMetric(
					"relative_return_energy_velocity",
					data.UnitVelocity,
					data.TimescalePerSecond,
					0.0,
					math.Max(math.Abs(reading.Rate), 1e-6),
				).Write(reading.Rate))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *EnergyVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
fold pushes one peer run through the cohort reduction and returns the summary.
*/
func fold(op core.Primitive, peers []Peer) CohortSummary {
	var summary CohortSummary

	for out := range op.Next(transport.NewValues(peers...).Next(nil)) {
		summary = *(*CohortSummary)(out)
	}

	return summary
}
