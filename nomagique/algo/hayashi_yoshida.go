package algo

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
span is one log return over the half-open interval it was measured across,
held while an evaluation is in flight.
*/
type span struct {
	from  float64
	to    float64
	value float64
}

/*
history is the estimator's retained working set for one pair of paths.

Arrivals are ordered, so an interval is dropped once it ends at or before the
newest start on the opposite path: nothing that follows can still overlap it.
*/
type history struct {
	left        []span
	right       []span
	lastLeft    span
	lastRight   span
	hasLeft     bool
	hasRight    bool
	covariance  float64
	support     float64
	leftEnergy  float64
	rightEnergy float64
}

/*
HayashiYoshidaServer owns the asynchronous covariance of two return paths.

Each return contributes once to its own path energy and once to every strictly
overlapping cross product on the opposite path. That is what lets two feeds be
correlated without resampling either onto an invented common clock, so a fast
path cannot inflate its own denominator with returns that never met the other
path. Support counts overlapping interval pairs, not independent samples.

The estimator retains nothing between evaluations. The accumulation arrives as
a value and the updated one leaves as a value, so a graph holds one history per
pair of symbols in storage it composed rather than in a map hidden here.
*/
type HayashiYoshidaServer struct {
	*runtime.System
	current history
}

func NewHayashiYoshida(ctx context.Context) *HayashiYoshidaServer {
	server := &HayashiYoshidaServer{
		System: runtime.NewSystem(ctx, "algo.hayashiYoshida"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write admits the current return interval of each leg into the accumulation it
was handed.
*/
func (server *HayashiYoshidaServer) Write(ctx context.Context, call HayashiYoshida_write) error {
	args := call.Args()

	encoded, err := args.State()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"algo.hayashiYoshida: failed to read state argument",
			err,
		))
	}

	restored, err := decodeHistory(encoded)

	if err != nil {
		return err
	}

	left := span{from: args.BoundsStart1(), to: args.BoundsEnd1(), value: args.Returns1()}
	right := span{from: args.BoundsStart2(), to: args.BoundsEnd2(), value: args.Returns2()}

	if left.to < left.from || right.to < right.from {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: return interval ends before it starts",
			nil,
		))
	}

	// The left leg is admitted first so a pair arriving in one evaluation is
	// counted once rather than from both sides.
	restored.admitLeft(left)
	restored.admitRight(right)
	restored.prune()

	server.current = restored
	return nil
}

/*
Done reports the estimate, the terms it was formed from, and the accumulation
the graph should retain for the next observation of this pair.
*/
func (server *HayashiYoshidaServer) Done(ctx context.Context, call HayashiYoshida_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to allocate done results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetCovariance(server.current.covariance)
	results.SetSupport(server.current.support)
	results.SetLeftEnergy(server.current.leftEnergy)
	results.SetRightEnergy(server.current.rightEnergy)

	// Undefined normalization stays undefined rather than being reported as a
	// correlation of zero, which would be indistinguishable from real evidence
	// of independence.
	results.SetCorrelation(
		server.current.covariance /
			math.Sqrt(server.current.leftEnergy*server.current.rightEnergy),
	)

	encoded, err := encodeHistory(server.current)

	if err != nil {
		return err
	}

	if err := results.SetState(encoded); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to set state",
			err,
		))
	}

	server.current = history{}
	return nil
}

/*
admitLeft accepts a new left interval, pairs it with every retained right
interval it overlaps, and adds its energy once.
*/
func (retained *history) admitLeft(arrival span) {
	if !admissible(arrival, retained.lastLeft, retained.hasLeft) {
		return
	}

	for _, candidate := range retained.right {
		if !overlaps(arrival, candidate) {
			continue
		}

		retained.covariance += arrival.value * candidate.value
		retained.support++
	}

	retained.leftEnergy += arrival.value * arrival.value
	retained.left = append(retained.left, arrival)
	retained.lastLeft = arrival
	retained.hasLeft = true
}

/*
admitRight mirrors admitLeft for the opposite path.
*/
func (retained *history) admitRight(arrival span) {
	if !admissible(arrival, retained.lastRight, retained.hasRight) {
		return
	}

	for _, candidate := range retained.left {
		if !overlaps(candidate, arrival) {
			continue
		}

		retained.covariance += candidate.value * arrival.value
		retained.support++
	}

	retained.rightEnergy += arrival.value * arrival.value
	retained.right = append(retained.right, arrival)
	retained.lastRight = arrival
	retained.hasRight = true
}

/*
prune drops the intervals no future arrival on the opposite path can overlap.
*/
func (retained *history) prune() {
	if retained.hasRight {
		retained.left = keep(retained.left, retained.lastRight.from)
	}

	if retained.hasLeft {
		retained.right = keep(retained.right, retained.lastLeft.from)
	}
}

