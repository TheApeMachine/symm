package manifold

import (
	"fmt"
	"math"
	"sort"

	mgrbook "github.com/theapemachine/api-go/v2/pkg/book"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
injectHeatFraction initializes thermal heat as a small fraction (1e-4) of kinetic energy
to implement "cold injection" where newly spawned particles do not immediately overheat the system.
symbolIndexMask limits symbol indices to 15 bits so the packed token ID stays cleanly within
Metal's 16-bit token bitmask (0xFFFF).
*/
const (
	injectHeatFraction float32 = 1e-4
	symbolIndexMask    uint32  = 0x7fff
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
  - Mass: Order quantity, allowing large liquidity walls to exert stronger gravitational/collision forces.
  - Energy: Hawkes arrival intensities, exciting particles during periods of high market activity.
  - Phase: Bid-Ask spread phase boundary ([0, pi) for Bids, [pi, 2pi) for Asks).
  - Omega: Uniform frequency distribution spanning the GPE spectral lattice.
*/
type Tokenizer struct {
	config   pfluid.Config
	universe []string
}

/*
NewTokenizer constructs a Tokenizer bound to a fluid configuration and a known symbol universe.
The symbol universe is automatically sorted to guarantee a stable, deterministic token basis
for GPU memory merging regardless of tick arrival order.
*/
func NewTokenizer(config pfluid.Config, symbols []string) *Tokenizer {
	return &Tokenizer{
		config:   config,
		universe: sortedUniverse(symbols),
	}
}

/*
NewBatch converts resting L3 bid and ask orders into an appendable particle batch in a single pass.

It merges bids and asks, extracts Decimal values into float64, derives dynamic geometry bounds,
resolves the symbol token ID, computes empirical age ranks, and projects every order directly into a physics particle:
  - Single-pass construction avoids intermediate struct allocation ceremony.
  - Order quantity directly dictates particle mass to preserve gravitational and collision kinetics on GPU.
  - Position phase uses Option 3 (Bid/Ask spread quantum boundary).
  - Returns the particle array alongside token content IDs expected by Metal's merge pipeline.
*/
func (tokenizer Tokenizer) NewBatch(
	bidOrders []*mgrbook.Order,
	askOrders []*mgrbook.Order,
	midPrice float64,
	buyIntensity float64,
	sellIntensity float64,
	symbol string,
) ([]pfluid.Particle, []uint32, error) {
	numBids := len(bidOrders)
	numAsks := len(askOrders)
	totalOrders := numBids + numAsks
	if totalOrders == 0 || midPrice <= 0 {
		return nil, nil, nil
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
	logPriceMin, logPriceMax, logSizeMin, logSizeMax, ok := geometryBounds(orders, midPrice)
	if !ok {
		return nil, nil, nil
	}

	// 3. Resolve symbol index for Metal token identification.
	symbolIdx, ok := universeIndex(tokenizer.universe, symbol)

	if !ok {
		return nil, nil, fmt.Errorf("manifold: symbol %s is outside the tokenizer universe", symbol)
	}

	ranks := orderAgeRanks(orders)
	circle := uint32(len(orders))

	grid := tokenizer.config.Grid
	extent := max(grid.X, grid.Y, grid.Z)
	spacing := float32(1.0 / float64(extent))
	span := tokenizer.config.OmegaMax - tokenizer.config.OmegaMin

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

		// Distribute omega evenly across the spectral GPE lattice [OmegaMin, OmegaMax].
		omega := tokenizer.config.OmegaMin +
			(float32(seq)+0.5)/float32(circle)*span

		// Hawkes intensity sets particle energy based on side-specific market excitement.
		energy := float32(buyIntensity)
		if entry.side == mgrbook.Ask {
			energy = float32(sellIntensity)
		}

		// Calculate relative queue position within its respective side (bids vs asks)
		sideSeq := seq
		sideCount := numBids
		if entry.side == mgrbook.Ask {
			sideSeq = seq - numBids
			sideCount = numAsks
		}

		particles = append(particles, pfluid.Particle{
			Position: marketPosition(
				price, midPrice, quantity, ranks[seq], len(orders),
				logPriceMin, logPriceMax, logSizeMin, logSizeMax,
				grid, spacing,
			),
			Velocity: pfluid.Vector{},
			Mass:     float32(quantity), // Mass scales with liquidity size
			Heat:     energy * injectHeatFraction,
			Energy:   energy,
			Phase:    positionPhase(sideSeq, sideCount, entry.side),
			Omega:    omega,
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
) (logPriceMin, logPriceMax, logSizeMin, logSizeMax float64, ok bool) {
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

		if price <= 0 || quantity <= 0 {
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

/*
orderAgeRanks ranks orders from 0 to N-1 based on ascending timestamps (oldest = rank 0).

Why:
  - Maps queue priority directly to the Z-axis in physical space.
  - Uses the `Timestamp` field on `*mgrbook.Order`.
  - Stable sort preserves walk order for orders sharing identical timestamps, ensuring smooth Z-span.
*/
func orderAgeRanks(orders []*mgrbook.Order) []int {
	indices := make([]int, len(orders))
	for i := range indices {
		indices[i] = i
	}

	sort.SliceStable(indices, func(left, right int) bool {
		return orders[indices[left]].Timestamp.Before(orders[indices[right]].Timestamp)
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
*/
func unitInRange(value, minimum, maximum float64) float64 {
	if maximum <= minimum {
		return 0.5
	}
	return (value - minimum) / (maximum - minimum)
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
	xUnit := unitInRange(math.Log(price/midPrice), logPriceMin, logPriceMax)
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
