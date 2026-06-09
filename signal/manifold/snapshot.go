package manifold

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/numeric/physics"
)

func (system *System) publishSnapshot(eventAt time.Time) error {
	payload, err := system.field.snapshotPayload(eventAt)

	if err != nil {
		return err
	}

	if payload == nil {
		return nil
	}

	return system.bus.Send("ui", "field_snapshot", payload)
}

func (field *Field) snapshotPayload(eventAt time.Time) (map[string]any, error) {
	if eventAt.IsZero() {
		return nil, fmt.Errorf("manifold: snapshot event time is zero")
	}

	field.stepMu.Lock()
	defer field.stepMu.Unlock()

	rho, rhoErr := field.solver.ReadRhoProjection()

	if rhoErr != nil {
		return nil, rhoErr
	}

	if len(rho) == 0 {
		return nil, nil
	}

	carriers := make([]map[string]any, 0, len(field.lastCarriers))

	for _, carrier := range field.lastCarriers {
		carriers = append(carriers, carrierRow(field.config, carrier))
	}

	return map[string]any{
		"type": "manifold",
		"ts":   eventAt.UTC().Format(time.RFC3339Nano),
		"grid": map[string]any{
			"x":       field.config.GridX,
			"y":       field.config.GridY,
			"z":       field.config.GridZ,
			"spacing": field.config.GridSpacing(),
		},
		"rho":      rho,
		"reading":  readingRow(field.lastReading),
		"carriers": carriers,
	}, nil
}

func readingRow(reading physics.Reading) map[string]any {
	return map[string]any{
		"pressure_grad_x":    reading.PressureGradX,
		"pressure_grad_y":    reading.PressureGradY,
		"pressure_grad_z":    reading.PressureGradZ,
		"pressure_grad_norm": reading.PressureGradNorm,
		"divergence":         reading.Divergence,
		"coherence_mag2":     reading.CoherenceMag2,
		"guidance_speed":     reading.GuidanceSpeed,
		"viscosity_proxy":    reading.ViscosityProxy,
	}
}

func carrierRow(config physics.Config, carrier fieldCarrier) map[string]any {
	spacing := config.GridSpacing()

	if spacing <= 0 {
		spacing = 1
	}

	cellX := int(carrier.oscillator.PosX/spacing+0.5) % int(config.GridX)
	cellY := int(carrier.oscillator.PosY/spacing+0.5) % int(config.GridY)
	cellZ := int(carrier.oscillator.PosZ/spacing+0.5) % int(config.GridZ)

	return map[string]any{
		"role":      carrier.role,
		"symbol":    carrier.symbol,
		"x":         carrier.oscillator.PosX,
		"y":         carrier.oscillator.PosY,
		"z":         carrier.oscillator.PosZ,
		"cell_x":    cellX,
		"cell_y":    cellY,
		"cell_z":    cellZ,
		"amplitude": carrier.oscillator.Amplitude,
		"heat":      carrier.oscillator.Heat,
		"omega":     carrier.oscillator.Omega,
		"phase":     carrier.oscillator.Phase,
		"vel_x":     carrier.oscillator.VelX,
		"vel_y":     carrier.oscillator.VelY,
		"vel_z":     carrier.oscillator.VelZ,
	}
}
