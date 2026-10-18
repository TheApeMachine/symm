package hindsight

import (
	"fmt"
	"slices"

	"github.com/theapemachine/errnie"
)

/*
annotationTrade is one print of the symbol inside the marked window.
*/
type annotationTrade struct {
	tick     uint64
	at       int64
	price    float64
	bid, ask float64
}

/*
annotationVector is the excursion a hand-placed A B C D stores. B and C are
the last trade of their tick, which is the price the tape draws. The trade
indexes count from the first trade of A, so a later read of the window
lands C on c_idx.
*/
func annotationVector(class string, marks [4]uint64, trades []annotationTrade) ([]float64, error) {
	if !slices.Contains(excursionClasses, class) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf("[hindsight] %q is not an excursion class", class), nil)
	}

	if !(marks[0] <= marks[1] && marks[1] <= marks[2] && marks[2] <= marks[3]) {
		return nil, errnie.Err(errnie.Validation, "[hindsight] A B C D must be in tape order", nil)
	}

	indexOf := func(tick uint64, last bool) (int, error) {
		found := -1

		for idx, trade := range trades {
			if trade.tick != tick {
				continue
			}

			found = idx

			if !last {
				break
			}
		}

		if found < 0 {
			return 0, errnie.Err(errnie.NotFound, fmt.Sprintf("[hindsight] tick %d has no trade to mark", tick), nil)
		}

		return found, nil
	}

	a, err := indexOf(marks[0], false)
	if err != nil {
		return nil, err
	}

	b, err := indexOf(marks[1], true)
	if err != nil {
		return nil, err
	}

	c, err := indexOf(marks[2], true)
	if err != nil {
		return nil, err
	}

	d, err := indexOf(marks[3], true)
	if err != nil {
		return nil, err
	}

	touch := func(price, side float64) float64 {
		if side > 0 {
			return side
		}

		return price
	}

	bb, cc := trades[b], trades[c]

	return []float64{
		float64(a), float64(marks[0]), float64(b), float64(marks[1]),
		float64(c), float64(marks[2]), float64(d), float64(marks[3]),
		bb.price, cc.price, touch(bb.price, bb.ask), touch(cc.price, cc.bid),
		float64(bb.at), float64(cc.at),
		float64(b), float64(marks[1]), float64(c), float64(marks[2]),
		touch(bb.price, bb.ask), touch(cc.price, cc.bid), float64(bb.at), float64(cc.at),
	}, nil
}

/*
markedTrades is the symbol's prints from tick from through tick to.
*/
func markedTrades(tape *runTape, symbol string, from, to uint64) []annotationTrade {
	if tape == nil {
		return nil
	}

	trades := make([]annotationTrade, 0)

	for _, row := range tape.bySymbol[symbol] {
		if row.frame == nil || row.tick == nil || row.frame.Tick < from || row.frame.Tick > to {
			continue
		}

		bid, ask := 0.0, 0.0

		if row.tick.Price.Bid != nil {
			bid = row.tick.Price.Bid.Float64()
		}

		if row.tick.Price.Ask != nil {
			ask = row.tick.Price.Ask.Float64()
		}

		for _, trade := range row.tick.Trades {
			trades = append(trades, annotationTrade{
				tick:  row.frame.Tick,
				at:    trade.Timestamp.UnixNano(),
				price: trade.Price.Float64(),
				bid:   bid,
				ask:   ask,
			})
		}
	}

	return trades
}
