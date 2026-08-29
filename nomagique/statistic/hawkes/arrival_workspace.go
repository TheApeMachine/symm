package hawkes

import "slices"

/*
arrivalWorkspace builds ephemeral streams while reusing merged-event storage.
A returned stream remains valid until the next call to stream/streamFrom.
*/
type arrivalWorkspace struct {
	marked []markedEvent
	gaps   gapSummary
}

/*
newArrivalWorkspace returns an empty reusable stream workspace.
*/
func newArrivalWorkspace() *arrivalWorkspace {
	return &arrivalWorkspace{}
}

/*
stream builds one sorted arrival view over the caller-provided timestamps.
*/
func (workspace *arrivalWorkspace) stream(buyTimesSec, sellTimesSec []float64) arrivalStream {
	result := workspace.streamFrom(0, buyTimesSec, sellTimesSec)

	if len(result.marked) > 0 {
		result.originSec = result.marked[0].atSec
	}

	return result
}

/*
streamFrom builds one sorted arrival view with an explicit observation
origin, reusing the workspace's backing arrays instead of allocating.
*/
func (workspace *arrivalWorkspace) streamFrom(
	originSec float64,
	buyTimesSec, sellTimesSec []float64,
) arrivalStream {
	required := len(buyTimesSec) + len(sellTimesSec)
	workspace.marked = slices.Grow(workspace.marked[:0], required)
	workspace.gaps.sorted = slices.Grow(workspace.gaps.sorted[:0], required)

	stream := arrivalStream{
		originSec: originSec,
		buy:       sortedCopy(buyTimesSec),
		sell:      sortedCopy(sellTimesSec),
	}
	workspace.marked = stream.mergeInto(workspace.marked)
	workspace.gaps.reset(workspace.marked)
	stream.marked = workspace.marked
	stream.gaps = workspace.gaps.sorted

	return stream
}
