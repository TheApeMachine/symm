package data

import (
	"context"

	"github.com/theapemachine/errnie"
)

type QualityServer struct {
	support        float64
	divergence     float64
	noiseVariance  float64
	mahalanobisSNR float64
	maturity       float64
}

func NewQuality() *QualityServer {
	return &QualityServer{}
}

func (server *QualityServer) Write(ctx context.Context, call Quality_write) error {
	args := call.Args()
	server.support = args.Support()
	server.divergence = args.Divergence()
	server.noiseVariance = args.NoiseVariance()
	server.mahalanobisSNR = args.MahalanobisSNR()
	server.maturity = args.Maturity()
	return nil
}

func (server *QualityServer) Done(ctx context.Context, call Quality_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"quality: alloc results failed",
			err,
		))
	}

	estimated := server.support > 0 || server.divergence != 0 || server.mahalanobisSNR > 0
	results.SetEstimated(estimated)

	mat := 1.0
	var snr float64
	var snrDefined bool

	if server.noiseVariance > 0 {
		snr = server.divergence * server.divergence / server.noiseVariance
		snrDefined = true
	}

	if server.support > 0 {
		mat = 0.0

		if server.support > 1 {
			mat = 1.0 - 1.0/server.support

			if server.mahalanobisSNR > 0 {
				snr = server.mahalanobisSNR
				snrDefined = true
			}
		}
	}

	if server.support == 0 && server.maturity > 0 {
		mat = server.maturity
	}

	results.SetMaturity(mat)
	results.SetSnr(snr)
	results.SetSnrDefined(snrDefined)

	*server = QualityServer{}
	return nil
}
