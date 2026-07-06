package logic

import (
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

func (frame *boundaryFrame) observe(measurement *types.Measurement) {
	if frame.price <= 0 && measurement.Metrics != nil {
		price := measurement.Metrics["price"]
		if price > 0 && finite(price) {
			frame.price = price
		}
	}

	if frame.eventAt.IsZero() || measurement.At.After(frame.eventAt) {
		frame.eventAt = measurement.At
	}
}

func (frame boundaryFrame) Intervene() boundaryFrame {
	out := frame
	out.clamps = make([]fieldClamp, 0, len(frame.clamps))
	out.oscillators = make([]pmanifold.Oscillator, 0, len(frame.oscillators))
	direction := frame.direction()

	for _, clamp := range frame.clamps {
		intervened := clamp.intervene(direction)
		out.clamps = append(out.clamps, intervened)
		out.oscillators = append(out.oscillators, intervened.oscillator())
	}

	return out
}

func (frame boundaryFrame) direction() float64 {
	momentum := frame.netMomentum()

	if momentum > 0 {
		return 1
	}

	if momentum < 0 {
		return -1
	}

	return 0
}

func (frame boundaryFrame) netMomentum() float64 {
	total := 0.0

	for _, clamp := range frame.clamps {
		total += clamp.momX
	}

	return total
}

func (frame boundaryFrame) netPressure() float64 {
	total := 0.0

	for _, clamp := range frame.clamps {
		total += clamp.pressure
	}

	return total
}

func (clamp fieldClamp) oscillator() pmanifold.Oscillator {
	phase := math.Atan2(clamp.momX, clamp.pressure)

	return pmanifold.Oscillator{
		Phase:     phase,
		Omega:     math.Abs(clamp.pressure),
		Amplitude: math.Sqrt(clamp.rho),
		PosX:      clamp.positionX,
		PosY:      float64(clamp.lane),
		PosZ:      clamp.positionZ,
		Heat:      clamp.energy,
		VelX:      clamp.momX,
		VelY:      clamp.momY,
		VelZ:      clamp.momZ,
	}
}

func (clamp fieldClamp) intervene(direction float64) fieldClamp {
	if direction == 0 || clamp.momX*direction >= 0 {
		return clamp
	}

	clamp.momX = 0
	clamp.positionX = signedPosition(clamp.momX)

	return clamp
}

func signedPosition(momentum float64) float64 {
	if momentum > 0 {
		return 1
	}

	if momentum < 0 {
		return 0
	}

	return 0.5
}

func pressurePosition(risk float64, pressure float64) float64 {
	if pressure <= 0 {
		return 0
	}

	position := risk / pressure

	if position < 0 {
		return 0
	}

	if position > 1 {
		return 1
	}

	return position
}

func bestCategory(rows []types.Category) types.Category {
	if len(rows) == 0 {
		return types.Category{Type: types.CategoryTypeNone}
	}

	out := rows[0]
	
	for _, row := range rows[1:] {
		if row.Confidence > out.Confidence {
			out = row
		}
	}

	return out
}
