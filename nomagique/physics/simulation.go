package physics

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

type SimulationServer struct {
	Engine *sensorium.Engine
	out    []byte
}

func NewSimulation() *SimulationServer {
	return &SimulationServer{}
}

func (server *SimulationServer) Write(ctx context.Context, call Simulation_write) error {
	data, _ := call.Args().Data()
	if server.Engine != nil {
		server.Engine.Synchronize()
	}

	server.out = bytes.Clone(data)
	return nil
}

func (server *SimulationServer) Done(ctx context.Context, call Simulation_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"simulation: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"simulation: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
