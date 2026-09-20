package learning

import (
	"context"

	"github.com/theapemachine/symm/nomagique/core"
)

type ForecastServer struct {
	DownstreamForecast func(context.Context, float64, float64, float64, float64) error
	count              float64
	mean               float64
	m2                 float64
	m3                 float64
	m4                 float64
}

func (s *ForecastServer) Write(ctx context.Context, call Forecast_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *ForecastServer) WriteParams(ctx context.Context, callArgs Forecast_write_Params) error {
	in := callArgs.In()
	s.count++
	n := s.count
	delta := in - s.mean
	deltaN := delta / n
	deltaN2 := deltaN * deltaN
	term1 := delta * deltaN * (n - 1)

	s.mean += deltaN
	s.m4 += term1*deltaN2*(n*n-3*n+3) + 6*deltaN2*s.m2 - 4*deltaN*s.m3
	s.m3 += term1*deltaN*(n-2) - 3*deltaN*s.m2
	s.m2 += term1

	variance := 0.0
	skewness := 0.0
	kurtosis := 0.0

	if n > core.Unit {
		variance = s.m2 / (n - core.Unit)
	}
	if s.m2 > 0 {
		skewness = (core.Unit * n * s.m3) / (s.m2 * s.m2)
		kurtosis = (n * s.m4) / (s.m2 * s.m2)
	}

	if s.DownstreamForecast != nil {
		return s.DownstreamForecast(ctx, s.mean, variance, skewness, kurtosis)
	}
	return nil
}

func (s *ForecastServer) Done(ctx context.Context, call Forecast_done) error {
	return nil
}
