package hawkes

import (
	"context"
	"math"
	"sort"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/stat"
)

/*
seedRatioFloor keeps a seed off the bounds themselves, whose unconstrained
coordinates are infinite.
*/
const seedRatioFloor = 1e-9

/*
DomainServer derives the region an optimizer may look for parameters in, and
a starting point inside it, from the observed window alone.

Nothing here is chosen. Every bound is stated in a scale the observation
itself carries:

  - A decay slower than the window is one the window cannot see decaying, and
    a decay faster than the arrivals are spaced is one the arrivals cannot
    resolve. The observed gaps set both ends.
  - A component cannot arrive less often than once in the whole window, nor
    more often than every arrival in it being that component's. The span and
    the count set both ends.
  - Excitation has the units of a rate and is read against the decay: an
    excitation equal to the decay means one arrival begets one arrival, the
    boundary past which cascades no longer die out.

The seed is the process that explains the window without any excitation at
all: each component arriving at its own observed rate, decaying on the
observed median gap. An optimizer that improves on it has found excitation
the data actually carries.
*/
type DomainServer struct {
	lower   []float64
	upper   []float64
	seed    []float64
	defined bool
}

func NewDomain() *DomainServer {
	return &DomainServer{}
}

func (server *DomainServer) Write(ctx context.Context, call Domain_write) error {
	args := call.Args()
	times, err := server.readFloats(args.Times())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to read times",
			err,
		))
	}

	components, err := server.readInts(args.Components())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to read components",
			err,
		))
	}

	server.clear()
	dimension := int(args.Dimension())
	origin := args.Origin()
	horizon := args.Horizon()
	span := horizon - origin
	coupled := args.Coupled() > 0

	if dimension <= 0 || span <= 0 || len(times) != len(components) || len(times) < 2 {
		return nil
	}

	gaps := server.gaps(times, horizon)

	if len(gaps) == 0 {
		return nil
	}

	counts, total := server.counts(components, times, origin, horizon, dimension)

	if total == 0 {
		return nil
	}

	server.build(dimension, span, total, counts, gaps, coupled)
	return nil
}

/*
clear drops the previous derivation so a window that cannot support one never
leaves the last answer standing in its place.
*/
func (server *DomainServer) clear() {
	server.lower = nil
	server.upper = nil
	server.seed = nil
	server.defined = false
}

