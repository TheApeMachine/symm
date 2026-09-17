package correlation

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/logic"
)


/*
Gate classifies the arrival: it reads the last price the feed wrote, consumes
that metric, and stamps the measurement's support baseline. Anything invalid
fails the measurement here and never reaches the path. The last price and the
metadata baseline are rewritten on every arrival, so the measurement carries
this arrival's facts, never the prior one's.
*/
type Gate struct {
	*core.PrimitiveError

	finite core.Primitive
}

func NewGate() *Gate {
	return &Gate{PrimitiveError: core.NewPrimitiveError(), finite: logic.NewFinite()}
}

func (gate *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			metric, traded := m.Metrics["last_price"]

			if !traded {
				m.Err = fmt.Errorf("%w: correlation: ticker requires a last price", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			last := metric.Raw

			if m.Metadata == nil {
				m.Metadata = make(map[string]string, 1)
			}

			m.Metadata[data.MetadataSupport] = "0"

			var holds bool
			for out := range gate.finite.Next(sequence.NewOne(unsafe.Pointer(&last)).Next(nil)) {
				holds = *(*bool)(out)
			}

			if !holds || last < 0 {
				m.Err = fmt.Errorf("%w: correlation: finite non-negative last price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["last_price"] = metric.Write(last)

			if last == 0 {
				m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
