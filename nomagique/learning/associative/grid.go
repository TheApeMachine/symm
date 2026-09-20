package associative

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"fmt"
	"math"
)

type GridServer struct {
	prev  []float64
	mean  []float64
	m2    []float64
	count float64
	Downstream func(context.Context, []byte) error
}

func NewGridServer() *GridServer {
	return &GridServer{}
}

func (s *GridServer) Write(ctx context.Context, call Grid_write) error {
	args, err := call.Args().Grid()
	if err != nil {
		return err
	}
	
	list, err := args.Impulse()
	if err != nil {
		return err
	}
	
	n := list.Len()
	if n == 0 {
		return nil
	}
	
	impulse := make([]float64, n)
	for i := 0; i < n; i++ {
		impulse[i] = list.At(i)
	}

	if s.Downstream == nil {
		return nil
	}

	if s.prev == nil {
		s.prev = make([]float64, n)
		s.mean = make([]float64, n)
		s.m2 = make([]float64, n)
		copy(s.prev, impulse)
		copy(s.mean, impulse)
		s.count = 1
		
		return s.Downstream(ctx, make([]byte, 32))
	}

	s.count++
	var globalMean, globalM2 float64

	for i, val := range impulse {
		delta := val - s.mean[i]
		s.mean[i] += delta / s.count
		delta2 := val - s.mean[i]
		s.m2[i] += delta * delta2

		gDelta := val - globalMean
		globalMean += gDelta / float64(i+1)
		gDelta2 := val - globalMean
		globalM2 += gDelta * gDelta2
	}

	globalStdDev := 0.0
	if n > 1 {
		globalStdDev = math.Sqrt(globalM2 / float64(n-1))
	}

	threshold := globalMean
	if n > 2 {
		threshold += globalStdDev
	}
	
	bestCell := -1
	maxPull := -1.0

	for i, val := range impulse {
		if val > threshold {
			variance := 0.0
			if s.count > 1 {
				variance = s.m2[i] / (s.count - 1)
			}

			snr := 0.0
			if variance > 0 {
				snr = math.Abs(s.mean[i]) / math.Sqrt(variance)
			}

			pull := val * snr
			if pull > maxPull {
				maxPull = pull
				bestCell = i
			}
		}
	}

	copy(s.prev, impulse)

	if bestCell >= 0 {
		return s.Downstream(ctx, fmt.Appendf(nil, "r%d", bestCell))
	}

	return nil
}

func (s *GridServer) Done(ctx context.Context, call Grid_done) error {
	return nil
}



type GridNode types.StreamNode[any, any]

func NewGrid() GridNode {
	server := &GridServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
