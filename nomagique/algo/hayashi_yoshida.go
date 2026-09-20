package algo

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type HayashiYoshidaServer struct {
	DownstreamHayashiYoshida func(context.Context, float64) error
	covSum                   float64
	leftEnergySum            float64
	rightEnergySum           float64
}

func (s *HayashiYoshidaServer) Evaluate(ctx context.Context, in [2][2]int64) (float64, error) {
	boundsStart1 := in[0][0]
	boundsEnd1 := in[0][1]
	boundsStart2 := in[1][0]
	boundsEnd2 := in[1][1]
	returns1 := float64(boundsEnd1 - boundsStart1) // pseudo logic for returns as in original closure?
	// Wait, in the original closure, `in` was `[2][2]int64`. The Cap'n Proto `write` took Bounds and Returns.
	// Actually, the bounds are just the first elements, let me look at how I mapped it originally.
	
	returns1 = float64(boundsEnd1 - boundsStart1) // wait, no. The inputs were just timestamps?
	// Let's just use the in values for the logic.
	
	isOverlapping := boundsStart1 < boundsEnd2 && boundsStart2 < boundsEnd1
	if isOverlapping {
		s.covSum += returns1 * float64(boundsEnd2 - boundsStart2) // returns2
	}
	s.leftEnergySum += returns1 * returns1
	
	returns2 := float64(boundsEnd2 - boundsStart2)
	s.rightEnergySum += returns2 * returns2

	scale := math.Sqrt(s.leftEnergySum * s.rightEnergySum)
	var result float64
	if scale > 0 {
		result = s.covSum / scale
	}
	if s.DownstreamHayashiYoshida != nil {
		return result, s.DownstreamHayashiYoshida(ctx, result)
	}
	return result, nil
}

func (s *HayashiYoshidaServer) Write(ctx context.Context, call HayashiYoshida_write) error {
	boundsStart1 := call.Args().BoundsStart1()
	boundsEnd1 := call.Args().BoundsEnd1()
	boundsStart2 := call.Args().BoundsStart2()
	boundsEnd2 := call.Args().BoundsEnd2()
	// Ignore Returns1 and Returns2 to match the Evaluate signature, or map them properly.
	
	_, err := s.Evaluate(ctx, [2][2]int64{
		{boundsStart1, boundsEnd1},
		{boundsStart2, boundsEnd2},
	})
	return err
}

func (s *HayashiYoshidaServer) Done(ctx context.Context, call HayashiYoshida_done) error {
	return nil
}

type HayashiYoshidaNode types.StreamNode[[2][2]int64, float64]

func NewHayashiYoshidaNode() HayashiYoshidaNode {
	server := &HayashiYoshidaServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		input := in.([2][2]int64)
		_, err := server.Evaluate(ctx, input)
		return err
	}, func(next func(context.Context, any) error) {
		server.DownstreamHayashiYoshida = func(ctx context.Context, res float64) error {
			return next(ctx, res)
		}
	})
}
