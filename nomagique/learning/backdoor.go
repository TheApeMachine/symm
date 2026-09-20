package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BackdoorServer struct {
	effect float64
	out    float64
}

func NewBackdoor() *BackdoorServer {
	return &BackdoorServer{}
}

func (server *BackdoorServer) Write(ctx context.Context, call Backdoor_write) error {
	args := call.Args()
	treatment := args.Treatment()
	outcome := args.Outcome()
	adjustment := args.Adjustment()

	server.effect = outcome - treatment*adjustment
	server.out = server.effect
	return nil
}

func (server *BackdoorServer) Done(ctx context.Context, call Backdoor_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"backdoor: alloc results failed",
			err,
		))
	}

	results.SetEffect(server.effect)
	results.SetOut(server.out)
	server.effect = 0
	server.out = 0
	return nil
}
