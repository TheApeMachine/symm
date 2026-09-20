package learning

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

type ForecastServer struct {
	count float64
	mean  float64
	m2    float64
	m3    float64
	m4    float64

	outMean     float64
	outVariance float64
	outSkewness float64
	outKurtosis float64
}

func NewForecast() *ForecastServer {
	return &ForecastServer{}
}

func (s *ForecastServer) Write(ctx context.Context, call Forecast_write) error {
	in := call.Args().In()
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

	s.outMean = s.mean
	s.outVariance = variance
	s.outSkewness = skewness
	s.outKurtosis = kurtosis
	return nil
}

func (s *ForecastServer) Done(ctx context.Context, call Forecast_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.outMean)
	results.SetMean(s.outMean)
	results.SetVariance(s.outVariance)
	results.SetSkewness(s.outSkewness)
	results.SetKurtosis(s.outKurtosis)

	s.outMean = 0
	s.outVariance = 0
	s.outSkewness = 0
	s.outKurtosis = 0
	return nil
}