/*
admissible rejects a span that carries no elapsed time and a repeat of the
interval already admitted for that path. A duplicate observation at the same
timestamp is not new price evidence.
*/
func admissible(arrival, previous span, seen bool) bool {
	if arrival.to == arrival.from {
		return false
	}

	if seen && arrival.from == previous.from && arrival.to == previous.to {
		return false
	}

	return true
}

/*
keep retains the intervals that still end after the opposite path's newest
start, which are exactly those a later arrival can still meet.
*/
func keep(intervals []span, boundary float64) []span {
	kept := intervals[:0]

	for _, candidate := range intervals {
		if candidate.to <= boundary {
			continue
		}

		kept = append(kept, candidate)
	}

	return kept
}

/*
overlaps reports the strict interval intersection Hayashi-Yoshida weights by.
*/
func overlaps(left, right span) bool {
	return left.from < right.to && right.from < left.to
}

/*
decodeHistory restores the accumulation a graph retained. Empty state is a
pair observed for the first time, not an error.
*/
func decodeHistory(encoded []byte) (history, error) {
	if len(encoded) == 0 {
		return history{}, nil
	}

	message, err := capnp.Unmarshal(encoded)

	if err != nil {
		return history{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: retained state is not a message",
			err,
		))
	}

	stored, err := ReadRootAccumulation(message)

	if err != nil {
		return history{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: retained state is not an accumulation",
			err,
		))
	}

	restored := history{
		hasLeft:     stored.LeftSeen(),
		hasRight:    stored.RightSeen(),
		covariance:  stored.Covariance(),
		support:     stored.Support(),
		leftEnergy:  stored.LeftEnergy(),
		rightEnergy: stored.RightEnergy(),
	}

	if restored.left, err = readSpans(stored.Left); err != nil {
		return history{}, err
	}

	if restored.right, err = readSpans(stored.Right); err != nil {
		return history{}, err
	}

	if restored.lastLeft, err = readSpan(stored.LastLeft); err != nil {
		return history{}, err
	}

	if restored.lastRight, err = readSpan(stored.LastRight); err != nil {
		return history{}, err
	}

	return restored, nil
}

/*
readSpans reads one retained interval list.
*/
func readSpans(read func() (Interval_List, error)) ([]span, error) {
	stored, err := read()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: failed to read retained intervals",
			err,
		))
	}

	intervals := make([]span, 0, stored.Len())

	for index := range stored.Len() {
		each := stored.At(index)
		intervals = append(intervals, span{
			from:  each.From(),
			to:    each.To(),
			value: each.Value(),
		})
	}

	return intervals, nil
}

/*
readSpan reads one retained interval.
*/
func readSpan(read func() (Interval, error)) (span, error) {
	stored, err := read()

	if err != nil {
		return span{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: failed to read retained interval",
			err,
		))
	}

	return span{from: stored.From(), to: stored.To(), value: stored.Value()}, nil
}

/*
encodeHistory renders the accumulation for the graph to retain.
*/
func encodeHistory(retained history) ([]byte, error) {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to create state message",
			err,
		))
	}

	stored, err := NewRootAccumulation(segment)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to create accumulation",
			err,
		))
	}

	stored.SetLeftSeen(retained.hasLeft)
	stored.SetRightSeen(retained.hasRight)
	stored.SetCovariance(retained.covariance)
	stored.SetSupport(retained.support)
	stored.SetLeftEnergy(retained.leftEnergy)
	stored.SetRightEnergy(retained.rightEnergy)

	if err := writeSpans(retained.left, stored.NewLeft); err != nil {
		return nil, err
	}

	if err := writeSpans(retained.right, stored.NewRight); err != nil {
		return nil, err
	}

	if err := writeSpan(retained.lastLeft, stored.NewLastLeft); err != nil {
		return nil, err
	}

	if err := writeSpan(retained.lastRight, stored.NewLastRight); err != nil {
		return nil, err
	}

	encoded, err := message.Marshal()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to marshal state",
			err,
		))
	}

	return encoded, nil
}

/*
writeSpans renders one retained interval list.
*/
func writeSpans(intervals []span, alloc func(int32) (Interval_List, error)) error {
	stored, err := alloc(int32(len(intervals)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to allocate retained intervals",
			err,
		))
	}

	for index, each := range intervals {
		target := stored.At(index)
		target.SetFrom(each.from)
		target.SetTo(each.to)
		target.SetValue(each.value)
	}

	return nil
}

/*
writeSpan renders one retained interval.
*/
func writeSpan(interval span, alloc func() (Interval, error)) error {
	stored, err := alloc()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"algo.hayashiYoshida: failed to allocate retained interval",
			err,
		))
	}

	stored.SetFrom(interval.from)
	stored.SetTo(interval.to)
	stored.SetValue(interval.value)

	return nil
}
