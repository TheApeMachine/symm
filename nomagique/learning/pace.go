package learning

import (
	"context"
	"math"
	"sort"
)

type PaceServer struct {
	DownstreamPace func(context.Context, float64) error
	Rest           float64
	Lower          float64
	Upper          float64
	Gain           float64
	Band           float64
	Window         int
	// state
	history []float64
	ema     float64
	count   int
}

func (s *PaceServer) Write(ctx context.Context, call Pace_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *PaceServer) WriteParams(ctx context.Context, callArgs Pace_write_Params) error {
	errorMag := callArgs.ErrorMagnitude()

	// 1. Maintain history and map error magnitude to empirical rank (Calibrator)
	if s.history == nil {
		s.history = make([]float64, 0, s.Window)
	}

	if len(s.history) >= s.Window {
		s.history = s.history[1:] // pop first
	}
	s.history = append(s.history, errorMag)

	// Compute empirical rank
	sorted := make([]float64, len(s.history))
	copy(sorted, s.history)
	sort.Float64s(sorted)

	rank := 0.0
	for i, v := range sorted {
		if errorMag <= v {
			rank = float64(i) / float64(len(sorted)-1)
			break
		}
		if i == len(sorted)-1 {
			rank = 1.0
		}
	}
	if len(sorted) <= 1 {
		rank = 0.5
	}

	// 2. Map rank to target log-alpha based on bands (Threshold)
	restLog := math.Log(s.Rest)
	lowerLog := math.Log(s.Lower)
	upperLog := math.Log(s.Upper)

	targetLog := restLog
	if rank < s.Band {
		targetLog = lowerLog
	} else if rank > 1.0-s.Band {
		targetLog = upperLog
	}

	// 3. Smooth target log-alpha with EMA
	if s.count == 0 {
		s.ema = targetLog
	} else {
		s.ema = s.Gain*targetLog + (1.0-s.Gain)*s.ema
	}
	s.count++

	// 4. Clamp log space
	if s.ema < lowerLog {
		s.ema = lowerLog
	}
	if s.ema > upperLog {
		s.ema = upperLog
	}

	// 5. Exp
	alpha := math.Exp(s.ema)

	// 6. Clamp hard bounds
	if alpha < s.Lower {
		alpha = s.Lower
	}
	if alpha > s.Upper {
		alpha = s.Upper
	}

	if s.DownstreamPace != nil {
		return s.DownstreamPace(ctx, alpha)
	}
	return nil
}

func (s *PaceServer) Done(ctx context.Context, call Pace_done) error {
	return nil
}
