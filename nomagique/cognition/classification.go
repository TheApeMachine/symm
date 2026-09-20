package cognition

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type ClassificationServer struct {
	Downstream func(context.Context, []byte, []byte, float64, float64, uint64, bool) error
	totalMass  float64
	list       []struct {
		name    []byte
		prob    float64
		support uint64
	}
	MinCont float64
}

func (s *ClassificationServer) Write(ctx context.Context, call Classification_write) error {
	class, _ := call.Args().Class()
	prob := call.Args().Prob()
	support := call.Args().Support()
	s.totalMass += prob
	s.list = append(s.list, struct {
		name    []byte
		prob    float64
		support uint64
	}{class, prob, support})
	return nil
}

func (s *ClassificationServer) Done(ctx context.Context, call Classification_done) error {
	// Process classification when stream is done
	if s.totalMass <= 0 || len(s.list) == 0 {
		return nil
	}
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
	// Clear state
	s.list = nil
	s.totalMass = 0
	return s.Downstream(ctx, winner.name, runnerUp, winner.prob, contrast, winner.support, passed)
}



type ClassificationNode types.StreamNode[any, any]

func NewClassification() ClassificationNode {
	server := &ClassificationServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
