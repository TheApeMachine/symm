package manifold

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
TouchCenter keeps the running best price on each side of one symbol, reduced to
a center, using the composed statistic.Touch primitive. It holds no book: each
observation only advances the adaptive rings feeding the touch, so the center
is derived from the symbol's own recent extremes rather than a reconstructed
order book.
*/
type TouchCenter struct {
	number *nomagique.Number[string]
	mu     sync.Mutex
	// indices assigns each symbol a stable uint32 index for particle token
	// packing; it is identity for the resident manifold embedding, not a
	// ranking.
	indices   map[string]uint32
	nextIndex uint32
}

/*
newTouchCenter composes one statistic.Touch per symbol, reading the two sides
and the shared event time off the frame under stable slot names.
*/
func newTouchCenter() *TouchCenter {
	upper := nmtypes.MustIntern("manifold/touch/upper")
	lower := nmtypes.MustIntern("manifold/touch/lower")
	sec := nmtypes.MustIntern("manifold/touch/sec")
	nsec := nmtypes.MustIntern("manifold/touch/nsec")

	touch := statistic.Touch("manifold/touch", upper, lower, sec, nsec)

	return &TouchCenter{
		number:    nomagique.NewNumber[string](touch),
		indices:   make(map[string]uint32),
		nextIndex: 0,
	}
}

/*
Observe feeds one upper and one lower observation for the symbol through the
composed touch and returns the current center. A zero side means that side has
no current observation this message; the touch still advances the present side.
*/
func (center *TouchCenter) Observe(symbol string, upper, lower, at float64) float64 {
	if center == nil || center.number == nil {
		return 0
	}

	sec := math.Floor(at)
	nsec := math.Floor((at - sec) * 1e9)

	input := nmtypes.Frame{}
	input.Put(nmtypes.MustIntern("manifold/touch/upper"), upper)
	input.Put(nmtypes.MustIntern("manifold/touch/lower"), lower)
	input.Put(nmtypes.MustIntern("manifold/touch/sec"), sec)
	input.Put(nmtypes.MustIntern("manifold/touch/nsec"), nsec)

	output := center.number.Step(symbol, input)

	if output.Err != nil {
		return 0
	}

	value, found := statistic.TouchCenter(&output, "manifold/touch")

	if !found {
		return 0
	}

	return value
}

/*
Index returns a stable uint32 identity for one symbol, assigning the next free
index on first use. It is used only to tag particles so their token space does
not collide across symbols.
*/
func (center *TouchCenter) Index(symbol string) uint32 {
	if center == nil {
		return 0
	}

	center.mu.Lock()
	defer center.mu.Unlock()

	if index, found := center.indices[symbol]; found {
		return index
	}

	index := center.nextIndex
	center.nextIndex++
	center.indices[symbol] = index

	return index
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
Scale returns the symbol's running RMS of log-distance from center, so particles
land on the Ω lattice against the symbol's own observed price spread rather than
an imposed constant. It is a single scalar accumulator per symbol, not a book.
*/
func (center *TouchCenter) Scale(symbol string, message kraken.Level3Data, mid float64) float64 {
	if center == nil || mid <= 0 {
		return 0
	}

	sum := 0.0
	count := 0

	for _, order := range message.Bids {
		if order.LimitPrice == nil {
			continue
		}

		if deviation := math.Log(order.LimitPrice.Float64()) - math.Log(mid); isFinite(deviation) {
			sum += deviation * deviation
			count++
		}
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil {
			continue
		}

		if deviation := math.Log(order.LimitPrice.Float64()) - math.Log(mid); isFinite(deviation) {
			sum += deviation * deviation
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return math.Sqrt(sum / float64(count))
}
