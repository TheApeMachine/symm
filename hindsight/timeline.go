package hindsight

import (
	"sort"
	"sync"
	"time"
)

/*
SymbolSummary is one instrument's presence on a Run's capture tape, together
with what the declared selector found there. It exists so an inspection session
can start from the question "which slides are worth putting under the
microscope?" without the answer ever consulting a SYMM trading output (§27).

Defined counts the observations where the declared coordinate was actually
measurable; Observations counts every observation captured for the symbol. The
two are reported separately so an instrument that was captured but never quoted
reads as unmeasured rather than as flat (§43).
*/
type SymbolSummary struct {
	Symbol       string          `json:"symbol"`
	Observations int             `json:"observations"`
	Defined      int             `json:"defined"`
	Tickers      int             `json:"tickers"`
	Trades       int             `json:"trades"`
	FirstSeq     CaptureSequence `json:"firstSequence"`
	LastSeq      CaptureSequence `json:"lastSequence"`
	FirstAt      time.Time       `json:"firstAt"`
	LastAt       time.Time       `json:"lastAt"`
	Episodes     int             `json:"episodes"`
	Insufficient bool            `json:"insufficientData"`

	// TopExcursion is the largest distance the declared coordinate travelled
	// in any one price episode here, and TopKind names that episode. Regimes
	// are counted separately and never ranked against price geometry: a
	// spread blow-out is a different kind of fact, not a bigger one.
	TopExcursion   float64     `json:"topExcursion"`
	TopKind        EpisodeKind `json:"topKind,omitempty"`
	PriceEpisodes  int         `json:"priceEpisodes"`
	RegimeEpisodes int         `json:"regimeEpisodes"`
}

/*
StreamSpan is one continuous connection span of one transport stream, as the
capture tape recorded it: a stream plus a StreamEpoch, bounded by the first and
last capture sequence observed on it. A new epoch on the same stream is a
reconnect, which is an operational Episode in its own right (§26) and is
detected from identity rather than from a timestamp gap (§7, §9).
*/
type StreamSpan struct {
	Stream    Stream          `json:"stream"`
	Epoch     StreamEpoch     `json:"epoch"`
	FromSeq   CaptureSequence `json:"fromSequence"`
	ToSeq     CaptureSequence `json:"toSequence"`
	FromAt    time.Time       `json:"fromAt"`
	ToAt      time.Time       `json:"toAt"`
	Frames    int             `json:"frames"`
	Reconnect bool            `json:"reconnect"`
}

/*
TimelineBucket is one contiguous span of CaptureSequence summarised for
display. Buckets partition the capture axis — the local observation order (§6)
— never venue or receive time, so the horizontal position of everything on the
timeline is the order in which SYMM actually observed the world.

Open/High/Low/Close describe the declared coordinate inside the span and are
meaningful only when Defined is true. CaptureRate is run-wide observer
throughput across the span (captures per second), which distinguishes "the
market was quiet" from "SYMM was barely observing" (§46).
*/
type TimelineBucket struct {
	Index   int             `json:"index"`
	FromSeq CaptureSequence `json:"fromSequence"`
	ToSeq   CaptureSequence `json:"toSequence"`
	FromAt  time.Time       `json:"fromAt"`
	ToAt    time.Time       `json:"toAt"`

	// Observed* is the span the bucket's observations actually covered, as
	// opposed to the nominal edges the axis drew. A bucket clicked on screen
	// resolves through these to exact capture identities.
	ObservedFromSeq CaptureSequence `json:"observedFromSequence"`
	ObservedToSeq   CaptureSequence `json:"observedToSequence"`
	ObservedFromAt  time.Time       `json:"observedFromAt"`
	ObservedToAt    time.Time       `json:"observedToAt"`

	Observations int     `json:"observations"`
	Tickers      int     `json:"tickers"`
	Trades       int     `json:"trades"`
	TradeQty     float64 `json:"tradeQty"`

	Defined bool    `json:"defined"`
	Open    float64 `json:"open"`
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Close   float64 `json:"close"`

	SpreadFraction    float64 `json:"spreadFraction"`
	HasSpreadFraction bool    `json:"hasSpreadFraction"`
	TouchDepth        float64 `json:"touchDepth"`
	HasTouchDepth     bool    `json:"hasTouchDepth"`

	CaptureRate    float64 `json:"captureRate"`
	HasCaptureRate bool    `json:"hasCaptureRate"`
}

