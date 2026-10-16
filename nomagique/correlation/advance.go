package correlation

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
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
				frame.Metrics["cohort_covariance_score"] = summary.SignedScore
				frame.Metrics["cohort_absolute_covariance_score"] = summary.AbsoluteScore
				frame.Metrics["cohort_effective_peer_count"] = summary.EffectivePeers
			}
			frame.Metrics["cohort_peer_count"] = summary.Peers

			if summary.DispersionDefined {
				frame.Metrics["cohort_covariance_score_dispersion"] = summary.Dispersion
			}

			if summary.PeerEnergyRate > 0 {
				frame.Metrics["peer_return_energy_rate"] = summary.PeerEnergyRate

				// The ratio is undefined when the peer rate is negligible next
				// to the measured one.
				if measuredRate, ok := frame.Metrics["return_energy_rate:measured"]; ok &&
					!core.Negligible(summary.PeerEnergyRate, measuredRate) {
					frame.Metrics["relative_return_energy"] = measuredRate / summary.PeerEnergyRate
				}
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
History scores the cohort's covariance score against its own adaptive
baseline, giving the measurement its baseline, divergence, and z-score. The
score is unbounded, so it is tracked directly rather than through atanh.
*/
type History struct {
	err       error
	baselines map[string]core.Primitive
}

func NewHistory() core.Primitive {
	return &History{baselines: make(map[string]core.Primitive)}
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

			signed, defined := frame.Metrics["cohort_covariance_score"]

			if !defined {
				if !yield(arriving) {
					return
				}

				continue
			}

			bl, exists := op.baselines[frame.Symbol]

			if !exists {
				bl = adaptive.NewBaseline(adaptive.NewWindow())
				op.baselines[frame.Symbol] = bl
			}

			reading := transport.To[float64, adaptive.BaselineReading](bl, &signed)

			if reading.HasPrior {
				frame.Metrics["covariance_score_baseline"] = reading.Baseline
				frame.Metrics["covariance_score_divergence"] = reading.Residual
			}

			if reading.ScoreScale > 0 {
				frame.Metrics["covariance_score_zscore"] = reading.ZScore
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
			frame := (*Frame)(arriving)

			relative, defined := frame.Metrics["relative_return_energy"]

			if !defined {
				if !yield(arriving) {
					return
				}

				continue
			}

			bl, exists := op.baselines[frame.Symbol]

			if !exists {
				bl = adaptive.NewBaseline(adaptive.NewWindow())
				op.baselines[frame.Symbol] = bl
			}

			reading := transport.To[float64, adaptive.BaselineReading](bl, &relative)

			if reading.HasPrior {
				frame.Metrics["relative_return_energy_baseline"] = reading.Baseline
				frame.Metrics["relative_return_energy_divergence"] = reading.Residual
			}

			if reading.ScoreScale > 0 {
				frame.Metrics["relative_return_energy_zscore"] = reading.ZScore
			}

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
CorrelationVelocity measures how fast the cohort's covariance score moves.
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
			frame := (*Frame)(arriving)

			if len(frame.Peers) == 0 || frame.Metrics["cohort_peer_count"] == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			signed, defined := frame.Metrics["cohort_covariance_score"]

			if !defined {
				if !yield(arriving) {
					return
				}

				continue
			}

			vel, exists := op.velocities[frame.Symbol]

			if !exists {
				vel = temporal.NewVelocity()
				op.velocities[frame.Symbol] = vel
			}

			observation := temporal.Observation{
				Value: signed,
				At:    frame.At,
			}

			reading := transport.To[temporal.Observation, temporal.VelocityReading](
				vel, &observation,
			)

			if reading.Defined {
				frame.Metrics["covariance_score_velocity"] = reading.Rate
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
			frame := (*Frame)(arriving)

			relative, defined := frame.Metrics["relative_return_energy"]

			if !defined {
				if !yield(arriving) {
					return
				}

				continue
			}

			vel, exists := op.velocities[frame.Symbol]

			if !exists {
				vel = temporal.NewVelocity()
				op.velocities[frame.Symbol] = vel
			}

			observation := temporal.Observation{
				Value: relative,
				At:    frame.At,
			}

			reading := transport.To[temporal.Observation, temporal.VelocityReading](
				vel, &observation,
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
Recurrence measures standardized (covariance_score_zscore, relative_return_energy_zscore)
trajectory recurrence against retained trajectory history, emitting historical path
distance and empirical percentile.
*/
type Recurrence struct {
	err              error
	historyPoints    map[string][][2]float64
	historyDistances map[string][]float64
}

func NewRecurrence() core.Primitive {
	return &Recurrence{
		historyPoints:    make(map[string][][2]float64),
		historyDistances: make(map[string][]float64),
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

			// The path point needs both z-scores; an undefined one is not 0.
			correlationZScore, hasCorrelationZ := frame.Metrics["covariance_score_zscore"]
			energyZScore, hasEnergyZ := frame.Metrics["relative_return_energy_zscore"]

			if !hasCorrelationZ || !hasEnergyZ {
				if !yield(arriving) {
					return
				}

				continue
			}

			target := [2]float64{correlationZScore, energyZScore}
			sym := frame.Symbol
			points := op.historyPoints[sym]
			distances := op.historyDistances[sym]

			if len(points) > 0 {
				diffX := target[0] - points[0][0]
				diffY := target[1] - points[0][1]
				minDist := math.Sqrt(diffX*diffX + diffY*diffY)

				for index := 1; index < len(points); index++ {
					deltaX := target[0] - points[index][0]
					deltaY := target[1] - points[index][1]
					distance := math.Sqrt(deltaX*deltaX + deltaY*deltaY)

					if distance < minDist {
						minDist = distance
					}
				}

				if len(distances) > 0 {
					belowCount := 0

					for _, pastDist := range distances {
						if pastDist <= minDist {
							belowCount++
						}
					}

					frame.Metrics["historical_path_percentile"] = float64(belowCount) / float64(len(distances))
				}

				frame.Metrics["historical_path_distance"] = minDist
				distances = append(distances, minDist)

				if len(distances) > 256 {
					distances = distances[len(distances)-256:]
				}
			}

			points = append(points, target)

			if len(points) > 256 {
				points = points[len(points)-256:]
			}

			op.historyPoints[sym] = points
			op.historyDistances[sym] = distances

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
