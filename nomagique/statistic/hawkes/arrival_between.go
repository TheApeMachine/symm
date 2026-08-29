package hawkes

import "sort"

/*
between returns arrivals inside [startSec, endSec] and sets the counted
observation interval to (startSec, endSec]. An event at startSec remains
available as prehistory.
*/
func (stream arrivalStream) between(startSec, endSec float64) arrivalStream {
	buyTimesSec := betweenTimes(stream.buyTimes(), startSec, endSec)
	sellTimesSec := betweenTimes(stream.sellTimes(), startSec, endSec)

	if len(buyTimesSec) == len(stream.buyTimes()) && len(sellTimesSec) == len(stream.sellTimes()) {
		return stream.withObservationOrigin(startSec)
	}

	return newArrivalStreamFrom(startSec, buyTimesSec, sellTimesSec)
}

/*
betweenInto returns an interval stream using caller-owned reusable storage.
*/
func (stream arrivalStream) betweenInto(
	startSec, endSec float64,
	workspace *arrivalWorkspace,
) arrivalStream {
	buyTimesSec := betweenTimes(stream.buyTimes(), startSec, endSec)
	sellTimesSec := betweenTimes(stream.sellTimes(), startSec, endSec)

	if len(buyTimesSec) == len(stream.buyTimes()) && len(sellTimesSec) == len(stream.sellTimes()) {
		return stream.withObservationOrigin(startSec)
	}

	return workspace.streamFrom(startSec, buyTimesSec, sellTimesSec)
}

func betweenTimes(timesSec []float64, startSec, endSec float64) []float64 {
	first := sort.Search(len(timesSec), func(index int) bool {
		return timesSec[index] >= startSec
	})
	last := sort.Search(len(timesSec), func(index int) bool {
		return timesSec[index] > endSec
	})

	if first >= last {
		return nil
	}

	return timesSec[first:last]
}