/*
TimelineSpan is the capture interval a timeline covers, in both the ordering
that governs causality (CaptureSequence) and the wall-clock evidence that
accompanies it (§8).
*/
type TimelineSpan struct {
	FromSeq CaptureSequence `json:"fromSequence"`
	ToSeq   CaptureSequence `json:"toSequence"`
	FromAt  time.Time       `json:"fromAt"`
	ToAt    time.Time       `json:"toAt"`
}

/*
TimelineAxis names the ordinate the display buckets partition.

Both are real and neither replaces the other (§8). AxisCapture partitions the
local observation order, so a burst of frames occupies the width it actually
took in SYMM's experience of the world — but a capture axis is shared with
every other stream, so a symbol quoted rarely inside a flood of book updates
collapses into a few buckets. AxisTime partitions wall-clock instants, which is
how a market move reads to a person.

Whichever axis is displayed, every span, marker, and reference point remains
addressed by CaptureSequence: the axis is a rendering, never an identity (§9).
*/
type TimelineAxis string

const (
	AxisCapture TimelineAxis = "capture"
	AxisTime    TimelineAxis = "time"
)

/*
Valid reports whether the axis is one this package can partition.
*/
func (axis TimelineAxis) Valid() bool {
	return axis == AxisCapture || axis == AxisTime
}

/*
TimelineRequest declares what is being projected: which Run, which instrument,
which market coordinate, over which capture interval, at what resolution.
*/
type TimelineRequest struct {
	Run     RunID
	Symbol  string
	Policy  DiscoveryPolicy
	Axis    TimelineAxis
	Buckets int
	FromSeq CaptureSequence
	ToSeq   CaptureSequence

	// Symbols asks for the instrument index alongside the projection. It is
	// the same answer for every window of a run, so a pan or a zoom asks for
	// the window only and leaves the index where the caller already has it.
	Symbols bool
}

/*
Timeline is the horizontal inspection projection of one Run: the declared
coordinate's shape over the capture axis, the Episodes a declared selector
found on it, the transport spans that produced it, and the instruments
available to inspect.

Everything here is market and capture evidence. Nothing in it is derived from a
SYMM artifact, and nothing in it ranks a moment by how well it agrees with what
SYMM did (§27, §58).
*/
type Timeline struct {
	Run        RunID            `json:"run"`
	Symbol     string           `json:"symbol"`
	Coordinate MarketCoordinate `json:"coordinate"`
	Policy     DiscoveryPolicy  `json:"policy"`
	Axis       TimelineAxis     `json:"axis"`
	Span       TimelineSpan     `json:"span"`
	RunSpan    TimelineSpan     `json:"runSpan"`
	Buckets    []TimelineBucket `json:"buckets"`
	Discovery  Discovery        `json:"discovery"`
	Streams    []StreamSpan     `json:"streams"`
	Symbols    []SymbolSummary  `json:"symbols"`

	TotalObservations int `json:"totalObservations"`
	TotalSymbols      int `json:"totalSymbols"`
}

/*
RunIndex is the in-memory projection of one Run's captured market observations,
grouped by instrument and kept in capture order. It is bounded, derived state:
the raw tape stays in durable capture and is streamed into this index once, so
inspection never rebuilds a giant retained market-event database to answer
historical questions (§62).

Discovery results are memoised per declared coordinate, because the coordinate
is part of the selector and a different coordinate is a different selector, not
a different rendering of the same one (§29).
*/
type RunIndex struct {
	run          RunID
	builtAt      time.Time
	observations int
	symbols      map[string][]Observation
	streams      []StreamSpan
	span         TimelineSpan

	// One assembled index serves every concurrent reader of a run, so the
	// memo it fills in as it answers is guarded. The observations themselves
	// are written once during construction and never mutated afterwards.
	memo        sync.Mutex
	discoveries map[string]Discovery
}

