package probability

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type DistributionServer struct {
	Downstream func(context.Context, NativeReading) error
}

func (s *DistributionServer) Write(ctx context.Context, call Distribution_write) error {
	inList, err := call.Args().In()
	if err != nil {
		return err
	}

	var probabilities []float64
	for i := 0; i < inList.Len(); i++ {
		probabilities = append(probabilities, inList.At(i))
	}

	if len(probabilities) == 0 {
		return nil
	}

	best := argmax(probabilities)
	amb := ambiguity(probabilities)

	result := NativeReading{
		Probabilities: probabilities,
		Winner:        best.Index,
		Confidence:    best.Value,
		Ambiguity:     amb,
		Sharpness:     1 - amb,
	}

	if s.Downstream != nil {
		return s.Downstream(ctx, result)
	}
	return nil
}

func argmax(vals []float64) ArgmaxResult {
	if len(vals) == 0 {
		return ArgmaxResult{}
	}
	best := ArgmaxResult{Index: 0, Value: vals[0]}
	for index := 1; index < len(vals); index++ {
		if vals[index] > best.Value {
			best = ArgmaxResult{Index: index, Value: vals[index]}
		}
	}
	return best
}

func ambiguity(vals []float64) float64 {
	if len(vals) <= 1 {
		return 0
	}
	var total float64
	for _, val := range vals {
		total += val
	}
	if total == 0 {
		return 0
	}
	entropy := 0.0
	for _, val := range vals {
		p := val / total
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}
	return entropy / math.Log(float64(len(vals)))
}

type NativeReading struct {
	Probabilities []float64
	Winner        int
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
}

type ArgmaxResult struct {
	Index int
	Value float64
}

func (s *DistributionServer) Done(ctx context.Context, call Distribution_done) error {
	return nil
}

type DistributionNode types.StreamNode[any, any]

func NewDistribution() DistributionNode {
	server := &DistributionServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {
			server.Downstream = func(c context.Context, p NativeReading) error {
				return next(c, p)
			}
		},
	)
}
