package manifold

import (
	"fmt"
	"math"
	"sort"
	"sync"

	mgrbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/adaptive"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
unitCarrierMass is the mass contributed by one observed order. Inelastic collision
adds these units when repeated observations merge into a resident carrier.
unitOscillatorEnergy is the energy every token carries on injection. It is the unit of
the oscillator store, so amplitude sqrt(Energy) is one for an unforced particle and the
system starts with exactly one unit of energy per observation.
symbolIndexMask limits symbol indices to 15 bits so the packed token ID stays cleanly within
Metal's 16-bit token bitmask (0xFFFF).
*/
const (
	unitCarrierMass      float32 = 1
	unitOscillatorEnergy float32 = 1
	symbolIndexMask      uint32  = 0x7fff
)

/*
orderEntry is an internal pair coupling an L3 order with its book direction (Bid/Ask).
Since *mgrbook.Order holds limit price and quantity decimals but not its side,
orderEntry carries side metadata through the batch creation pipeline.
*/
type orderEntry struct {
	order *mgrbook.Order
	side  mgrbook.BookDirection
}

/*
Tokenizer maps Level 3 order book samples into Sensorium thermo-manifold particles.

Rather than rasterizing orders onto a static grid or treating text as tokens, Tokenizer
projects market structure into a continuous 3D pilot-wave fluid:
  - Position X: Relative log price log(Price / MidPrice), placing Bids at X < 0.5 and Asks at X > 0.5.
  - Position Y: Log order quantity log(Quantity), compressing liquidity across orders of magnitude.
  - Position Z: Empirical order age rank (oldest = 0 at front, newest = N-1 at back).
  - Mass: One carrier unit per observed resting order. Repeated observations gain mass through inelastic collision.
  - Heat: Zero on injection. Heat is earned from collision and Planck relaxation, not stamped.
  - Energy: One unit per order, forced above unit by the side's Hawkes self-excitation.
  - Phase: Bid-Ask spread phase boundary ([0, pi) for Bids, [pi, 2pi) for Asks), swept by price-time queue priority.
  - Omega: Log distance from mid, measured against the symbol's own accumulated scale.

Position is a viewport: X, Y and Z normalize against the current batch so the visible
book always fills the PIC grid. Omega is an identity and therefore does not, because a
frequency that is re-derived from whichever orders share the batch cannot hold a carrier
attractor well in place.
*/
type Tokenizer struct {
	config    pfluid.Config
	universe  []string
	mutex     sync.Mutex
	scales    map[string]*adaptive.Accumulator
	converged map[string]float64
}

/*
NewTokenizer constructs a Tokenizer bound to a fluid configuration and a known symbol universe.
The symbol universe is automatically sorted to guarantee a stable, deterministic token basis
for GPU memory merging regardless of tick arrival order.
*/
func NewTokenizer(config pfluid.Config, symbols []string) *Tokenizer {
	return &Tokenizer{
		config:    config,
		universe:  sortedUniverse(symbols),
		scales:    make(map[string]*adaptive.Accumulator),
		converged: make(map[string]float64),
	}
}

/*
NewBatch converts resting L3 bid and ask orders into an appendable particle batch in a single pass.

It merges bids and asks, extracts Decimal values into float64, derives dynamic geometry bounds,
resolves the symbol token ID, computes empirical age ranks, and projects every order directly into a physics particle:
  - Single-pass construction avoids intermediate struct allocation ceremony.
  - Every observed order contributes one unit of carrier mass; quantity remains encoded in Position Y.
  - Position phase uses Option 3 (Bid/Ask spread quantum boundary).
  - Returns the particle array alongside token content IDs expected by Metal's merge pipeline.

The caller collects levels by walking a Go map, so the two orders in the argument slices
are shuffled on every tick. Nothing stamped onto a particle may read the slice index:
every coordinate is derived either from the order itself or from a rank recomputed under
a total ordering.

buyExcitation and sellExcitation are the side's Hawkes self-excitation expressed as a
multiple of its own immigrant baseline, (lambda - mu) / mu. They are the forcing term:
a side in a self-exciting cascade enters above unit energy and therefore drives the wave
field harder than the side it is arriving against.
*/
func (tokenizer *Tokenizer) NewBatch(
	bidOrders []*mgrbook.Order,
	askOrders []*mgrbook.Order,
	midPrice float64,
	buyExcitation float64,
	sellExcitation float64,
	symbol string,
) ([]pfluid.Particle, []uint32, error) {
	numBids := len(bidOrders)
	numAsks := len(askOrders)
	totalOrders := numBids + numAsks

	if totalOrders == 0 || midPrice <= 0 {
		return nil, nil, nil
	}

	if !finiteNonnegative(buyExcitation) || !finiteNonnegative(sellExcitation) {
		return nil, nil, fmt.Errorf(
			"manifold: Hawkes excitation must be finite, nonnegative, and representable as binary32",
		)
	}

	// 1. Combine bid and ask orders while retaining side information.
	orders := make([]*mgrbook.Order, 0, totalOrders)
	entries := make([]orderEntry, 0, totalOrders)

	for _, o := range bidOrders {
		orders = append(orders, o)
		entries = append(entries, orderEntry{order: o, side: mgrbook.Bid})
	}
	for _, o := range askOrders {
		orders = append(orders, o)
		entries = append(entries, orderEntry{order: o, side: mgrbook.Ask})
	}

	// 2. Derive dynamic log-space extents relative to midPrice.
	logPriceMin, logPriceMax, logSizeMin, logSizeMax, ok := geometryBounds(
		orders,
		midPrice,
	)

	if !ok {
		return nil, nil, nil
	}

	// 3. Resolve symbol index for Metal token identification.
	symbolIdx, ok := universeIndex(tokenizer.universe, symbol)

	if !ok {
		return nil, nil, fmt.Errorf("manifold: symbol %s is outside the tokenizer universe", symbol)
	}

	// 4. Accumulate this batch into the symbol's content-frequency scale, then read
	// the converged scale back out. Omega is normalized against it rather than
	// against this batch's own extents.
	scale, err := tokenizer.omegaScale(symbol, orders, midPrice)

	if err != nil {
		return nil, nil, err
	}

	ranks := orderAgeRanks(orders)
	queue := queueRanks(entries)

	grid := tokenizer.config.Grid
	extent := max(grid.X, grid.Y, grid.Z)
	spacing := float32(1.0 / float64(extent))
	particles := make([]pfluid.Particle, 0, len(orders))
	contentIDs := make([]uint32, 0, len(orders))

	// 4. Single-pass generation: maps *mgrbook.Order straight to pfluid.Particle & tokenID.
	for seq, entry := range entries {
		price := entry.order.LimitPrice.Float64()
		quantity := entry.order.Quantity.Float64()

		if quantity <= 0 || price <= 0 {
			continue
		}

		tokenID := packContent(symbolIdx, entry.side)

		// Hawkes is the forcing element. It rides on top of the unit, so a quiet side
		// enters at unit energy and an excited side enters hot and drives the field.
		energy := unitOscillatorEnergy + float32(buyExcitation)

		if entry.side == mgrbook.Ask {
			energy = unitOscillatorEnergy + float32(sellExcitation)
		}

		if energy == 0 {
			continue
		}

		// Queue position is the order's rank in its own side's price-time priority,
		// never its position in the shuffled input slice.
		sideCount := numBids

		if entry.side == mgrbook.Ask {
			sideCount = numAsks
		}

		particles = append(particles, pfluid.Particle{
			Position: marketPosition(
				price, midPrice, quantity, ranks[seq], len(orders),
				logPriceMin, logPriceMax, logSizeMin, logSizeMax,
				grid, spacing,
			),
			Velocity: pfluid.Vector{},
			Mass:     unitCarrierMass,
			// Heat is earned, never stamped. It accumulates from inelastic collision and
			// from Planck relaxation draining the oscillator store, so tokens enter cold.
			Heat:   0,
			Energy: energy,
			Phase:  positionPhase(queue[seq], sideCount, entry.side),
			Omega: positionOmega(
				price,
				midPrice,
				scale,
				tokenizer.config,
			),
		})

		contentIDs = append(contentIDs, tokenID)
	}

	return particles, contentIDs, nil
}

/*
packContent encodes (symbolIndex << 1) | sideBit into a 32-bit integer.

Why:
  - Bit 0 indicates order direction (0 = Bid, 1 = Ask).
  - Bits 1–15 encode the symbol index.
  - The resulting value fits within 16 bits (0xFFFF), matching Metal's `merge_compute_keys` contract:
    `ulong token = (ulong)(content[gid] & 0xFFFFu);`
*/
func packContent(symbolIndex uint32, side mgrbook.BookDirection) uint32 {
	sideBit := uint32(0)
	if side == mgrbook.Ask {
		sideBit = 1
	}
	return ((symbolIndex & symbolIndexMask) << 1) | sideBit
}

/*
universeIndex performs a binary search on the pre-sorted symbol universe to locate a symbol's index.

Why:
  - Guarantees O(log N) lookup without allocating dynamic heap maps on every tick.
*/
func universeIndex(names []string, symbol string) (uint32, bool) {
	index := sort.SearchStrings(names, symbol)
	if index == len(names) || names[index] != symbol {
		return 0, false
	}
	return uint32(index), true
}

/*
sortedUniverse returns a lexicographically sorted copy of the symbol universe.

Why:
  - Establishes a stable index basis across ticks so token IDs stay identical over time.
*/
func sortedUniverse(names []string) []string {
	universe := append([]string(nil), names...)
	sort.Strings(universe)
	return universe
}

/*
geometryBounds computes the minimum and maximum log-price (relative to midPrice) and log-size.

Why:
  - Normalizing price relative to midPrice log(Price / MidPrice) keeps the bid-ask spread
    centered around 0.5 regardless of absolute price scale (e.g. $0.001 vs $100,000).
  - Normalizing order size in log-space prevents massive orders from squishing retail orders to zero.
  - Pre-calculates log(midPrice) outside the loop to save float division operations.
  - Extracts float64 values safely from *decimal.Decimal struct fields.
  - Guards against empty order books or non-positive midPrices by returning ok = false.
*/
func geometryBounds(
	orders []*mgrbook.Order,
	midPrice float64,
) (
	logPriceMin, logPriceMax, logSizeMin, logSizeMax float64,
	ok bool,
) {
	if len(orders) == 0 || midPrice <= 0 {
		return 0, 0, 0, 0, false
	}

	logMid := math.Log(midPrice)
	logPriceMin = math.Inf(1)
	logPriceMax = math.Inf(-1)
	logSizeMin = math.Inf(1)
	logSizeMax = math.Inf(-1)

	for _, resting := range orders {
		price := resting.LimitPrice.Float64()
		quantity := resting.Quantity.Float64()

		if price <= 0 || quantity <= 0 || math.IsNaN(price) || math.IsNaN(quantity) ||
			math.IsInf(price, 0) || math.IsInf(quantity, 0) {
			return 0, 0, 0, 0, false
		}

		logPrice := math.Log(price) - logMid
		logSize := math.Log(quantity)

		logPriceMin = min(logPriceMin, logPrice)
		logPriceMax = max(logPriceMax, logPrice)
		logSizeMin = min(logSizeMin, logSize)
		logSizeMax = max(logSizeMax, logSize)
	}

	return logPriceMin, logPriceMax, logSizeMin, logSizeMax, true
}

func finiteNonnegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) &&
		value >= 0 && value <= math.MaxFloat32
}