/*
NewRunIndex groups one Run's captured observations by instrument, in capture
order, and records the transport spans they arrived on.
*/
func NewRunIndex(run RunID, observations []Observation) *RunIndex {
	index := &RunIndex{
		run:          run,
		builtAt:      time.Now().UTC(),
		observations: len(observations),
		symbols:      make(map[string][]Observation),
		discoveries:  make(map[string]Discovery),
	}

	ordered := make([]Observation, len(observations))
	copy(ordered, observations)
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Capture.Sequence != ordered[right].Capture.Sequence {
			return ordered[left].Capture.Sequence < ordered[right].Capture.Sequence
		}

		return ordered[left].Ordinal < ordered[right].Ordinal
	})

	type streamKey struct {
		stream Stream
		epoch  StreamEpoch
	}

	spans := make(map[streamKey]*StreamSpan)
	epochsSeen := make(map[Stream]int)

	for _, observation := range ordered {
		index.symbols[observation.Symbol] = append(index.symbols[observation.Symbol], observation)

		if index.span.FromSeq == 0 || observation.Capture.Sequence < index.span.FromSeq {
			index.span.FromSeq = observation.Capture.Sequence
			index.span.FromAt = observation.At()
		}

		if observation.Capture.Sequence > index.span.ToSeq {
			index.span.ToSeq = observation.Capture.Sequence
			index.span.ToAt = observation.At()
		}

		key := streamKey{stream: observation.Capture.Stream, epoch: observation.Capture.StreamEpoch}
		span, known := spans[key]

		if !known {
			epochsSeen[observation.Capture.Stream]++

			spans[key] = &StreamSpan{
				Stream:    observation.Capture.Stream,
				Epoch:     observation.Capture.StreamEpoch,
				FromSeq:   observation.Capture.Sequence,
				ToSeq:     observation.Capture.Sequence,
				FromAt:    observation.At(),
				ToAt:      observation.At(),
				Frames:    1,
				Reconnect: epochsSeen[observation.Capture.Stream] > 1,
			}

			continue
		}

		span.ToSeq = observation.Capture.Sequence
		span.ToAt = observation.At()
		span.Frames++
	}

	index.streams = make([]StreamSpan, 0, len(spans))

	for _, span := range spans {
		index.streams = append(index.streams, *span)
	}

	sort.SliceStable(index.streams, func(left, right int) bool {
		return index.streams[left].FromSeq < index.streams[right].FromSeq
	})

	return index
}

/*
Run is the capture Run this index projects.
*/
func (index *RunIndex) Run() RunID {
	return index.run
}

/*
BuiltAt is when the index was assembled from the tape. A Run still being
captured grows after that instant, so a caller can tell a stale projection from
a complete one instead of assuming the tape ended where the index did.
*/
func (index *RunIndex) BuiltAt() time.Time {
	return index.builtAt
}

/*
Observations is the number of market observations decoded from the tape.
*/
func (index *RunIndex) Observations() int {
	return index.observations
}

/*
Discover runs the declared selector over one instrument, memoising the result
per (symbol, coordinate) — the pair that identifies the selector.
*/
func (index *RunIndex) Discover(symbol string, policy DiscoveryPolicy) Discovery {
	policy = policy.normalised()
	key := symbol + "\x00" + string(policy.Coordinate)

	index.memo.Lock()
	discovery, known := index.discoveries[key]
	index.memo.Unlock()

	if known {
		return discovery
	}

	discovery = DiscoverEpisodes(symbol, index.symbols[symbol], policy)

	index.memo.Lock()
	index.discoveries[key] = discovery
	index.memo.Unlock()

	return discovery
}