/*
gaps returns the positive intervals between consecutive arrivals, sorted.
Simultaneous arrivals contribute no interval: they say nothing about how
fast the process can be resolved.
*/
func (server *DomainServer) gaps(times []float64, horizon float64) []float64 {
	gaps := make([]float64, 0, len(times))

	for index := 1; index < len(times); index++ {
		if times[index] > horizon {
			break
		}

		gap := times[index] - times[index-1]

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	sort.Float64s(gaps)
	return gaps
}

/*
counts returns how many observed arrivals each component contributed, and how
many there were in total. Arrivals at or before the origin are prehistory and
are not counted.
*/
func (server *DomainServer) counts(
	components []float64,
	times []float64,
	origin, horizon float64,
	dimension int,
) ([]float64, float64) {
	counts := make([]float64, dimension)
	total := 0.0

	for index, value := range components {
		if times[index] <= origin || times[index] > horizon {
			continue
		}

		component := int(value)

		if component < 0 || component >= dimension {
			continue
		}

		counts[component]++
		total++
	}

	return counts, total
}

/*
build lays out the bounds and the seed in the coordinate order the rest of
this package uses: the baselines, the excitation matrix row-major, then the
decay rate, each in log space.
*/
func (server *DomainServer) build(
	dimension int,
	span, total float64,
	counts, gaps []float64,
	coupled bool,
) {
	quickest := stat.Quantile(0.25, stat.LinInterp, gaps, nil)
	median := stat.Quantile(0.5, stat.LinInterp, gaps, nil)

	if quickest <= 0 || median <= 0 {
		return
	}

	decayLow := 1 / span
	decayHigh := 1 / quickest
	rateLow := 1 / span
	rateHigh := total / span

	if decayHigh <= decayLow || rateHigh <= rateLow {
		return
	}

	// An excitation this small begets less than one arrival across the whole
	// window, which is the least the window could possibly evidence.
	exciteLow := decayLow / total
	exciteHigh := decayHigh
	decaySeed := 1 / median

	// Excitation starts halfway across the only interval that means
	// anything for it: from a process that never excites itself to one
	// whose cascades never die out. With every entry equal, the branching
	// matrix has spectral radius dimension times the entry, so half of
	// critical is reached at half over the dimension.
	exciteSeed := decaySeed / (2 * float64(dimension))

	width := dimension + dimension*dimension + 1
	lower := make([]float64, width)
	upper := make([]float64, width)
	seed := make([]float64, width)

	for component := 0; component < dimension; component++ {
		lower[component] = math.Log(rateLow)
		upper[component] = math.Log(rateHigh)
		seed[component] = math.Log(math.Max(rateLow, counts[component]/span))
	}

	for index := dimension; index < width-1; index++ {
		offset := index - dimension
		lower[index] = math.Log(exciteLow)
		upper[index] = math.Log(exciteHigh)
		seed[index] = math.Log(exciteSeed)

		if coupled || offset/dimension == offset%dimension {
			continue
		}

		// A restricted process in which this component does not excite the
		// other: the bound is degenerate, so the map reports no slope here
		// and a search cannot move off it.
		lower[index] = math.Log(exciteLow)
		upper[index] = math.Log(exciteLow)
		seed[index] = math.Log(exciteLow)
	}

	lower[width-1] = math.Log(decayLow)
	upper[width-1] = math.Log(decayHigh)
	seed[width-1] = math.Log(decaySeed)

	server.lower = lower
	server.upper = upper
	server.seed = server.coordinates(seed, lower, upper)
	server.defined = true
}

/*
coordinates turns the seed from the parameters it describes into the
unconstrained coordinates a search moves in, by inverting the bounded map
Parameters applies. The two are ends of one convention: a seed handed over
in the wrong one is not a bad starting point but a different process
entirely.
*/
func (server *DomainServer) coordinates(seed, lower, upper []float64) []float64 {
	for index := range seed {
		span := upper[index] - lower[index]

		if span <= 0 {
			seed[index] = 0
			continue
		}

		ratio := (seed[index] - lower[index]) / span

		// The coordinates mapping onto the bounds themselves are infinite,
		// so a seed sitting on one is drawn just inside it.
		ratio = math.Max(seedRatioFloor, math.Min(1-seedRatioFloor, ratio))
		lift := ratio / (1 - ratio)

		// The inverse softplus log(exp(x) - 1), written as x + log(1 - exp(-x))
		// so a seed drawn in against the upper bound does not overflow exp.
		seed[index] = lift + math.Log(-math.Expm1(-lift))
	}

	return seed
}

/*
readFloats copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *DomainServer) readFloats(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

/*
readInts copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *DomainServer) readInts(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *DomainServer) Done(ctx context.Context, call Domain_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to allocate results",
			err,
		))
	}

	lowerList, err := results.NewLower(int32(len(server.lower)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to allocate lower list",
			err,
		))
	}

	for index, value := range server.lower {
		lowerList.Set(index, value)
	}

	upperList, err := results.NewUpper(int32(len(server.upper)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to allocate upper list",
			err,
		))
	}

	for index, value := range server.upper {
		upperList.Set(index, value)
	}

	seedList, err := results.NewSeed(int32(len(server.seed)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes domain: failed to allocate seed list",
			err,
		))
	}

	for index, value := range server.seed {
		seedList.Set(index, value)
	}

	results.SetDefined(server.defined)
	return nil
}
