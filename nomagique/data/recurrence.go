package data

import (
	"errors"
	"iter"
	"math"
	"strings"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Recurrence tracks historical standardized trajectories and measures
recurrence/novelty via nearest-neighbour distance and empirical percentile.
*/
type Recurrence struct {
	err       error
	keys      []string
	history   [][]float64
	distances []float64
	capacity  int
}

/*
NewRecurrence creates a trajectory recurrence primitive for the declared keys.
If no keys are given, all metrics ending in _zscore are observed.
*/
func NewRecurrence(keys ...string) core.Primitive {
	return &Recurrence{
		keys:      keys,
		history:   make([][]float64, 0, 512),
		distances: make([]float64, 0, 512),
		capacity:  512,
	}
}

func (op *Recurrence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**Measurement[float64])(arriving)

			if measurement != nil {
				vector := op.extractVector(measurement)

				if len(vector) > 0 {
					if len(op.history) > 0 {
						minDistance := math.MaxFloat64

						for _, prior := range op.history {
							distance := op.euclidean(vector, prior)

							if distance < minDistance {
								minDistance = distance
							}
						}

						if minDistance < math.MaxFloat64 {
							op.distances = append(op.distances, minDistance)

							lessCount := 0.0
							for _, distVal := range op.distances {
								if distVal <= minDistance {
									lessCount++
								}
							}

							percentile := lessCount / float64(len(op.distances))

							measurement.WriteMetric("historical_path_distance", minDistance)
							measurement.WriteNormalized("historical_path_percentile", percentile)

							if len(op.distances) > op.capacity {
								op.distances = op.distances[1:]
							}
						}
					}

					op.history = append(op.history, vector)

					if len(op.history) > op.capacity {
						op.history = op.history[1:]
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Recurrence) extractVector(measurement *Measurement[float64]) []float64 {
	targetKeys := op.keys

	if len(targetKeys) == 0 {
		var zKeys []string
		measurement.RangeMetrics(func(k string, metric Metric[float64]) bool {
			if strings.HasSuffix(k, "_zscore") {
				zKeys = append(zKeys, k)
			}
			return true
		})
		targetKeys = zKeys
	}

	if len(targetKeys) == 0 {
		return nil
	}

	vector := make([]float64, 0, len(targetKeys))

	for _, key := range targetKeys {
		metric, found := measurement.LookupMetric(key)

		if !found {
			return nil
		}

		if math.IsNaN(metric.Raw) || math.IsInf(metric.Raw, 0) {
			return nil
		}

		vector = append(vector, metric.Raw)
	}

	return vector
}

func (op *Recurrence) euclidean(vectorA, vectorB []float64) float64 {
	length := len(vectorA)

	if length == 0 || length != len(vectorB) {
		return math.MaxFloat64
	}

	sumSquares := 0.0

	for index := 0; index < length; index++ {
		diff := vectorA[index] - vectorB[index]
		sumSquares += diff * diff
	}

	return math.Sqrt(sumSquares / float64(length))
}

func (op *Recurrence) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