/*
omegaScale folds one batch into the symbol's accumulated log-distance scale and returns
the root mean square of log(Price / MidPrice) over every order the tokenizer has ever
seen for that symbol.

Why:
  - Omega is the particle's identity, so its normalization cannot be a statistic of
    whichever orders happen to share the current batch. Normalizing against per-batch
    extents means one distant order arriving or cancelling re-assigns every other
    particle's natural frequency, and the carrier cannot hold an attractor well at a
    frequency that moves underneath it.
  - A running mean square converges: after the opening ticks the scale barely moves, so
    the same distance from mid keeps mapping to the same frequency across ticks. That
    convergence is the property Psi(omega) needs, and it is also the trade-off — the
    scale follows a volatility regime change only as fast as the sample count dilutes.
  - Measuring dispersion about zero rather than about a sample mean keeps mid price at
    the centre of the lattice, which is the only physically meaningful origin here.
  - Kahan-compensated accumulation preserves the low-order terms of a stream that runs
    for millions of orders.
  - Every resting order is sampled again on every tick, so the scale is weighted by how
    long orders sit rather than by how often they arrive.
  - A mean square is pulled by orders far from mid. At market.l3_depth 10 the book is
    tight enough that this does not bite, but a much deeper subscription would let a few
    far-parked orders compress the near-touch levels into a sliver of the lattice.
*/
func (tokenizer *Tokenizer) omegaScale(
	symbol string,
	orders []*mgrbook.Order,
	midPrice float64,
) (float64, error) {
	logMid := math.Log(midPrice)

	tokenizer.mutex.Lock()
	defer tokenizer.mutex.Unlock()

	accumulated, ok := tokenizer.scales[symbol]

	if !ok {
		accumulated = adaptive.NewAccumulator()
		tokenizer.scales[symbol] = accumulated
	}

	var output adaptive.AccumulatorOutput

	for _, resting := range orders {
		price := resting.LimitPrice.Float64()

		if price <= 0 {
			continue
		}

		deviation := math.Log(price) - logMid
		measured, err := accumulated.Measure(deviation * deviation)

		if err != nil {
			return 0, err
		}

		output = measured
	}

	if output.Count == 0 {
		return 0, nil
	}

	scale := math.Sqrt(output.Value / float64(output.Count))
	tokenizer.converged[symbol] = scale

	return scale, nil
}

