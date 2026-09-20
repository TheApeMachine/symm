package probability

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type KolmogorovSmirnovServer struct {
	Downstream types.Float64Sink
	window     []float64
	maxSize    int
}

func (s *KolmogorovSmirnovServer) Write(ctx context.Context, payload any) error {
	val, ok := payload.(float64)
	if !ok {
		return nil
	}

	s.window = append(s.window, val)
	if s.maxSize > 0 && len(s.window) > s.maxSize {
		s.window = s.window[1:]
	}

	stat := s.computeKSStatistic()
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(stat)
			return nil
		})
	}
	return nil
}

func (s *KolmogorovSmirnovServer) computeKSStatistic() float64 {
	n := len(s.window)
	if n == 0 {
		return 0.0
	}

	sorted := make([]float64, n)
	copy(sorted, s.window)

	for i := 1; i < n; i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var dMax float64
	for i, v := range sorted {
		ecdf := float64(i+1) / float64(n)
		cdf := 0.5 * (1.0 + math.Erf(v/math.Sqrt2))
		d := math.Abs(ecdf - cdf)
		if d > dMax {
			dMax = d
		}
	}
	return dMax
}

func (s *KolmogorovSmirnovServer) Done(ctx context.Context) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewKolmogorovSmirnov() *KolmogorovSmirnovServer {
	return &KolmogorovSmirnovServer{maxSize: 100}
}
