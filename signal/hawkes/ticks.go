package hawkes

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/nomagique/hawkes"
)

const bivariateParamCount = 7

type fitEventKey struct {
	buyCount            int
	sellCount           int
	buyFirst, buyLast   int64
	sellFirst, sellLast int64
}

/*
ArrivalStreamFromTicks extracts buy and sell timestamps from ticks inside a window.
Buy maps to stream X and sell maps to stream Y in the Hawkes kernel.
*/
func ArrivalStreamFromTicks(
	ticks []market.TradeUpdate,
	windowStart, horizon time.Time,
) hawkes.ArrivalStream {
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

	return hawkes.NewArrivalStream(buyTimes, sellTimes)
}

/*
FitContextFromTicks builds an adaptive fit context and arrival stream from ticks.
*/
func FitContextFromTicks(
	ticks []market.TradeUpdate,
	windowStart, horizon time.Time,
) (hawkes.FitContext, hawkes.ArrivalStream, bool) {
	stream := ArrivalStreamFromTicks(ticks, windowStart, horizon)

	if len(stream.BuyTimes())+len(stream.SellTimes()) < 2 {
		return hawkes.FitContext{}, hawkes.ArrivalStream{}, false
	}

	probe, ok := hawkes.NewFitContext(stream, horizon)

	if !ok {
		return hawkes.FitContext{}, hawkes.ArrivalStream{}, false
	}

	adaptiveStart := horizon.Add(-probe.TradeWindow)
	stream = ArrivalStreamFromTicks(ticks, adaptiveStart, horizon)
	context, ok := hawkes.NewFitContext(stream, horizon)

	if !ok {
		return hawkes.FitContext{}, hawkes.ArrivalStream{}, false
	}

	return context, stream, true
}

func revisionKey(stream hawkes.ArrivalStream) fitEventKey {
	buyTimes := stream.BuyTimes()
	sellTimes := stream.SellTimes()
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
