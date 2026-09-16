package hawkes

import "sort"

/*
eventSide identifies which of the two coupled streams an arrival belongs to.
*/
type eventSide int

const (
	sideBuy eventSide = iota
	sideSell
)

/*
markedEvent is one arrival tagged by stream side, timestamped in seconds
since an arbitrary but consistent epoch (the caller's Frame carries whichever
epoch it likes; only relative differences matter to every computation here).
*/
type markedEvent struct {
	atSec float64
	side  eventSide
}

/*
arrivalStream holds sorted buy and sell timestamps (seconds) inside one
measurement window, plus their chronological merge and inter-arrival gaps.
originSec is the last point before which arrivals are prehistory (excitation
only, not counted observations): counted arrivals follow (originSec, horizon].
*/
type arrivalStream struct {
	originSec float64
	buy       []float64
	sell      []float64
	marked    []markedEvent
	gaps      []float64
}

/*
newArrivalStream sorts both arrival timestamp slices, merges them
chronologically, and sets the observation origin to the first marked event.
*/
func newArrivalStream(buyTimesSec, sellTimesSec []float64) arrivalStream {
	stream := arrivalStream{
		buy:  sortedCopy(buyTimesSec),
		sell: sortedCopy(sellTimesSec),
	}
	stream.marked = stream.merge()
	stream.gaps = gapsFromMarked(stream.marked)

	if len(stream.marked) > 0 {
		stream.originSec = stream.marked[0].atSec
	}

	return stream
}

/*
newArrivalStreamFrom constructs a stream with an explicit observation origin.
Events at or before origin are retained as excitation prehistory but are not
counted observations.
*/
func newArrivalStreamFrom(originSec float64, buyTimesSec, sellTimesSec []float64) arrivalStream {
	stream := newArrivalStream(buyTimesSec, sellTimesSec)
	stream.originSec = originSec

	return stream
}

func sortedCopy(times []float64) []float64 {
	if len(times) < 2 {
		return times
	}

	sorted := append([]float64(nil), times...)
	sort.Float64s(sorted)

	return sorted
}

func (arrivalStream arrivalStream) merge() []markedEvent {
	return arrivalStream.mergeInto(make([]markedEvent, 0, len(arrivalStream.buy)+len(arrivalStream.sell)))
}

/*
mergeInto chronologically merges buy/sell timestamps into caller-owned
storage, avoiding an allocation when workspace reuse is available.
*/
func (arrivalStream arrivalStream) mergeInto(marked []markedEvent) []markedEvent {
	buyIndex, sellIndex := 0, 0

	for buyIndex < len(arrivalStream.buy) && sellIndex < len(arrivalStream.sell) {
		if arrivalStream.buy[buyIndex] <= arrivalStream.sell[sellIndex] {
			marked = append(marked, markedEvent{atSec: arrivalStream.buy[buyIndex], side: sideBuy})
			buyIndex++
			continue
		}

		marked = append(marked, markedEvent{atSec: arrivalStream.sell[sellIndex], side: sideSell})
		sellIndex++
	}

	for ; buyIndex < len(arrivalStream.buy); buyIndex++ {
		marked = append(marked, markedEvent{atSec: arrivalStream.buy[buyIndex], side: sideBuy})
	}

	for ; sellIndex < len(arrivalStream.sell); sellIndex++ {
		marked = append(marked, markedEvent{atSec: arrivalStream.sell[sellIndex], side: sideSell})
	}

	return marked
}

func gapsFromMarked(marked []markedEvent) []float64 {
	if len(marked) < 2 {
		return nil
	}

	gaps := make([]float64, 0, len(marked)-1)

	for index := 1; index < len(marked); index++ {
		gap := marked[index].atSec - marked[index-1].atSec

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

/*
span returns exposure seconds on the common interval (origin, horizon].
*/
func (arrivalStream arrivalStream) span(horizonSec float64) float64 {
	if horizonSec <= arrivalStream.originSec {
		return 0
	}

	return horizonSec - arrivalStream.originSec
}

/*
observationCounts returns side counts on the common interval (origin, horizon].
*/
func (arrivalStream arrivalStream) observationCounts(horizonSec float64) (buy, sell int) {
	return observationCount(arrivalStream.buy, arrivalStream.originSec, horizonSec),
		observationCount(arrivalStream.sell, arrivalStream.originSec, horizonSec)
}

func observationCount(times []float64, originSec, horizonSec float64) int {
	count := 0

	for _, eventTime := range times {
		if eventTime <= originSec {
			continue
		}

		if eventTime > horizonSec {
			break
		}

		count++
	}

	return count
}

/*
observationMarked returns marked events strictly after origin and at or
before horizon, in chronological order.
*/
func (arrivalStream arrivalStream) observationMarked(horizonSec float64) []markedEvent {
	marked := make([]markedEvent, 0, len(arrivalStream.marked))

	for _, event := range arrivalStream.marked {
		if event.atSec <= arrivalStream.originSec {
			continue
		}

		if event.atSec > horizonSec {
			break
		}

		marked = append(marked, event)
	}

	return marked
}

/*
buyIntensityAt evaluates the fitted buy-side conditional intensity at horizon.
*/
func (arrivalStream arrivalStream) buyIntensityAt(
	horizonSec float64, muBuy, alphaBB, alphaBS, beta float64,
) float64 {
	return intensityAt(arrivalStream.buy, arrivalStream.sell, horizonSec, muBuy, alphaBB, alphaBS, beta)
}

/*
sellIntensityAt evaluates the fitted sell-side conditional intensity at
horizon.
*/
func (arrivalStream arrivalStream) sellIntensityAt(
	horizonSec float64, muSell, alphaSB, alphaSS, beta float64,
) float64 {
	return intensityAt(arrivalStream.buy, arrivalStream.sell, horizonSec, muSell, alphaSB, alphaSS, beta)
}

/*
kernelIntegralSupport returns the per-side closed-form kernel integral used
by the compensator.
*/
func (arrivalStream arrivalStream) kernelIntegralSupport(horizonSec, beta float64) (buy, sell float64) {
	return observationKernelIntegralSupport(arrivalStream.buy, arrivalStream.originSec, horizonSec, beta),
		observationKernelIntegralSupport(arrivalStream.sell, arrivalStream.originSec, horizonSec, beta)
}
