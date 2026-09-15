package leadlag

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
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
	Retained map[string]nmcorrelation.PathReading
	window   func() core.Primitive
}

func NewPathStep(window func() core.Primitive) *PathStep {
	return &PathStep{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]core.Primitive),
		Retained:       make(map[string]nmcorrelation.PathReading),
		window:         window,
	}
}

func (op *PathStep) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			path := op.paths[m.Label]

			if path == nil {
				path = nmcorrelation.NewPath(op.window())
				op.paths[m.Label] = path
			}

			price := temporal.Price{At: m.At.UnixNano(), Value: m.Metrics["last_price"].Raw}
			var focal nmcorrelation.PathReading

			for out := range path.Next(transport.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*nmcorrelation.PathReading)(out)
			}

			if err := path.Error(); err != nil {
				op.Error(err)
				continue
			}

			m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(focal.Count)

			if !focal.Accepted {
				continue
			}

			m.From = time.Unix(0, focal.From)
			op.Retained[m.Label] = focal

			if !yield(arriving) {
				return
			}
		}
	}
}
