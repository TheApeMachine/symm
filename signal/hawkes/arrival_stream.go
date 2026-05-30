package hawkes

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/decay"
	"github.com/theapemachine/symm/numeric/timeline"
)

type eventSide int

const (
	sideBuy eventSide = iota
	sideSell
)

type fitEventKey struct {
	buyCount            int
	sellCount           int
	buyFirst, buyLast   int64
	sellFirst, sellLast int64
}

/*
markedEvent is one trade arrival tagged by aggressor side.
*/
type markedEvent struct {
	at   time.Time
	side eventSide
}

/*
ArrivalStream holds sorted buy and sell timelines inside one measurement window.
*/
type ArrivalStream struct {
	buy  timeline.Timeline
	sell timeline.Timeline
}

/*
NewArrivalStream copies and sorts both sides of an arrival window.
*/
func NewArrivalStream(buyTimes, sellTimes []time.Time) ArrivalStream {
	return ArrivalStream{
		buy:  timeline.New(buyTimes),
		sell: timeline.New(sellTimes),
	}
}

/*
ArrivalStreamFromTicks extracts buy and sell timestamps from ticks inside a window.
*/
func ArrivalStreamFromTicks(
	ticks []market.TradeUpdate,
	windowStart, horizon time.Time,
) ArrivalStream {
	buyTimes := make([]time.Time, 0, len(ticks))
	sellTimes := make([]time.Time, 0, len(ticks))

	for _, tick := range ticks {
		if tick.Timestamp.Before(windowStart) {
			continue
		}

		if tick.Timestamp.After(horizon) {
			continue
		}

		switch tick.Side {
		case "buy":
			buyTimes = append(buyTimes, tick.Timestamp)
		case "sell":
			sellTimes = append(sellTimes, tick.Timestamp)
		}
	}

	return NewArrivalStream(buyTimes, sellTimes)
}

func (stream ArrivalStream) BuyTimes() []time.Time {
	return stream.buy.Times()
}

func (stream ArrivalStream) SellTimes() []time.Time {
	return stream.sell.Times()
}

func (stream ArrivalStream) RevisionKey() fitEventKey {
	buyTimes := stream.buy.Times()
	sellTimes := stream.sell.Times()
	key := fitEventKey{
		buyCount:  len(buyTimes),
		sellCount: len(sellTimes),
	}

	if len(buyTimes) > 0 {
		key.buyFirst = buyTimes[0].UnixNano()
		key.buyLast = buyTimes[len(buyTimes)-1].UnixNano()
	}

	if len(sellTimes) > 0 {
		key.sellFirst = sellTimes[0].UnixNano()
		key.sellLast = sellTimes[len(sellTimes)-1].UnixNano()
	}

	return key
}

func (stream ArrivalStream) Marked() []markedEvent {
	buyTimes := stream.buy.Times()
	sellTimes := stream.sell.Times()
	marked := make([]markedEvent, 0, len(buyTimes)+len(sellTimes))
	buyIndex := 0
	sellIndex := 0

	for buyIndex < len(buyTimes) && sellIndex < len(sellTimes) {
		if !buyTimes[buyIndex].After(sellTimes[sellIndex]) {
			marked = append(marked, markedEvent{at: buyTimes[buyIndex], side: sideBuy})
			buyIndex++
			continue
		}

		marked = append(marked, markedEvent{at: sellTimes[sellIndex], side: sideSell})
		sellIndex++
	}

	for buyIndex < len(buyTimes) {
		marked = append(marked, markedEvent{at: buyTimes[buyIndex], side: sideBuy})
		buyIndex++
	}

	for sellIndex < len(sellTimes) {
		marked = append(marked, markedEvent{at: sellTimes[sellIndex], side: sideSell})
		sellIndex++
	}

	return marked
}

func (stream ArrivalStream) markedTimeline() timeline.Timeline {
	marked := stream.Marked()
	times := make([]time.Time, len(marked))

	for index, event := range marked {
		times[index] = event.at
	}

	return timeline.New(times)
}

func (stream ArrivalStream) Gaps() []float64 {
	return stream.markedTimeline().Gaps()
}

func (stream ArrivalStream) Span(horizon time.Time) float64 {
	return stream.markedTimeline().Span(horizon)
}

func (stream ArrivalStream) buyIntensityAt(
	horizon time.Time,
	muBuy, alphaBB, alphaBS, beta float64,
) float64 {
	return decay.IntensityAt(
		stream.buy, stream.sell, horizon,
		muBuy, alphaBB, alphaBS, beta,
	)
}

func (stream ArrivalStream) sellIntensityAt(
	horizon time.Time,
	muSell, alphaSB, alphaSS, beta float64,
) float64 {
	return decay.IntensityAt(
		stream.buy, stream.sell, horizon,
		muSell, alphaSB, alphaSS, beta,
	)
}

func (stream ArrivalStream) intensityAt(
	at time.Time,
	mu, alphaFromBuy, alphaFromSell, beta float64,
) float64 {
	return decay.IntensityAt(
		stream.buy, stream.sell, at,
		mu, alphaFromBuy, alphaFromSell, beta,
	)
}

func (stream ArrivalStream) kernelSupport(horizon time.Time, beta float64) (buy, sell float64) {
	return decay.KernelSupport(stream.buy, horizon, beta),
		decay.KernelSupport(stream.sell, horizon, beta)
}
