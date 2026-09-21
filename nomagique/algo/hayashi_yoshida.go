package algo

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
interval is one log return over the half-open span it was measured across.
*/
type interval struct {
	from  float64
	to    float64
	value float64
}

/*
pair is the accumulated asynchronous covariance of one ordered symbol pair.

The retained intervals are the estimator's working set. Arrivals are ordered,
so an interval is dropped once it ends at or before the newest start on the
opposite path: nothing that follows can still overlap it.
*/
type pair struct {
	left        []interval
	right       []interval
	lastLeft    interval
	lastRight   interval
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

Correlation is symmetric but provenance is not, so the estimator holds one
accumulation per ordered symbol pair. A single graph therefore measures every
pair flowing through it rather than one configured pair.
*/
type HayashiYoshidaServer struct {
	*runtime.System
	pairs   map[[2]string]*pair
	current *pair
}

func NewHayashiYoshida(ctx context.Context) *HayashiYoshidaServer {
	server := &HayashiYoshidaServer{
		System: runtime.NewSystem(ctx, "algo.hayashiYoshida"),
		pairs:  make(map[[2]string]*pair),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write admits the current return interval of each leg into the accumulation for
the symbol pair they belong to.
*/
func (server *HayashiYoshidaServer) Write(ctx context.Context, call HayashiYoshida_write) error {
	args := call.Args()

	symbol1, err := args.Symbol1()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"algo.hayashiYoshida: failed to read symbol1 argument",
			err,
		))
	}

	symbol2, err := args.Symbol2()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"algo.hayashiYoshida: failed to read symbol2 argument",
			err,
		))
	}

	leftArrival := interval{
		from:  args.BoundsStart1(),
		to:    args.BoundsEnd1(),
		value: args.Returns1(),
	}

	rightArrival := interval{
		from:  args.BoundsStart2(),
		to:    args.BoundsEnd2(),
		value: args.Returns2(),
	}

	if leftArrival.to < leftArrival.from || rightArrival.to < rightArrival.from {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"algo.hayashiYoshida: return interval ends before it starts",
			nil,
		))
	}

	accumulated := server.pair(symbol1, symbol2)

	// The left leg is admitted first so a pair arriving in one evaluation is
	// counted once rather than from both sides.
	accumulated.admitLeft(leftArrival)
	accumulated.admitRight(rightArrival)
	accumulated.prune()

	server.current = accumulated
	return nil
}

/*
Done reports the estimate for the pair the last write belonged to, together
with the covariance and energies it was formed from. The estimator keeps its
accumulated paths, because a pair correlation is the history of both feeds
rather than one observation.
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

	if server.current == nil {
		return nil
	}

	results.SetCovariance(server.current.covariance)
	results.SetSupport(server.current.support)
	results.SetLeftEnergy(server.current.leftEnergy)
	results.SetRightEnergy(server.current.rightEnergy)
	results.SetCorrelation(
		server.current.covariance /
			math.Sqrt(server.current.leftEnergy*server.current.rightEnergy),
	)

	server.current = nil
	return nil
}

/*
pair resolves the accumulation belonging to one ordered symbol pair, admitting
it on first observation.
*/
func (server *HayashiYoshidaServer) pair(symbol1, symbol2 string) *pair {
	identity := [2]string{symbol1, symbol2}
	accumulated, known := server.pairs[identity]

	if known {
		return accumulated
	}

	accumulated = &pair{}
	server.pairs[identity] = accumulated
	server.Info("measuring pair %s/%s", symbol1, symbol2)

	return accumulated
}

/*
admitLeft accepts a new left interval, pairs it with every retained right
interval it overlaps, and adds its energy once.
*/
func (accumulated *pair) admitLeft(arrival interval) {
	if !admissible(arrival, accumulated.lastLeft, accumulated.hasLeft) {
		return
	}

	for _, candidate := range accumulated.right {
		if !overlaps(arrival, candidate) {
			continue
		}

		accumulated.covariance += arrival.value * candidate.value
		accumulated.support++
	}

	accumulated.leftEnergy += arrival.value * arrival.value
	accumulated.left = append(accumulated.left, arrival)
	accumulated.lastLeft = arrival
	accumulated.hasLeft = true
}

/*
admitRight mirrors admitLeft for the opposite path.
*/
func (accumulated *pair) admitRight(arrival interval) {
	if !admissible(arrival, accumulated.lastRight, accumulated.hasRight) {
		return
	}

	for _, candidate := range accumulated.left {
		if !overlaps(candidate, arrival) {
			continue
		}

		accumulated.covariance += candidate.value * arrival.value
		accumulated.support++
	}

	accumulated.rightEnergy += arrival.value * arrival.value
	accumulated.right = append(accumulated.right, arrival)
	accumulated.lastRight = arrival
	accumulated.hasRight = true
}

/*
prune drops the intervals no future arrival on the opposite path can overlap.
*/
func (accumulated *pair) prune() {
	if accumulated.hasRight {
		accumulated.left = retain(accumulated.left, accumulated.lastRight.from)
	}

	if accumulated.hasLeft {
		accumulated.right = retain(accumulated.right, accumulated.lastLeft.from)
	}
}

/*
admissible rejects a span that carries no elapsed time and a repeat of the
interval already admitted for that path. A duplicate observation at the same
timestamp is not new price evidence.
*/
func admissible(arrival, previous interval, seen bool) bool {
	if arrival.to == arrival.from {
		return false
	}

	if seen && arrival.from == previous.from && arrival.to == previous.to {
		return false
	}

	return true
}

/*
retain keeps the intervals that still end after the opposite path's newest
start, which are exactly those a later arrival can still meet.
*/
func retain(intervals []interval, boundary float64) []interval {
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
func overlaps(left, right interval) bool {
	return left.from < right.to && right.from < left.to
}
