package learning

import (
	"context"
	"math"
	"sort"

	"github.com/theapemachine/errnie"
)

type PaceServer struct {
	Rest    float64
	Lower   float64
	Upper   float64
	Gain    float64
	Band    float64
	Window  int
	history []float64
	ema     float64
	count   int
	out     float64
}

func NewPace() *PaceServer {
	return &PaceServer{}
}

func (s *PaceServer) Write(ctx context.Context, call Pace_write) error {
	errorMag := call.Args().ErrorMagnitude()

	if s.history == nil {
		win := s.Window
		if win <= 0 {
			win = 100
		}
		s.history = make([]float64, 0, win)
	}

	if s.Window > 0 && len(s.history) >= s.Window {
		s.history = s.history[1:]
	}
	s.history = append(s.history, errorMag)

	sorted := make([]float64, len(s.history))
	copy(sorted, s.history)
	sort.Float64s(sorted)

	rank := 0.0
	for i, v := range sorted {
		if errorMag <= v {
			if len(sorted) > 1 {
				rank = float64(i) / float64(len(sorted)-1)
			}
			break
		}
		if i == len(sorted)-1 {
			rank = 1.0
		}
	}
	if len(sorted) <= 1 {
		rank = 0.5
	}

	rest := s.Rest
	if rest <= 0 {
		rest = 0.01
	}
	restLog := math.Log(rest)
	targetLog := restLog

	band := s.Band
	if band <= 0 {
		band = 0.2
	}
	upper := s.Upper
	if upper <= 0 {
		upper = 0.1
	}
	lower := s.Lower
	if lower <= 0 {
		lower = 0.001
	}

	if rank > (1.0 - band) {
		targetLog = math.Log(upper)
	}

	if rank < band {
		targetLog = math.Log(lower)
	}

	gain := s.Gain
	if gain <= 0 {
		gain = 0.1
	}

	s.count++
	if s.count == 1 {
		s.ema = targetLog
	}

	if s.count > 1 {
		s.ema = gain*targetLog + (1.0-gain)*s.ema
	}

	s.out = math.Exp(s.ema)
	return nil
}

func (s *PaceServer) Done(ctx context.Context, call Pace_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
