package equation

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Price is a value at its nanosecond coordinate.
*/
type Price struct {
	At    int64
	Value float64
}

/*
LogReturn is one adjacent log-price difference on an open-left time interval.
*/
type LogReturn struct {
	From  int64
	To    int64
	Value float64
}

/*
LogReturns owns typed return storage for one path. Load replaces the previous
path; no observation from another delivery run is retained.
*/
type LogReturns struct {
	core.Base[Price, LogReturn]
	Intervals     []LogReturn
	Energy        float64
	From, Through int64
	rates         []float64
	sorted        bool
}

func NewLogReturns() *LogReturns {
	return &LogReturns{}
}

func (op *LogReturns) Next(
	in iter.Seq[core.Primitive[Price, Price]],
) iter.Seq[core.Primitive[LogReturn, LogReturn]] {
	return func(yield func(core.Primitive[LogReturn, LogReturn]) bool) {
		var observations []Price

		for arriving := range in {
			observations = append(observations, arriving.Read())
		}

		if err := op.Load(observations); err != nil {
			op.Error(err)
			return
		}

		for _, interval := range op.Intervals {
			if !yield(op.Carrier(interval)) {
				return
			}
		}
	}
}

/*
Load decodes each observation once and computes its adjacent log difference.
*/
func (op *LogReturns) Load(observations []Price) error {
	op.Intervals = op.Intervals[:0]
	op.rates = op.rates[:0]
	op.sorted = false
	op.Energy, op.From, op.Through = 0, 0, 0
	previousLog := 0.0

	for index, observation := range observations {
		logValue := math.Log(observation.Value)

		if index == 0 {
			op.From, op.Through, previousLog = observation.At, observation.At, logValue
			continue
		}

		if observation.At <= op.Through {
			return fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, observation.At, op.Through)
		}

		interval := LogReturn{From: op.Through, To: observation.At, Value: logValue - previousLog}
		op.Intervals = append(op.Intervals, interval)
		energy := interval.Value * interval.Value
		op.Energy += energy
		op.rates = append(op.rates, energy/(float64(observation.At-op.Through)/float64(time.Second)))
		op.Through, previousLog = observation.At, logValue
	}

	return nil
}

/*
MedianEnergyRate returns the central order statistic of squared returns/second.
*/
func (op *LogReturns) MedianEnergyRate() float64 {
	if len(op.rates) == 0 {
		op.Error(core.ErrNotHeld)
		return 0
	}

	if !op.sorted {
		slices.Sort(op.rates)
		op.sorted = true
	}

	count := len(op.rates)
	return (op.rates[(count-1)/2] + op.rates[count/2]) * 0.5
}