/*
positionOmega maps an order's log distance from mid onto the content-frequency lattice.

Why:
  - The only inputs are the order's own price, the mid it is quoted against, and the
    symbol's converged scale, so two identical orders one tick apart receive the same
    frequency. Nothing about the rest of the batch enters.
  - The sign is meaningful rather than incidental: bids sit below the lattice centre and
    asks above it, with mid price at the centre. Extent normalization gave a lopsided
    book the whole lattice, so its bids could land in ask territory.
  - tanh is a monotone bounded map, chosen so distance ordering survives and no particle
    can rail. Extent normalization pinned the nearest and furthest order to exactly
    OmegaMin and OmegaMax every tick, making them spuriously resonant with each other;
    tanh approaches the bounds without reaching them. Its tail compression is a
    consequence of that choice, not a derived market law.
*/
func positionOmega(
	price, midPrice, scale float64,
	config pfluid.Config,
) float32 {
	centre := (config.OmegaMax + config.OmegaMin) / 2

	if !(scale > 0) {
		return centre
	}

	halfSpan := (config.OmegaMax - config.OmegaMin) / 2
	deviation := math.Log(price) - math.Log(midPrice)

	return centre + halfSpan*float32(math.Tanh(deviation/scale))
}

/*
queueRanks ranks every entry within its own side by exchange price-time priority and
returns each entry's rank (best resting order = 0).

Why:
  - Phase is the sequence coordinate of the Sensorium mapping, so it has to belong to
    the order. The caller assembles its slices by walking a Go map, whose iteration
    order is randomized per tick, so a slice index re-randomizes a resting order's
    phase on every batch and destroys the relative phase offsets that carry position.
  - Bids rank best-first by descending price, asks best-first by ascending price.
  - Timestamp then order ID complete the ordering, so orders sharing a price and a
    timestamp still rank deterministically instead of inheriting map order through an
    unstable comparison.
*/
func queueRanks(entries []orderEntry) []int {
	indices := make([]int, len(entries))

	for index := range indices {
		indices[index] = index
	}

	sort.Slice(indices, func(left, right int) bool {
		first := entries[indices[left]]
		second := entries[indices[right]]

		if first.side != second.side {
			return first.side == mgrbook.Bid
		}

		if order := first.order.LimitPrice.Cmp(second.order.LimitPrice); order != 0 {
			if first.side == mgrbook.Bid {
				return order > 0
			}

			return order < 0
		}

		if !first.order.Timestamp.Equal(second.order.Timestamp) {
			return first.order.Timestamp.Before(second.order.Timestamp)
		}

		return first.order.ID < second.order.ID
	})

	ranks := make([]int, len(entries))
	sideRanks := map[mgrbook.BookDirection]int{}

	for _, index := range indices {
		side := entries[index].side
		ranks[index] = sideRanks[side]
		sideRanks[side]++
	}

	return ranks
}

