package probability

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type KolmogorovSmirnovNode types.StreamNode[any, any]

type KolmogorovSmirnovServer struct {
	Downstream func(context.Context, any) error
	window     []float64
	maxSize    int
}

func (s *KolmogorovSmirnovServer) Write(ctx context.Context, payload any) error {
	val, ok := payload.(float64)
	if !ok {
		return s.Downstream(ctx, payload) // pass-through if not float
	}

	s.window = append(s.window, val)
	if len(s.window) > s.maxSize {
		s.window = s.window[1:]
	}

	stat := s.computeKSStatistic()
	return s.Downstream(ctx, stat)
}

func (s *KolmogorovSmirnovServer) computeKSStatistic() float64 {
	n := len(s.window)
	if n == 0 {
		return 0.0
	}

	// Calculate empirical CDF and compare to standard normal CDF
	// First sort a copy of the window
	sorted := make([]float64, n)
	copy(sorted, s.window)
	
	// simple insertion sort for small window
	for i := 1; i < n; i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var dMax float64
	for i, v := range sorted {
		ecdf := float64(i+1) / float64(n)
		
		// Approximate Standard Normal CDF
		cdf := 0.5 * (1.0 + math.Erf(v/math.Sqrt2))
		
		d := math.Abs(ecdf - cdf)
		if d > dMax {
			dMax = d
		}
	}
	return dMax
}

func NewKolmogorovSmirnov() KolmogorovSmirnovNode {
	server := &KolmogorovSmirnovServer{
		maxSize: 100, // sliding window size
	}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return server.Write(ctx, payload)
		},
		func(next func(context.Context, any) error) {
			server.Downstream = next
		},
	)
}
