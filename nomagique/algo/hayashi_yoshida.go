package algo

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type HayashiYoshidaServer struct {
	covSum         float64
	leftEnergySum  float64
	rightEnergySum float64
	out            float64
}

func (s *HayashiYoshidaServer) Evaluate(ctx context.Context, in [2][2]int64) (float64, error) {
	boundsStart1 := in[0][0]
	boundsEnd1 := in[0][1]
	boundsStart2 := in[1][0]
	boundsEnd2 := in[1][1]
	returns1 := float64(boundsEnd1 - boundsStart1)

	isOverlapping := boundsStart1 < boundsEnd2 && boundsStart2 < boundsEnd1
	if isOverlapping {
		s.covSum += returns1 * float64(boundsEnd2-boundsStart2)
	}

	s.leftEnergySum += returns1 * returns1

	returns2 := float64(boundsEnd2 - boundsStart2)
	s.rightEnergySum += returns2 * returns2

	scale := math.Sqrt(s.leftEnergySum * s.rightEnergySum)
	var result float64

	if scale > 0 {
		result = s.covSum / scale
	}

	return result, nil
}

func (s *HayashiYoshidaServer) Write(ctx context.Context, call HayashiYoshida_write) error {
	boundsStart1 := call.Args().BoundsStart1()
	boundsEnd1 := call.Args().BoundsEnd1()
	boundsStart2 := call.Args().BoundsStart2()
	boundsEnd2 := call.Args().BoundsEnd2()

	res, err := s.Evaluate(ctx, [2][2]int64{
		{boundsStart1, boundsEnd1},
		{boundsStart2, boundsEnd2},
	})
	if err != nil {
		return err
	}

	s.out = res
	return nil
}

func (s *HayashiYoshidaServer) Done(ctx context.Context, call HayashiYoshida_done) error {
	res, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	res.SetOut(s.out)
	s.out = 0
	return nil
}

func NewHayashiYoshida() *HayashiYoshidaServer {
	return &HayashiYoshidaServer{}
}
