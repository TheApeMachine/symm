package equation

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* LogReturn is one adjacent log-price difference on an open-left time interval. */
type LogReturn struct {
	From, To int64
	Value    float64
}

/*
LogReturns owns reusable typed return storage for one path evaluation. Load
replaces the previous path; no observation from another delivery run is retained.
*/
type LogReturns struct {
	Intervals     []LogReturn
	Energy        float64
	From, Through int64
	rates         []float64
	sorted        bool
}

/* Load decodes each observation once and computes its adjacent log difference. */
func (returns *LogReturns) Load(observations []core.Primitive) error {
	returns.Intervals = returns.Intervals[:0]
	returns.rates = returns.rates[:0]
	returns.sorted = false
	returns.Energy, returns.From, returns.Through = 0, 0, 0
	previousLog := 0.0

	for index, observation := range observations {
		fields := core.To[map[string]core.Primitive](observation)

		if err := observation.Error(); err != nil {
			return err
		}
		decoder := core.NewDecoder(fields)
		at := core.Decode[int64](decoder, "at")
		value := core.Decode[float64](decoder, "value")

		if err := decoder.Error(); err != nil {
			return err
		}
		logValue := math.Log(value)

		if index == 0 {
			returns.From, returns.Through, previousLog = at, at, logValue
			continue
		}

		if at <= returns.Through {
			return fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, at, returns.Through)
		}
		interval := LogReturn{From: returns.Through, To: at, Value: logValue - previousLog}
		returns.Intervals = append(returns.Intervals, interval)
		energy := interval.Value * interval.Value
		returns.Energy += energy
		returns.rates = append(returns.rates, energy/(float64(at-returns.Through)/float64(time.Second)))
		returns.Through, previousLog = at, logValue
	}
	return nil
}

/* MedianEnergyRate returns the central order statistic of squared returns/second. */
func (returns *LogReturns) MedianEnergyRate() float64 {
	if len(returns.rates) == 0 {
		return math.NaN()
	}
	if !returns.sorted {
		slices.Sort(returns.rates)
		returns.sorted = true
	}
	// Match the generic median's undefined result when any rate is undefined.
	if math.IsNaN(returns.rates[0]) {
		return returns.rates[0]
	}
	return (returns.rates[(len(returns.rates)-1)/2] + returns.rates[len(returns.rates)/2]) * 0.5
}

/*
NewLogReturns emits the existing {from,to,value} records from typed differences.
The adjacent-pair width is two observations; it is not a retained time horizon.
*/
func NewLogReturns() core.Primitive {
	return transport.NewPipe(
		transport.NewCollect[core.Primitive](),
		transport.NewMap(&logReturnStep{seed: transport.NewIO(core.From([]core.Primitive{}))}),
		transport.NewSpread[core.Primitive](),
	)
}

/* logReturnStep owns the Primitive delivery boundary for a complete price path. */
type logReturnStep struct {
	core.PrimitiveError
	returns LogReturns
	seed    *transport.IO
	current core.Primitive
}

func (step *logReturnStep) Next(input core.Primitive) core.Primitive {
	result := core.Yield(step.seed, input,
		func(_ []core.Primitive, observations []core.Primitive) []core.Primitive {
			if err := step.returns.Load(observations); err != nil {
				step.Error(err)
				return nil
			}
			intervals := make([]core.Primitive, len(step.returns.Intervals))

			for index, interval := range step.returns.Intervals {
				intervals[index] = core.Record(map[string]any{
					"from": interval.From, "to": interval.To, "value": interval.Value,
				})
			}
			return intervals
		}, step)

	if result != nil {
		step.current = result
	}
	return result
}

func (step *logReturnStep) Read() any { return core.To[any](step.current) }