/*
orderAgeRanks ranks orders from 0 to N-1 based on ascending timestamps (oldest = rank 0).

Why:
  - Maps queue age directly to the Z-axis in physical space.
  - Uses the `Timestamp` field on `*mgrbook.Order`.
  - Order ID breaks timestamp ties. A stable sort preserves walk order instead, and walk
    order here is Go map iteration order, so every same-millisecond cluster of orders
    would shuffle its Z coordinates between ticks.
*/
func orderAgeRanks(orders []*mgrbook.Order) []int {
	indices := make([]int, len(orders))
	for i := range indices {
		indices[i] = i
	}

	sort.Slice(indices, func(left, right int) bool {
		first := orders[indices[left]]
		second := orders[indices[right]]

		if !first.Timestamp.Equal(second.Timestamp) {
			return first.Timestamp.Before(second.Timestamp)
		}

		return first.ID < second.ID
	})

	ranks := make([]int, len(orders))
	for rank, index := range indices {
		ranks[index] = rank
	}
	return ranks
}

/*
unitInRange maps a continuous value onto the range [0.0, 1.0] across observed min/max bounds.

Why:
  - Handles degenerate spans (min == max) by anchoring the value at the 0.5 midline so single
    observations do not get pinned artificially to domain edges.
  - Clamps the result. The bounds are derived by one expression and applied by another, and
    a value sitting exactly on a bound can land a few ULP outside it, which is enough to
    push a particle off the PIC grid.
*/
func unitInRange(value, minimum, maximum float64) float64 {
	if maximum <= minimum {
		return 0.5
	}
	return min(max((value-minimum)/(maximum-minimum), 0), 1)
}