/*
Summaries returns every instrument on the tape with what the declared selector
found there, ranked so the largest observed market geometry sorts first. The
ranking orders candidate inspection targets by the distance the declared
coordinate travelled — market geometry alone. It never consults an Opportunity,
a Perspective, a decision, or any other SYMM output (§27), and a high rank
asserts nothing about what SYMM should have done there.
*/
func (index *RunIndex) Summaries(policy DiscoveryPolicy) []SymbolSummary {
	policy = policy.normalised()
	summaries := make([]SymbolSummary, 0, len(index.symbols))

	for symbol, observations := range index.symbols {
		if len(observations) == 0 {
			continue
		}

		discovery := index.Discover(symbol, policy)

		summary := SymbolSummary{
			Symbol:       symbol,
			Observations: len(observations),
			Defined:      discovery.Defined,
			FirstSeq:     observations[0].Capture.Sequence,
			LastSeq:      observations[len(observations)-1].Capture.Sequence,
			FirstAt:      observations[0].At(),
			LastAt:       observations[len(observations)-1].At(),
			Episodes:     len(discovery.Episodes),
			Insufficient: discovery.InsufficientData,
		}

		for _, observation := range observations {
			switch observation.Kind {
			case "ticker":
				summary.Tickers++
			case "trade":
				summary.Trades++
			}
		}

		for _, episode := range discovery.Episodes {
			if !episode.IsPriceGeometry() {
				summary.RegimeEpisodes++
				continue
			}

			summary.PriceEpisodes++

			if magnitude := episode.Magnitude(); magnitude > summary.TopExcursion {
				summary.TopExcursion = magnitude
				summary.TopKind = episode.Kind
			}
		}

		summaries = append(summaries, summary)
	}

	sort.SliceStable(summaries, func(left, right int) bool {
		if summaries[left].TopExcursion != summaries[right].TopExcursion {
			return summaries[left].TopExcursion > summaries[right].TopExcursion
		}

		if summaries[left].PriceEpisodes != summaries[right].PriceEpisodes {
			return summaries[left].PriceEpisodes > summaries[right].PriceEpisodes
		}

		if summaries[left].Observations != summaries[right].Observations {
			return summaries[left].Observations > summaries[right].Observations
		}

		return summaries[left].Symbol < summaries[right].Symbol
	})

	return summaries
}

/*
CapturesBefore returns the instrument's own envelopes at or before one capture
coordinate, newest first and bounded by limit.

These are the only envelopes that could have carried this instrument's signals,
so an as-of walk over them reaches the latest causally available value without
touching the rest of the run's tape. Ordering is CaptureSequence descending —
the causal order run backwards, never a time sort.
*/
func (index *RunIndex) CapturesBefore(
	symbol string,
	sequence CaptureSequence,
	limit int,
) []EnvelopeRef {
	observations := index.symbols[symbol]
	refs := make([]EnvelopeRef, 0, limit)

	if limit <= 0 {
		limit = 64
	}

	for position := len(observations) - 1; position >= 0; position-- {
		observation := observations[position]

		if observation.Capture.Sequence > sequence {
			continue
		}

		refs = append(refs, EnvelopeRef{
			Origin:  observation.Capture,
			Ordinal: observation.Ordinal,
		})

		if len(refs) >= limit {
			break
		}
	}

	return refs
}

/*
ObservationAt returns the instrument's observation at one exact capture
coordinate, when it has one.
*/
func (index *RunIndex) ObservationAt(
	symbol string,
	sequence CaptureSequence,
) (Observation, bool) {
	for _, observation := range index.symbols[symbol] {
		if observation.Capture.Sequence == sequence {
			return observation, true
		}
	}

	return Observation{}, false
}

