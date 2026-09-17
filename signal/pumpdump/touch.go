package pumpdump

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
TouchReading is one executable-touch observation.
*/
type TouchReading struct {
	Symbol                                   string
	Bid, Ask, Midpoint, Spread, Relative float64
	Baseline, Ratio, Divergence, ZScore  float64
	HasBaseline                          bool
}

type touchPath struct {
	baseline core.Primitive
	at       int64
}

/*
Touch measures spread geometry of one quoted arrival.
*/
type Touch struct {
	*core.PrimitiveError

	paths map[string]*touchPath
	out   TouchReading
}

func NewTouch() *Touch {
	return &Touch{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*touchPath),
	}
}

func (touch *Touch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			quote := *(*Quote)(arriving)

			if quote.Bid <= 0 || quote.Ask <= 0 || quote.Ask <= quote.Bid {
				continue
			}

			state := touch.paths[quote.Symbol]

			if state == nil {
				state = &touchPath{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
				touch.paths[quote.Symbol] = state
			}

			if state.at != 0 && quote.At < state.at {
				continue
			}

			midpoint := (quote.Bid + quote.Ask) / 2
			spread := quote.Ask - quote.Bid
			relative := spread / midpoint
			reading := TouchReading{
				Symbol:   quote.Symbol,
				Bid:      quote.Bid,
				Ask:      quote.Ask,
				Midpoint: midpoint,
				Spread:   spread,
				Relative: relative,
			}

			if relative > 0 {
				logged := math.Log(relative)
				var baseline adaptive.BaselineReading

				for out := range state.baseline.Next(sequence.NewOne(unsafe.Pointer(&logged)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.baseline.Error(); err != nil {
					touch.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasBaseline = true
					reading.Baseline = math.Exp(baseline.Baseline)
					reading.Ratio = relative / reading.Baseline
					reading.Divergence = logged - baseline.Baseline

					if baseline.ScoreScale > 0 {
						reading.ZScore = reading.Divergence / baseline.ScoreScale
					}
				}
			}

			state.at = quote.At
			touch.out = reading

			if !yield(unsafe.Pointer(&touch.out)) {
				return
			}
		}
	}
}
