package store

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"math"
)

/*
resonanceTask retains an affine least-squares model without a ridge or invented
prior samples. Greville's row-augmentation update maintains the Moore-Penrose
inverse of the observed Gram matrix and its null-space projector. Predictive
uncertainty is Student-t uncertainty from the measured residual degrees of freedom.
*/
type resonanceTask struct {
	beta         []float64
	inverse      [][]float64
	null         [][]float64
	rank         int
	observations int
	residual     float64
}

func newResonanceTask(width int) *resonanceTask {
	dimension := width + 1 // The constant coordinate is the affine intercept.
	task := &resonanceTask{beta: make([]float64, dimension), inverse: make([][]float64, dimension), null: make([][]float64, dimension)}
	for row := range dimension {
		task.inverse[row] = make([]float64, dimension)
		task.null[row] = make([]float64, dimension)
		task.null[row][row] = 1
	}
	return task
}

func (task *resonanceTask) Evaluate(features []float64, target float64, observed bool) algo.RLSPosterior {
	design := make([]float64, len(task.beta))
	design[0] = 1
	copy(design[1:], features)
	projected := make([]float64, len(design))
	independent := make([]float64, len(design))
	prediction, energy, independentEnergy, designEnergy := 0.0, 0.0, 0.0, 0.0
	for row, value := range design {
		prediction += task.beta[row] * value
		designEnergy += value * value
		for column, coordinate := range design {
			projected[row] += task.inverse[row][column] * coordinate
			independent[row] += task.null[row][column] * coordinate
		}
		energy += value * projected[row]
		independentEnergy += independent[row] * independent[row]
	}
	forecast := algo.RLSForecast{RLSState: algo.RLSState{Beta: task.beta, Design: design, Observations: float64(task.observations)}, Prediction: prediction}
	resolution := math.Nextafter(1, 2) - 1
	newDirection := task.rank < len(design) && independentEnergy > resolution*resolution*float64(len(design))*designEnergy
	degrees := task.observations - task.rank
	if degrees > 0 && !newDirection {
		forecast.DegreesOfFreedom = float64(degrees)
		forecast.PredictiveVariance = task.residual / float64(degrees) * (1 + energy)
		forecast.Scale = math.Sqrt(forecast.PredictiveVariance)
		forecast.Ready = true
	}
	if !observed {
		return algo.RLSPosterior{RLSForecast: forecast}
	}
	innovation := target - prediction
	gain := make([]float64, len(design))
	// Numerical rank uses floating-point resolution, not a market confidence cutoff.
	if newDirection {
		for row := range gain {
			gain[row] = independent[row] / independentEnergy
		}
		for row := range gain {
			for column := range gain {
				task.inverse[row][column] += (1+energy)*gain[row]*gain[column] - gain[row]*projected[column] - projected[row]*gain[column]
				task.null[row][column] -= independent[row] * independent[column] / independentEnergy
			}
		}
		task.rank++
	}
	if !newDirection {
		for row := range gain {
			gain[row] = projected[row] / (1 + energy)
		}
		for row := range gain {
			for column := range gain {
				task.inverse[row][column] -= gain[row] * projected[column]
			}
		}
		task.residual += innovation * innovation / (1 + energy)
	}
	for row := range task.beta {
		task.beta[row] += gain[row] * innovation
	}
	task.observations++
	forecast.Beta = task.beta
	return algo.RLSPosterior{RLSForecast: forecast, Innovation: innovation, Gain: gain}
}
