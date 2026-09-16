package leadlag

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	correlation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
PathStep owns one price path per symbol label. Each arriving measurement's
last_price is fed to the label's Path primitive, and the observation count
is stamped. Arrivals whose timestamp is rejected by the path are dropped.

Retained path readings are stored so downstream stages (PairSearch) can
access them via the same reference.
*/
type PathStep struct {
	*core.PrimitiveError
	paths    map[string]core.Primitive
	Retained map[string]correlation.PathReading
	window   func() *adaptive.Window
}

func NewPathStep(window func() *adaptive.Window) *PathStep {
	return &PathStep{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]core.Primitive),
		Retained:       make(map[string]correlation.PathReading),
		window:         window,
	}
}

func (pathStep *PathStep) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			path := pathStep.paths[m.Label]

			if path == nil {
				path = correlation.NewPath(pathStep.window())
				pathStep.paths[m.Label] = path
			}

			price := temporal.Price{At: m.At.UnixNano(), Value: m.Metrics["last_price"].Raw}
			var focal correlation.PathReading

			for out := range path.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*correlation.PathReading)(out)
			}

			if err := path.Error(); err != nil {
				pathStep.Error(err)
				continue
			}

			m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(focal.Count)

			if !focal.Accepted {
				continue
			}

			m.From = time.Unix(0, focal.From)
			pathStep.Retained[m.Label] = focal

			if !yield(arriving) {
				return
			}
		}
	}
}
