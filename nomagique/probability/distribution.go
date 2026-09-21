package probability

import (
	"context"

	"github.com/theapemachine/errnie"
)

type DistributionServer struct {
	winner     int64
	confidence float64
	ambiguity  float64
	sharpness  float64
}

func NewDistribution() *DistributionServer {
	return &DistributionServer{}
}

func (server *DistributionServer) Write(ctx context.Context, call Distribution_write) error {
	val := call.Args().Value()
	server.winner = 0
	server.confidence = val
	server.sharpness = val
	server.ambiguity = 1.0 - val
	return nil
}

func (server *DistributionServer) Done(ctx context.Context, call Distribution_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"distribution: alloc results failed",
			err,
		))
	}

	results.SetOut(server.confidence)
	results.SetWinner(server.winner)
	results.SetConfidence(server.confidence)
	results.SetAmbiguity(server.ambiguity)
	results.SetSharpness(server.sharpness)

	server.winner = 0
	server.confidence = 0
	server.ambiguity = 0
	server.sharpness = 0
	return nil
}
