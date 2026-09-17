package pumpdump

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
ClockReading is one volume-clock observation.
*/
type ClockReading struct {
	Symbol                                       string
	Price, Qty, Notional                         float64
	Interval                                     float64
	HasInterval                                  bool
	Target, BarQty, BarNotional, BarCount        float64
	Duration                                     float64
	HasTarget                                    bool
	VolumeRate, NotionalRate, TradeRate          float64
	HasRates                                     bool
	Completed                                    float64
	NotionalBaseline, NotionalRatio, NotionalDiv float64
	NotionalZ                                    float64
	HasNotionalBaseline                          bool
}

type clockPath struct {
	qtys        []float64
	median      core.Primitive
	baseline    core.Primitive
	open        bool
	start       int64
	prev        int64
	hasPrev     bool
	target      float64
	hasTarget   bool
	barQty      float64
	barNotional float64
	barCount    float64
	completed   float64
}

/*
Clock accumulates trades onto an adaptive volume clock.
*/
type Clock struct {
	*core.PrimitiveError

	paths map[string]*clockPath
	out   ClockReading
}

func NewClock() *Clock {
	return &Clock{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*clockPath),
	}
}

func (clock *Clock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fill := *(*Fill)(arriving)

			if fill.Price <= 0 || fill.Qty <= 0 {
				continue
			}

			state := clock.paths[fill.Symbol]

			if state == nil {
				state = &clockPath{
					median:   statistic.NewMedian(),
					baseline: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				clock.paths[fill.Symbol] = state
			}

			if state.hasPrev && fill.At < state.prev {
				continue
			}

			reading := ClockReading{
				Symbol:   fill.Symbol,
				Price:    fill.Price,
				Qty:      fill.Qty,
				Notional: fill.Price * fill.Qty,
			}

			if state.hasPrev {
				delta := float64(fill.At-state.prev) / 1e9

				if delta > 0 {
					reading.Interval = delta
					reading.HasInterval = true
				}
			}

			if !state.open {
				if len(state.qtys) > 0 {
					var target float64

					for out := range state.median.Next(sequence.NewValues(state.qtys...).Next(nil)) {
						target = *(*float64)(out)
					}

					if err := state.median.Error(); err != nil {
						clock.Error(err)
						return
					}

					state.target = target
					state.hasTarget = true
				}

				state.start = fill.At
				state.open = true
			}

			state.qtys = append(state.qtys, fill.Qty)
			state.barQty += fill.Qty
			state.barNotional += reading.Notional
			state.barCount++
			reading.BarQty = state.barQty
			reading.BarNotional = state.barNotional
			reading.BarCount = state.barCount
			reading.HasTarget = state.hasTarget
			reading.Target = state.target

			if state.start != 0 && fill.At >= state.start {
				reading.Duration = float64(fill.At-state.start) / 1e9
			}

			complete := false

			if reading.Duration > 0 {
				if !state.hasTarget {
					complete = true
				}

				if state.hasTarget && state.barQty >= state.target {
					complete = true
				}
			}

			if complete {
				state.completed++
				reading.Completed = state.completed
				reading.VolumeRate = state.barQty / reading.Duration
				reading.NotionalRate = state.barNotional / reading.Duration
				reading.TradeRate = state.barCount / reading.Duration
				reading.HasRates = true
				state.open = false
				state.barQty = 0
				state.barNotional = 0
				state.barCount = 0
			}

			if complete && state.hasTarget && reading.NotionalRate > 0 {
				logged := math.Log(reading.NotionalRate)
				var baseline adaptive.BaselineReading

				for out := range state.baseline.Next(sequence.NewOne(unsafe.Pointer(&logged)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.baseline.Error(); err != nil {
					clock.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasNotionalBaseline = true
					reading.NotionalBaseline = math.Exp(baseline.Baseline)
					reading.NotionalRatio = reading.NotionalRate / reading.NotionalBaseline
					reading.NotionalDiv = logged - baseline.Baseline
				}

				if baseline.HasPrior && baseline.ScoreScale > 0 {
					reading.NotionalZ = reading.NotionalDiv / baseline.ScoreScale
				}
			}

			state.prev = fill.At
			state.hasPrev = true
			clock.out = reading

			if !yield(unsafe.Pointer(&clock.out)) {
				return
			}
		}
	}
}
