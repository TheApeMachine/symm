package physics

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewSimulation creates a primitive closure wrapping the sensorium physics Engine.
It encapsulates the complex Metal/CUDA state into a standard types.Value pipeline.
*/
type Simulation types.Value[any, any]

func NewSimulation(gx, gy, gz types.Integer, spacing types.Float) Simulation {
	metallibPath := "nomagique/physics/sensorium/kernels.metallib"

	x, y, z := 0, 0, 0
	if gx != nil {
		x = gx(nil)
	}
	if gy != nil {
		y = gy(nil)
	}
	if gz != nil {
		z = gz(nil)
	}
	sp := float32(1.0)
	if spacing != nil {
		sp = float32(spacing(nil))
	}

	engine, err := sensorium.NewEngine(metallibPath, x, y, z, sp)
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"physics: failed to initialize sensorium engine",
			err,
		))
		return func(in any) any { return in }
	}

	return func(in any) any {
		engine.Synchronize()
		return in
	}
}