/*
Project assembles the horizontal timeline for one request: the coordinate's
shape bucketed along the capture axis, the Episodes the declared selector
found, the transport spans, and the instrument index.
*/
func (index *RunIndex) Project(request TimelineRequest) Timeline {
	policy := request.Policy.normalised()

	if !request.Axis.Valid() {
		request.Axis = AxisTime
	}

	timeline := Timeline{
		Axis:              request.Axis,
		Run:               index.run,
		Symbol:            request.Symbol,
		Coordinate:        policy.Coordinate,
		Policy:            policy,
		RunSpan:           index.span,
		Streams:           index.streams,
		Symbols:           make([]SymbolSummary, 0),
		TotalObservations: index.observations,
		TotalSymbols:      len(index.symbols),
		Buckets:           make([]TimelineBucket, 0),
	}

	if request.Symbols {
		timeline.Symbols = index.Summaries(policy)
	}

	observations := index.symbols[request.Symbol]

	// An instrument absent from the tape still gets a well-formed, empty
	// discovery: unavailable is a state the reader must be able to see, and a
	// half-populated result would make it indistinguishable from a quiet one.
	timeline.Discovery = Discovery{
		Symbol:     request.Symbol,
		Coordinate: policy.Coordinate,
		Policy:     policy,
		Episodes:   make([]Episode, 0),
	}

	if len(observations) == 0 {
		return timeline
	}

	timeline.Discovery = index.Discover(request.Symbol, policy)

	from := request.FromSeq
	to := request.ToSeq

	if from == 0 {
		from = observations[0].Capture.Sequence
	}

	if to == 0 || to < from {
		to = observations[len(observations)-1].Capture.Sequence
	}

	windowed := make([]Observation, 0, len(observations))

	for _, observation := range observations {
		if observation.Capture.Sequence < from || observation.Capture.Sequence > to {
			continue
		}

		windowed = append(windowed, observation)
	}

	if len(windowed) == 0 {
		timeline.Span = TimelineSpan{FromSeq: from, ToSeq: to}
		return timeline
	}

	timeline.Span = TimelineSpan{
		FromSeq: from,
		ToSeq:   to,
		FromAt:  windowed[0].At(),
		ToAt:    windowed[len(windowed)-1].At(),
	}

	timeline.Buckets = bucketObservations(
		windowed,
		from,
		to,
		request.Axis,
		request.Buckets,
		policy.Coordinate,
	)

	return timeline
}

/*
bucketObservations partitions the requested window into count equal spans of
the requested axis and folds each observation into the span it falls in.

On the capture axis the partition is uniform in CaptureSequence; on the time
axis it is uniform in the instants the observations carry. Either way each
bucket reports the capture range it actually covered, so a bucket clicked on
screen resolves to exact capture identities rather than to a time neighbourhood
(§9).
*/
func bucketObservations(
	observations []Observation,
	from, to CaptureSequence,
	axis TimelineAxis,
	count int,
	coordinate MarketCoordinate,
) []TimelineBucket {
	if count <= 0 {
		count = 240
	}

	if len(observations) == 0 {
		return make([]TimelineBucket, 0)
	}

	// Resolving finer than the observations themselves buys nothing and costs
	// legibility: it shreds a sparse instrument's track into single-sample
	// fragments separated by spans that were never observed at all.
	if count > len(observations) {
		count = len(observations)
	}

	slotOf, edges, count := axisPartition(observations, from, to, axis, count)
	buckets := make([]TimelineBucket, count)

	for index := range buckets {
		buckets[index] = TimelineBucket{Index: index}

		if edges != nil {
			buckets[index].FromSeq = edges[index].fromSeq
			buckets[index].ToSeq = edges[index].toSeq
			buckets[index].FromAt = edges[index].fromAt
			buckets[index].ToAt = edges[index].toAt
		}
	}

	spreadTotals := make([]float64, count)
	spreadCounts := make([]int, count)
	depthTotals := make([]float64, count)
	depthCounts := make([]int, count)
	observedFrom := make([]CaptureSequence, count)
	observedTo := make([]CaptureSequence, count)

	for _, observation := range observations {
		slot := slotOf(observation)

		if slot < 0 || slot >= count {
			continue
		}

		bucket := &buckets[slot]
		bucket.Observations++

		if observedFrom[slot] == 0 || observation.Capture.Sequence < observedFrom[slot] {
			observedFrom[slot] = observation.Capture.Sequence
		}

		if observation.Capture.Sequence > observedTo[slot] {
			observedTo[slot] = observation.Capture.Sequence
		}

		switch observation.Kind {
		case "ticker":
			bucket.Tickers++
		case "trade":
			bucket.Trades++
			bucket.TradeQty += observation.TradeQty
		}

		at := observation.At()

		if !at.IsZero() {
			if bucket.ObservedFromAt.IsZero() || at.Before(bucket.ObservedFromAt) {
				bucket.ObservedFromAt = at
			}

			if at.After(bucket.ObservedToAt) {
				bucket.ObservedToAt = at
			}
		}

		if spread, ok := observation.SpreadFraction(); ok {
			spreadTotals[slot] += spread
			spreadCounts[slot]++
		}

		if depth, ok := observation.TouchDepth(); ok {
			depthTotals[slot] += depth
			depthCounts[slot]++
		}

		value, defined := observation.Value(coordinate)

		if !defined {
			continue
		}

		if !bucket.Defined {
			bucket.Defined = true
			bucket.Open = value
			bucket.High = value
			bucket.Low = value
		}

		if value > bucket.High {
			bucket.High = value
		}

		if value < bucket.Low {
			bucket.Low = value
		}

		bucket.Close = value
	}

	for index := range buckets {
		bucket := &buckets[index]

		bucket.ObservedFromSeq = observedFrom[index]
		bucket.ObservedToSeq = observedTo[index]

		if spreadCounts[index] > 0 {
			bucket.SpreadFraction = spreadTotals[index] / float64(spreadCounts[index])
			bucket.HasSpreadFraction = true
		}

		if depthCounts[index] > 0 {
			bucket.TouchDepth = depthTotals[index] / float64(depthCounts[index])
			bucket.HasTouchDepth = true
		}

		// Observer throughput is run-wide: how many captures of any stream the
		// process took while this bucket's observations arrived. It separates
		// "the market was quiet" from "SYMM was barely observing" (§46).
		elapsed := bucket.ObservedToAt.Sub(bucket.ObservedFromAt).Seconds()

		if elapsed > 0 && bucket.ObservedToSeq > bucket.ObservedFromSeq {
			bucket.CaptureRate = float64(bucket.ObservedToSeq-bucket.ObservedFromSeq) / elapsed
			bucket.HasCaptureRate = true
		}
	}

	return buckets
}

