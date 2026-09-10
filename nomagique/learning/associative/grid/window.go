package grid

import "math"

/*
window is the rolling feature matrix the affinity channels are measured over:
quantities against recent observation bins, with absence retained as absence.

A bin exists because quantities do not arrive together. Producers are driven by
different raw frames — trade-driven, ticker-driven and book-driven quantities
are published on separate updates and never share one — so a matrix whose
columns are single updates has permanently disjoint support per producer, and
any affinity read from it measures the transport rather than the market. A bin
gathers one observation of each quantity that is still publishing, so
quantities driven by different frames become comparable without any of them
being asserted to have moved when it did not.

Slots hold the movement observed in that bin, and present says whether the
quantity was observed there at all. An unobserved slot is absent, never zero:
"not published in this bin" and "published and did not move" are opposite
readings and must not share a representation (§43).
*/
type window struct {
	slots    [][]float64
	present  [][]bool
	standard [][]float64
	varies   []bool
	dirty    []bool
	pending  []int
	open     []float64
	observed []bool
	// membership is what the open bin must reach before it closes: the
	// quantities the previous bin saw, less those that have since gone silent.
	membership []bool
	silence    []int
	first      int
	count      int
	capacity   int
}

/*
newWindow retains capacity bins. The capacity is a declared span of recent
history, not a derived one: how much of the past still describes the present is
a modelling choice, and it travels with the readings it produced rather than
hiding inside them.
*/
func newWindow(capacity int) *window {
	return &window{capacity: max(capacity, 2)}
}

/* columns extends every retained bin when the grid admits a new quantity. */
func (retained *window) columns(count int) {
	for len(retained.open) < count {
		retained.open = append(retained.open, 0)
		retained.observed = append(retained.observed, false)
		retained.membership = append(retained.membership, false)
		retained.silence = append(retained.silence, 0)

		retained.varies = append(retained.varies, false)
		retained.dirty = append(retained.dirty, false)

		for bin := range retained.slots {
			retained.slots[bin] = append(retained.slots[bin], 0)
			retained.present[bin] = append(retained.present[bin], false)
			retained.standard[bin] = append(retained.standard[bin], 0)
		}
		retained.soil(len(retained.open) - 1)
	}
}

/*
observe records one quantity's movement in the open bin.

A bin holds at most one reading of each quantity, so a quantity arriving a
second time belongs to the next bin, not to this one: the bin closes first and
the new reading opens its successor. Overwriting in place instead would put two
observations taken at different moments into one bin, and the later one would
be read against its neighbours' earlier ones — a whole rotation out of step.
*/
func (retained *window) observe(column int, movement float64) {
	if retained.observed[column] {
		retained.close()
	}
	retained.open[column] = movement
	retained.observed[column] = true
}

/*
covered reports that the open bin now holds every quantity the previous bin
held, so it spans one full rotation of whatever is currently publishing.

Closing on the first repeated quantity alone would size the bin to the FASTEST
producer, which is precisely the failure this exists to remove: the bin would
shut before a slower producer had published and the two would never share one.
Coverage sizes the bin to the slowest producer still speaking, and a quantity
silent for a whole window stops being required, so a producer that goes away
cannot stall the bin forever.

Before any bin has closed there is no membership to cover, and a repeat is then
the only rotation evidence the stream has yet offered.
*/
func (retained *window) covered() bool {
	required := false

	for column, needed := range retained.membership {
		if !needed {
			continue
		}

		if !retained.observed[column] {
			return false
		}
		required = true
	}

	return required
}

/*
close commits the open bin and starts the next. The committed bin's membership
becomes what the next bin must cover, less quantities that have been silent for
a whole window.
*/
func (retained *window) close() {
	if retained.count < retained.capacity {
		bin := make([]float64, len(retained.open))
		presence := make([]bool, len(retained.observed))
		copy(bin, retained.open)
		copy(presence, retained.observed)
		retained.slots = append(retained.slots, bin)
		retained.present = append(retained.present, presence)
		retained.standard = append(retained.standard, make([]float64, len(bin)))
		retained.count++
	} else {
		// The bin leaving the window changes the standing of every quantity it
		// held, exactly as the arriving one does. Its storage is reused: a full
		// window commits a bin per rotation, and allocating one each time made
		// the grid's steady state a stream of garbage.
		for column, held := range retained.present[retained.first] {
			if held {
				retained.soil(column)
			}
		}
		copy(retained.slots[retained.first], retained.open)
		copy(retained.present[retained.first], retained.observed)
		retained.first = (retained.first + 1) % retained.capacity
	}

	for column, held := range retained.observed {
		if held {
			retained.soil(column)
		}
	}

	for column := range retained.observed {
		if retained.observed[column] {
			retained.silence[column] = 0
			retained.membership[column] = true
		} else {
			retained.silence[column]++

			if retained.silence[column] >= retained.capacity {
				retained.membership[column] = false
			}
		}
		retained.open[column], retained.observed[column] = 0, false
	}
}

/*
refresh standardizes each quantity against its own behaviour across the bins it
was observed in, which is what makes quantities in different units comparable
at all.

A quantity is standardized once, over its own whole window, rather than once
per partner. Standardizing per partner would give the same quantity a different
scale depending on who it was being compared against, so its relationships
would no longer be readings of one thing.

A quantity that did not vary across the window supports no statement about
co-movement with anything. It is recorded as not varying rather than divided by
an absent dispersion, which would manufacture a relationship out of arithmetic.
*/
func (retained *window) refresh() {
	if len(retained.pending) == 0 {
		return
	}

	for _, column := range retained.pending {
		retained.dirty[column] = false
		sum, count := 0.0, 0

		for bin := range retained.slots {
			if retained.present[bin][column] {
				sum += retained.slots[bin][column]
				count++
			}
		}

		if count < 2 {
			retained.varies[column] = false
			continue
		}
		mean := sum / float64(count)
		spread := 0.0

		for bin := range retained.slots {
			if retained.present[bin][column] {
				difference := retained.slots[bin][column] - mean
				spread += difference * difference
			}
		}
		scale := math.Sqrt(spread / float64(count))
		retained.varies[column] = scale > 0

		if !retained.varies[column] {
			continue
		}

		for bin := range retained.slots {
			if retained.present[bin][column] {
				retained.standard[bin][column] = (retained.slots[bin][column] - mean) / scale
			}
		}
	}
	retained.pending = retained.pending[:0]
}

/*
soil marks one quantity's standing as needing recomputation.

Only the quantities a closing or departing bin actually held can have changed,
so refreshing every quantity on every bin was work proportional to the whole
grid for a change that touched one producer's worth of it.
*/
func (retained *window) soil(column int) {
	if retained.dirty[column] {
		return
	}
	retained.dirty[column] = true
	retained.pending = append(retained.pending, column)
}

/* support counts the bins one quantity was observed in. */
func (retained *window) support(column int) int {
	observed := 0

	for bin := range retained.present {
		if retained.present[bin][column] {
			observed++
		}
	}

	return observed
}
