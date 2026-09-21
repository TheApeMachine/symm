package probability

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type KolmogorovSmirnovServer struct {
	window  []float64
	maxSize int
	out     float64
}

func NewKolmogorovSmirnov() *KolmogorovSmirnovServer {
	return &KolmogorovSmirnovServer{maxSize: 100}
}

func (s *KolmogorovSmirnovServer) Write(ctx context.Context, call KolmogorovSmirnov_write) error {
	val := call.Args().Value()
	s.window = append(s.window, val)
	if s.maxSize > 0 && len(s.window) > s.maxSize {
		s.window = s.window[1:]
	}

	s.out = s.computeKSStatistic()
	return nil
}

func (s *KolmogorovSmirnovServer) computeKSStatistic() float64 {
	numItems := len(s.window)
	if numItems == 0 {
		return 0.0
	}

	sorted := make([]float64, numItems)
	copy(sorted, s.window)

	for i := 1; i < numItems; i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var dMax float64
	for i, v := range sorted {
		ecdf := float64(i+1) / float64(numItems)
		cdf := 0.5 * (1.0 + math.Erf(v/math.Sqrt2))
		d := math.Abs(ecdf - cdf)
		if d > dMax {
			dMax = d
		}
	}
	return dMax
}

func (s *KolmogorovSmirnovServer) Done(ctx context.Context, call KolmogorovSmirnov_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