/*
bucketEdge is one display bucket's nominal boundary on both ordinates.
*/
type bucketEdge struct {
	fromSeq CaptureSequence
	toSeq   CaptureSequence
	fromAt  time.Time
	toAt    time.Time
}

/*
axisPartition builds the bucket edges and the observation-to-bucket mapping for
the requested axis, returning the count actually used — a window narrower than
the requested resolution is partitioned at its own width rather than padded
with empty buckets.
*/
func axisPartition(
	observations []Observation,
	from, to CaptureSequence,
	axis TimelineAxis,
	count int,
) (func(Observation) int, []bucketEdge, int) {
	if axis == AxisTime {
		start := observations[0].At()
		end := observations[len(observations)-1].At()

		for _, observation := range observations {
			at := observation.At()

			if at.IsZero() {
				continue
			}

			if start.IsZero() || at.Before(start) {
				start = at
			}

			if at.After(end) {
				end = at
			}
		}

		span := end.Sub(start)

		if span > 0 {
			width := span / time.Duration(count)

			if width <= 0 {
				width = time.Nanosecond
			}

			edges := make([]bucketEdge, count)

			for index := range edges {
				edges[index] = bucketEdge{
					fromAt: start.Add(time.Duration(index) * width),
					toAt:   start.Add(time.Duration(index+1) * width),
				}
			}

			edges[count-1].toAt = end

			return func(observation Observation) int {
				at := observation.At()

				if at.IsZero() {
					return -1
				}

				slot := int(at.Sub(start) / width)

				if slot >= count {
					slot = count - 1
				}

				return slot
			}, edges, count
		}
	}

	span := uint64(to-from) + 1

	if uint64(count) > span {
		count = int(span)
	}

	if count <= 0 {
		count = 1
	}

	width := span / uint64(count)

	if width == 0 {
		width = 1
	}

	edges := make([]bucketEdge, count)

	for index := range edges {
		bucketFrom := uint64(from) + uint64(index)*width
		bucketTo := bucketFrom + width - 1

		if index == count-1 {
			bucketTo = uint64(to)
		}

		edges[index] = bucketEdge{
			fromSeq: CaptureSequence(bucketFrom),
			toSeq:   CaptureSequence(bucketTo),
		}
	}

	return func(observation Observation) int {
		slot := int((uint64(observation.Capture.Sequence) - uint64(from)) / width)

		if slot >= count {
			slot = count - 1
		}

		return slot
	}, edges, count
}
