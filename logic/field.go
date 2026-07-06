package logic

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

type physicalField struct{}

func newPhysicalField() *physicalField {
	return &physicalField{}
}

func (field *physicalField) Rho(rows [][]float64) (rhoEvidence, error) {
	if len(rows) == 0 {
		return rhoEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: rho projection required",
			nil,
		))
	}

	total := 0.0
	peak := 0.0
	centerX := 0.0
	centerZ := 0.0

	for rowIndex, row := range rows {
		if len(row) == 0 {
			return rhoEvidence{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"decision physical: rho row required",
				nil,
			))
		}

		for columnIndex, value := range row {
			if !finite(value) {
				return rhoEvidence{}, errnie.Error(errnie.Err(
					errnie.Validation,
					"decision physical: rho projection must be finite",
					nil,
				))
			}

			if value <= 0 {
				continue
			}

			total += value
			centerX += float64(columnIndex) * value
			centerZ += float64(rowIndex) * value

			if value > peak {
				peak = value
			}
		}
	}

	if total <= 0 {
		return rhoEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: rho projection mass required",
			nil,
		))
	}

	out := rhoEvidence{
		mass:    total,
		peak:    peak,
		centerX: centerX / total,
		centerZ: centerZ / total,
	}

	return field.rhoShape(rows, out)
}

func (field *physicalField) rhoShape(
	rows [][]float64,
	out rhoEvidence,
) (rhoEvidence, error) {
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			if value <= 0 {
				continue
			}

			share := value / out.mass
			deltaX := float64(columnIndex) - out.centerX
			deltaZ := float64(rowIndex) - out.centerZ

			out.spreadX += deltaX * deltaX * share
			out.spreadZ += deltaZ * deltaZ * share
			out.entropy -= share * math.Log(share)
			out.gradient += field.localGradient(rows, rowIndex, columnIndex)
		}
	}

	return out, nil
}

func (field *physicalField) localGradient(
	rows [][]float64,
	rowIndex int,
	columnIndex int,
) float64 {
	current := rows[rowIndex][columnIndex]
	gradient := 0.0

	if columnIndex > 0 {
		gradient += math.Abs(current - rows[rowIndex][columnIndex-1])
	}

	if columnIndex+1 < len(rows[rowIndex]) {
		gradient += math.Abs(current - rows[rowIndex][columnIndex+1])
	}

	if rowIndex > 0 && columnIndex < len(rows[rowIndex-1]) {
		gradient += math.Abs(current - rows[rowIndex-1][columnIndex])
	}

	if rowIndex+1 < len(rows) && columnIndex < len(rows[rowIndex+1]) {
		gradient += math.Abs(current - rows[rowIndex+1][columnIndex])
	}

	return gradient
}

func (field *physicalField) Oscillators(
	oscillators []pmanifold.Oscillator,
) (oscillatorEvidence, error) {
	if len(oscillators) == 0 {
		return oscillatorEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: oscillator clamps required",
			nil,
		))
	}

	var out oscillatorEvidence
	phaseX := 0.0
	phaseY := 0.0
	weight := 0.0

	for _, oscillator := range oscillators {
		values := map[string]float64{
			"phase":     oscillator.Phase,
			"omega":     oscillator.Omega,
			"amplitude": oscillator.Amplitude,
			"heat":      oscillator.Heat,
			"vel_x":     oscillator.VelX,
			"vel_z":     oscillator.VelZ,
		}

		for name, value := range values {
			if !finite(value) {
				return oscillatorEvidence{}, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"decision physical: oscillator %s must be finite",
						name,
					),
					nil,
				))
			}
		}

		amplitude := math.Abs(oscillator.Amplitude)
		phaseX += math.Cos(oscillator.Phase) * amplitude
		phaseY += math.Sin(oscillator.Phase) * amplitude
		weight += amplitude
		out.kinetic += math.Sqrt(
			oscillator.VelX*oscillator.VelX +
				oscillator.VelZ*oscillator.VelZ,
		)
		out.thermal += math.Abs(oscillator.Heat)
		out.omega += math.Abs(oscillator.Omega)
	}

	if weight > 0 {
		out.coherence = math.Sqrt(phaseX*phaseX+phaseY*phaseY) / weight
	}

	count := float64(len(oscillators))
	out.kinetic /= count
	out.thermal /= count
	out.omega /= count

	return out, nil
}
