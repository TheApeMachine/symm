package liquidity

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Gate classifies the arrival: it reads the touch quote the feed wrote, and
stamps the measurement's support baseline. Anything invalid fails the
measurement here and never reaches the touch.
*/
type Gate struct {
	*core.PrimitiveError
	finite core.Primitive
}

func NewGate() core.Primitive {
	return &Gate{
		PrimitiveError: core.NewPrimitiveError(),
		finite:         logic.NewFinite(),
	}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			bid, ask := m.Metrics["bid"].Raw, m.Metrics["ask"].Raw
			bidQty, askQty := m.Metrics["bid_qty"].Raw, m.Metrics["ask_qty"].Raw

			m.Metadata = map[string]float64{data.MetadataSupport: 0}

			if bid == 0 || ask == 0 {
				m.Err = fmt.Errorf("%w: liquidity: ticker requires bid and ask", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			finite := true

			for _, value := range []float64{bid, ask, bidQty, askQty, bid * bidQty, ask * askQty} {
				probe := value
				var holds bool
				for out := range op.finite.Next(transport.NewOne(unsafe.Pointer(&probe)).Next(nil)) {
					holds = *(*bool)(out)
				}

				if !holds || probe <= 0 {
					finite = false
				}
			}

			if !finite {
				m.Err = fmt.Errorf("%w: liquidity: finite positive prices and displayed quantities required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if ask <= bid {
				m.Err = fmt.Errorf("%w: liquidity: positive order violated (%f <= %f)", core.ErrDomain, ask, bid)

				if !yield(arriving) {
					return
				}

				continue
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
