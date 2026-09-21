package probability

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ConcentrationServer struct {
	out float64
}

func NewConcentration() *ConcentrationServer {
	return &ConcentrationServer{}
}

func (server *ConcentrationServer) Write(ctx context.Context, call Concentration_write) error {
	val := call.Args().Value()
	server.out = val * val
	return nil
}

func (server *ConcentrationServer) Done(ctx context.Context, call Concentration_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"concentration: alloc results failed",
			err,
		))
	}

	results.SetOut(server.out)
	server.out = 0
	return nil
}
