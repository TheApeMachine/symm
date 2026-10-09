package correlation

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Fold folds the admitted peers into one cohort summary and derives the
focal-to-cohort relative return energy rate.
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
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			summary := fold(op.cohort, frame.Peers)

			if err := op.cohort.Error(); err != nil {
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if summary.Defined {
				frame.Metrics["cohort_signed_correlation"] = summary.SignedCorrelation
				frame.Metrics["cohort_absolute_correlation"] = summary.AbsoluteCorrelation
				frame.Metrics["cohort_effective_peer_count"] = summary.EffectivePeers
			}
			frame.Metrics["cohort_peer_count"] = summary.Peers

			if summary.FisherDefined {
				frame.Metrics["cohort_correlation_dispersion"] = summary.Dispersion
			}

			if summary.PeerEnergyRate > 0 {
				frame.Metrics["peer_return_energy_rate"] = summary.PeerEnergyRate
				measuredRate := frame.Metrics["return_energy_rate:measured"]
				frame.Metrics["relative_return_energy"] = measuredRate / summary.PeerEnergyRate
				frame.Metrics["relative_cohort_return_energy"] = measuredRate / summary.PeerEnergyRate
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
	err       error
	estimator core.Primitive
}

func NewHistory() core.Primitive {
	return &History{estimator: NewFisherEstimator()}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			signed := frame.Metrics["cohort_signed_correlation"]
			view := data.To[float64, FisherView](op.estimator, &signed)

			frame.Metrics["correlation_baseline"] = view.Baseline
			frame.Metrics["correlation_divergence"] = view.Divergence
			frame.Metrics["correlation_zscore"] = view.ZScore

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
	err      error
	baseline core.Primitive
}

func NewRelative() core.Primitive {
	return &Relative{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			relative := frame.Metrics["relative_return_energy"]
			reading := data.To[float64, adaptive.BaselineReading](op.baseline, &relative)

			frame.Metrics["relative_return_energy_baseline"] = reading.Baseline
			frame.Metrics["relative_return_energy_divergence"] = reading.Residual
			frame.Metrics["relative_return_energy_zscore"] = reading.ZScore

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
	err      error
	velocity core.Primitive
}

func NewCorrelationVelocity() core.Primitive {
	return &CorrelationVelocity{velocity: temporal.NewVelocity()}
}

func (op *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: frame.Metrics["cohort_signed_correlation"],
				At:    frame.At,
			}

			reading := data.To[temporal.Observation, temporal.VelocityReading](
				op.velocity, &observation,
			)

			if reading.Defined {
				frame.Metrics["correlation_velocity"] = reading.Rate
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
	err      error
	velocity core.Primitive
}

func NewEnergyVelocity() core.Primitive {
	return &EnergyVelocity{velocity: temporal.NewVelocity()}
}

func (op *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: frame.Metrics["relative_return_energy"],
				At:    frame.At,
			}

			reading := data.To[temporal.Observation, temporal.VelocityReading](
				op.velocity, &observation,
			)

			if reading.Defined {
				frame.Metrics["relative_return_energy_velocity"] = reading.Rate
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
Recurrence measures standardized (correlation_zscore, relative_return_energy_zscore)
trajectory recurrence against retained trajectory history, emitting historical path
distance and empirical percentile.
*/
type Recurrence struct {
	err              error
	historyPoints    [][2]float64
	historyDistances []float64
}

func NewRecurrence() core.Primitive {
	return &Recurrence{
		historyPoints:    make([][2]float64, 0, 256),
		historyDistances: make([]float64, 0, 256),
	}
}

func (op *Recurrence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			correlationZScore := frame.Metrics["correlation_zscore"]
			energyZScore := frame.Metrics["relative_return_energy_zscore"]
			target := [2]float64{correlationZScore, energyZScore}

			histDist := 0.0
			histPerc := 0.0

			if len(op.historyPoints) > 0 {
				diffX := target[0] - op.historyPoints[0][0]
				diffY := target[1] - op.historyPoints[0][1]
				minDist := math.Sqrt(diffX*diffX + diffY*diffY)

				for index := 1; index < len(op.historyPoints); index++ {
					deltaX := target[0] - op.historyPoints[index][0]
					deltaY := target[1] - op.historyPoints[index][1]
					distance := math.Sqrt(deltaX*deltaX + deltaY*deltaY)

					if distance < minDist {
						minDist = distance
					}
				}

				if len(op.historyDistances) > 0 {
					belowCount := 0

					for _, pastDist := range op.historyDistances {
						if pastDist <= minDist {
							belowCount++
						}
					}

					histPerc = float64(belowCount) / float64(len(op.historyDistances))
				}

				histDist = minDist
				op.historyDistances = append(op.historyDistances, minDist)

				if len(op.historyDistances) > 256 {
					op.historyDistances = op.historyDistances[len(op.historyDistances)-256:]
				}
			}

			op.historyPoints = append(op.historyPoints, target)

			if len(op.historyPoints) > 256 {
				op.historyPoints = op.historyPoints[len(op.historyPoints)-256:]
			}

			frame.Metrics["historical_path_distance"] = histDist
			frame.Metrics["historical_path_percentile"] = histPerc

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Recurrence) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

func fold(op core.Primitive, peers []Peer) CohortSummary {
	var summary CohortSummary

	for out := range op.Next(transport.NewValues(peers...).Next(nil)) {
		summary = *(*CohortSummary)(out)
	}

	return summary
}
