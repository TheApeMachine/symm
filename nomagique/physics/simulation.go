package physics

import (
	"context"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

type SimulationServer struct {
	DownstreamSimulation func(context.Context, []byte) error
	Engine               *sensorium.Engine
}

func (s *SimulationServer) Write(ctx context.Context, call Simulation_write) error {
	data, _ := call.Args().Data()
	if s.Engine != nil {
		s.Engine.Synchronize()
	}
	if s.DownstreamSimulation != nil {
		return s.DownstreamSimulation(ctx, data)
	}

	return nil
}

func (s *SimulationServer) Done(ctx context.Context, call Simulation_done) error {
	return nil
}

func NewSimulation() *SimulationServer {
	return &SimulationServer{}
}