/*
marketPosition computes the 3D spatial position vector of a resting order.

Why:
  - X axis: Relative log price. Bids span [0.0, 0.5), Asks span (0.5, 1.0]. MidPrice sits at 0.5.
  - Y axis: Log size distribution.
  - Z axis: Queue age rank.
  - Scales positions into grid units using `spacing = 1.0 / extent` so coordinates align perfectly
    with Metal PIC (Particle-in-Cell) trilinear interpolation kernels.
*/
func marketPosition(
	price, midPrice, quantity float64,
	ageRank, ageCount int,
	logPriceMin, logPriceMax, logSizeMin, logSizeMax float64,
	grid pfluid.Grid,
	spacing float32,
) pfluid.Vector {
	// Matches geometryBounds term for term. log(price/mid) rounds differently from
	// log(price) - log(mid), and the difference is what put the cheapest order a few
	// ULP below zero on the X axis.
	xUnit := unitInRange(math.Log(price)-math.Log(midPrice), logPriceMin, logPriceMax)
	yUnit := unitInRange(math.Log(quantity), logSizeMin, logSizeMax)
	zUnit := 0.5

	if ageCount > 1 {
		zUnit = float64(ageRank) / float64(ageCount-1)
	}

	return pfluid.Vector{
		X: float32(xUnit) * float32(max(grid.X-1, 0)) * spacing,
		Y: float32(yUnit) * float32(max(grid.Y-1, 0)) * spacing,
		Z: float32(zUnit) * float32(max(grid.Z-1, 0)) * spacing,
	}
}

/*
positionPhase derives a phase angle phi in [0, 2pi) using the Bid-Ask spread as a quantum boundary.

Why:
  - Bids (buy side) span phase range [0, pi).
  - Asks (sell side) span phase range [pi, 2pi).
  - Destructive interference at the boundary (e^(i*0) + e^(i*pi) = 0) creates a zero-density node
    in the spread gap, preventing pilot-wave guidance currents from artificially bleeding across sides.
*/
func positionPhase(sideSeq int, sideCount int, side mgrbook.BookDirection) float32 {
	if sideCount < 1 {
		sideCount = 1
	}

	progress := float64(sideSeq) / float64(sideCount)

	basePhase := 0.0
	if side == mgrbook.Ask {
		basePhase = math.Pi
	}

	phase := basePhase + math.Pi*progress
	return float32(math.Mod(phase, 2*math.Pi))
}

/*
Scale reports a symbol's most recently converged RMS log distance from mid,
which is the book's own measure of how far its price has to move to be
distinguishable from its resting depth.
*/
func (tokenizer *Tokenizer) Scale(symbol string) (float64, bool) {
	tokenizer.mutex.Lock()
	defer tokenizer.mutex.Unlock()

	scale, ok := tokenizer.converged[symbol]

	return scale, ok && scale > 0
}
