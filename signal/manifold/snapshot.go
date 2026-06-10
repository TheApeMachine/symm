package manifold

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/physics"
)

func (field *Field) snapshotPayload(eventAt time.Time) (map[string]any, error) {
	if eventAt.IsZero() {
		return nil, fmt.Errorf("manifold: snapshot event time is zero")
	}

	rho, rhoErr := field.solver.ReadRhoProjection()

	if rhoErr != nil {
		return nil, rhoErr
	}

	if len(rho) == 0 {
		return nil, nil
	}

	if err := requireFiniteRho(rho); err != nil {
		return nil, err
	}

	snapshotReading := field.lastReading

	reading, readingErr := readingRow(snapshotReading)

	if readingErr != nil {
		return nil, readingErr
	}

	carriers := make([]map[string]any, 0, len(field.lastCarriers))

	for _, carrier := range field.lastCarriers {
		row, carrierErr := carrierRow(field.config, carrier)

		if carrierErr != nil {
			return nil, carrierErr
		}

		carriers = append(carriers, row)
	}

	payload := map[string]any{
		"type": "manifold",
		"ts":   eventAt.UTC().Format(time.RFC3339Nano),
		"grid": map[string]any{
			"x":       field.config.GridX,
			"y":       field.config.GridY,
			"z":       field.config.GridZ,
			"spacing": field.config.GridSpacing(),
		},
		"rho":      rho,
		"reading":  reading,
		"carriers": carriers,
	}

	return payload, nil
}

func readingRow(reading physics.Reading) (map[string]any, error) {
	fields := map[string]float64{
		"pressure_grad_x":    reading.PressureGradX,
		"pressure_grad_y":    reading.PressureGradY,
		"pressure_grad_z":    reading.PressureGradZ,
		"pressure_grad_norm": reading.PressureGradNorm,
		"divergence":         reading.Divergence,
		"coherence_mag2":     reading.CoherenceMag2,
		"guidance_speed":     reading.GuidanceSpeed,
		"viscosity_proxy":    reading.ViscosityProxy,
	}

	row := make(map[string]any, len(fields))

	for name, value := range fields {
		if err := requireFinite(name, value); err != nil {
			return nil, err
		}

		row[name] = value
	}

	return row, nil
}

func carrierRow(config physics.Config, carrier fieldCarrier) (map[string]any, error) {
	spacing := config.GridSpacing()

	if spacing <= 0 {
		return nil, fmt.Errorf("manifold: grid spacing must be positive")
	}

	cellX := int(carrier.oscillator.PosX/spacing+0.5) % int(config.GridX)
	cellY := int(carrier.oscillator.PosY/spacing+0.5) % int(config.GridY)
	cellZ := int(carrier.oscillator.PosZ/spacing+0.5) % int(config.GridZ)

	floatFields := map[string]float64{
		"x":         carrier.oscillator.PosX,
		"y":         carrier.oscillator.PosY,
		"z":         carrier.oscillator.PosZ,
		"amplitude": carrier.oscillator.Amplitude,
		"heat":      carrier.oscillator.Heat,
		"omega":     carrier.oscillator.Omega,
		"phase":     carrier.oscillator.Phase,
		"vel_x":     carrier.oscillator.VelX,
		"vel_y":     carrier.oscillator.VelY,
		"vel_z":     carrier.oscillator.VelZ,
	}

	for name, value := range floatFields {
		if err := requireFinite(name, value); err != nil {
			return nil, err
		}
	}

	return map[string]any{
		"role":      carrier.role,
		"symbol":    carrier.symbol,
		"x":         floatFields["x"],
		"y":         floatFields["y"],
		"z":         floatFields["z"],
		"cell_x":    cellX,
		"cell_y":    cellY,
		"cell_z":    cellZ,
		"amplitude": floatFields["amplitude"],
		"heat":      floatFields["heat"],
		"omega":     floatFields["omega"],
		"phase":     floatFields["phase"],
		"vel_x":     floatFields["vel_x"],
		"vel_y":     floatFields["vel_y"],
		"vel_z":     floatFields["vel_z"],
	}, nil
}

func requireFinite(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("manifold: %s is non-finite", name)
	}

	return nil
}

func requireFiniteRho(rho [][]float64) error {
	for rowIndex, row := range rho {
		for colIndex, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf(
					"manifold: rho[%d][%d] is non-finite",
					rowIndex,
					colIndex,
				)
			}
		}
	}

	return nil
}
