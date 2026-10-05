package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Relaxation performs one weighted stress descent displacement step.
*/
type Relaxation struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewRelaxation() *Relaxation {
	output := data.NewOutputMap()
	output.Values["x"] = 0
	output.Values["y"] = 0
	output.Values["displacement"] = 0

	return &Relaxation{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"x", "x",
			"y", "y",
			"target_x", "target_x",
			"target_y", "target_y",
			"strength", "strength",
			"authority", "authority",
		),
		output: output,
	}
}

func (op *Relaxation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			currentX, xOK := values.Values["x"]
			currentY, yOK := values.Values["y"]
			targetX, txOK := values.Values["target_x"]
			targetY, tyOK := values.Values["target_y"]
			strength, strOK := values.Values["strength"]
			authority, authOK := values.Values["authority"]

			if !xOK || !yOK || !txOK || !tyOK || !strOK || !authOK {
				op.Error(core.ErrNotHeld)
				return
			}

			deltaX := targetX - currentX
			deltaY := targetY - currentY
			distance := math.Hypot(deltaX, deltaY)

			displacement := 0.0
			newX := currentX
			newY := currentY

			if distance > 0 && authority > 0 && strength != 0 {
				targetDist := 1.0 - strength

				if strength > 0 {
					targetDist = 1.0 / (1.0 + strength)
				}

				weight := math.Abs(strength)
				force := weight * (distance - targetDist) / distance
				dx := force * deltaX / authority
				dy := force * deltaY / authority

				newX += dx
				newY += dy
				displacement = dx*dx + dy*dy
			}

			op.output.Values["x"] = newX
			op.output.Values["y"] = newY
			op.output.Values["displacement"] = displacement

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
