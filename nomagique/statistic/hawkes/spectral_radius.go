package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SpectralRadiusServer struct {
	out float64
}

func NewSpectralRadius() *SpectralRadiusServer {
	return &SpectralRadiusServer{}
}

func (server *SpectralRadiusServer) Write(ctx context.Context, call SpectralRadius_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *SpectralRadiusServer) Done(ctx context.Context, call SpectralRadius_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"spectral_radius: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
