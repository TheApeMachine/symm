package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type ZScoreServer struct {
	out   float64
	mean  float64
	m2    float64
	count float64
}

func (srv *ZScoreServer) Write(ctx context.Context, call ZScore_write) error {
	inVal := call.Args().Value()
	srv.count++
	delta := inVal - srv.mean
	srv.mean += delta / srv.count
	delta2 := inVal - srv.mean
	srv.m2 += delta * delta2

	if srv.count <= 1 {
		srv.out = 0
		return nil
	}

	variance := srv.m2 / (srv.count - 1)
	if variance <= 0 {
		srv.out = 0
		return nil
	}

	srv.out = (inVal - srv.mean) / math.Sqrt(variance)
	return nil
}

func (srv *ZScoreServer) Done(ctx context.Context, call ZScore_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc z_score results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewZScore() *ZScoreServer {
	return &ZScoreServer{}
}
