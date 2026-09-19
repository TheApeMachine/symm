package physics

import (
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewSimulation creates a primitive closure wrapping the sensorium physics Engine.
It encapsulates the complex Metal/CUDA state into a standard types.Value pipeline.
*/
type Simulation types.Value[any, any]
func NewSimulation(gx, gy, gz int, spacing float32) Simulation {
	// We hardcode the path to the metallib for now, as it's not dynamically configured.
	metallibPath := "nomagique/physics/sensorium/kernels.metallib"
	
	// Initialize the engine once
	engine, err := sensorium.NewEngine(metallibPath, gx, gy, gz, spacing)
	if err != nil {
		// If the engine fails to load, we panic in the builder phase to fail fast,
		// or log. For now, we panic since primitives are built during initialization.
		panic(err)
	}

	return func(in any) any {
		// In a real pipeline, `in` would configure the step or input state,
		// but since we only need the primitive type available for now,
		// we just synchronize to ensure the engine ticks.
		engine.Synchronize()
		return in
	}
}
