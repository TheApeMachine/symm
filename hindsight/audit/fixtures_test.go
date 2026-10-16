package audit

import (
	"strings"

	"github.com/theapemachine/symm/data"
)

/*
fixtureSignal is one raw signal value of a fixture tick.
*/
type fixtureSignal struct {
	source string
	label  string
	raw    float64
}

/*
signalJoin builds the join TickViews would hand the grid stages for symbol
at tick: raw signal layers, plus the touch when bid and ask are given.
*/
func signalJoin(tick uint64, symbol string, signals []fixtureSignal, bidAsk ...float64) *data.Value {
	join := data.NewValue(data.TickOrigin, "runtime:join", symbol)
	join.Epoch, join.Tick = 1, tick
	join.FormatPrefix()

	for _, signal := range signals {
		data.AppendSignalVector(join, signal.source, symbol, []float64{signal.raw}, signal.label)
	}

	if len(bidAsk) == 2 {
		data.AppendVector(join, bidAsk, "best_price:bid", "best_price:ask", touchTag)
	}

	return join
}

/*
oscillating is 1 on odd ticks and 3 on even ones (phase flips it): its SNM
and energy change every tick, so its regions light. A constant source has
SNM 0 and never lights.
*/
func oscillating(tick uint64, phase bool) float64 {
	if (tick%2 == 1) != phase {
		return 1
	}

	return 3
}

const constantRaw = 5.0

/*
regimeJoin lights cvd (R01) when cvdLit, else liquidity (R04); the other
source is held constant.
*/
func regimeJoin(tick uint64, cvdLit bool) *data.Value {
	cvd, liquidity := constantRaw, oscillating(tick, false)

	if cvdLit {
		cvd, liquidity = oscillating(tick, false), constantRaw
	}

	return signalJoin(tick, "BTC/USD", []fixtureSignal{
		{"cvd", "signed_net_fraction", cvd},
		{"liquidity", "relative_spread", liquidity},
	})
}

/*
excursionValue is a detector excursion Value carrying every ExcursionKeys
field, overridden by fields.
*/
func excursionValue(class, symbol string, fields map[string]float64) *data.Value {
	value := data.NewValue(data.ExcursionOrigin, class, symbol)
	values := make([]float64, len(data.ExcursionKeys))

	for idx, key := range data.ExcursionKeys {
		values[idx] = fields[key]
	}

	data.AppendVector(value, values, data.ExcursionKeys...)
	return value
}

func hasViolation(violations []*Violation, name string) bool {
	for _, violation := range violations {
		if violation.Name == name {
			return true
		}
	}

	return false
}

func hasInsufficient(violations []*Violation) bool {
	for _, violation := range violations {
		if strings.HasSuffix(violation.Name, ".insufficient_evidence") {
			return true
		}
	}

	return false
}
