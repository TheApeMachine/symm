package cognition

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type ClassificationServer struct {
	totalMass float64
	list      []classificationItem
	MinCont   float64
}

type classificationItem struct {
	name    []byte
	prob    float64
	support uint64
}

func NewClassification() *ClassificationServer {
	return &ClassificationServer{}
}

func (s *ClassificationServer) Write(ctx context.Context, call Classification_write) error {
	class, _ := call.Args().Class()
	prob := call.Args().Prob()
	support := call.Args().Support()

	s.totalMass += prob
	s.list = append(s.list, classificationItem{name: class, prob: prob, support: support})
	return nil
}

func (s *ClassificationServer) Done(ctx context.Context, call Classification_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if s.totalMass > 0 && len(s.list) > 0 {
		for i := range s.list {
			s.list[i].prob /= s.totalMass
		}

		winnerIdx := 0
		maxProb := s.list[0].prob
		for i := 1; i < len(s.list); i++ {
			if s.list[i].prob > maxProb {
				maxProb = s.list[i].prob
				winnerIdx = i
			}
		}

		winner := s.list[winnerIdx]
		var runnerUp []byte
		contrast := 0.0
		highestOther := -1.0
		for i, c := range s.list {
			if i != winnerIdx && c.prob > highestOther {
				highestOther = c.prob
				runnerUp = c.name
			}
		}

		if runnerUp != nil && highestOther > 0 {
			contrast = math.Log2(winner.prob / highestOther)
		}

		passed := true
		if s.MinCont > 0 && contrast < s.MinCont {
			passed = false
		}

		_ = results.SetWinner(winner.name)
		_ = results.SetRunnerUp(runnerUp)
		results.SetProb(winner.prob)
		results.SetContrast(contrast)
		results.SetSupport(winner.support)
		results.SetPassed(passed)
	}

	s.list = nil
	s.totalMass = 0
	return nil
}
