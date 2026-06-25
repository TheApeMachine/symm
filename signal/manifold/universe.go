package manifold

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

/*
Coords maps market events into the 3D torus: X depth, Y instrument lane, Z cross-asset rank.
*/
type Coords struct {
	cellX uint32
	cellY uint32
	cellZ uint32
	posX  float64
	posY  float64
	posZ  float64
}

/*
UniverseState tracks one instrument lane projection into the manifold lattice.
*/
type UniverseState struct {
	symbol      string
	base        string
	lane        InstrumentLane
	rank        uint32
	midPrice    float64
	tickSize    float64
	tickPinned  bool
	halfWidth   int
	lastPrice   float64
	lastEventAt time.Time
	returns     atomic.Pointer[[]float64]
	tradeQtys   atomic.Pointer[[]float64]
	bookReady   bool
	book        BookUpdate
	bookDepth   int
}

type Universe struct {
	states           sync.Map
	config           mkernel.Config
	ranks            atomic.Pointer[map[string]uint32]
	rankVersion      uint64
	defaultTickSize  float64
	fluidTickSize    float64
	defaultHalfWidth int
	fluidHalfWidth   int
	defaultBookDepth int
}

func NewUniverse(kernelConfig mkernel.Config) (*Universe, error) {
	tickSize := 1.0 / float64(math.Pow(2, float64(kernelConfig.GridX)))

	if configuredTick := viper.GetFloat64("signals.manifold.tick_size"); configuredTick > 0 {
		tickSize = configuredTick
	}

	halfWidth := viper.GetInt("signals.manifold.grid_half_width")
	fluidHalfWidth := halfWidth
	fluidTickSize := tickSize

	if halfWidth <= 0 {
		halfWidth = 10 * 3
	}

	if fluidHalfWidth <= 0 {
		fluidHalfWidth = halfWidth
	}

	depth := 10

	u := &Universe{
		config:           kernelConfig,
		defaultTickSize:  tickSize,
		fluidTickSize:    fluidTickSize,
		defaultHalfWidth: halfWidth,
		fluidHalfWidth:   fluidHalfWidth,
		defaultBookDepth: depth,
	}
	initialRanks := make(map[string]uint32)
	u.ranks.Store(&initialRanks)

	return u, nil
}

func (universe *Universe) stateKey(identity InstrumentIdentity) string {
	return fmt.Sprintf("%s:%d", identity.Base, identity.Lane)
}

func (universe *Universe) loadIdentity(identity InstrumentIdentity) *UniverseState {
	key := universe.stateKey(identity)

	raw, _ := universe.states.LoadOrStore(key, &UniverseState{
		symbol:    identity.Symbol,
		base:      identity.Base,
		lane:      identity.Lane,
		tickSize:  universe.defaultTickSize,
		halfWidth: universe.defaultHalfWidth,
		bookDepth: universe.defaultBookDepth,
	})

	state, ok := raw.(*UniverseState)

	if !ok {
		return nil
	}

	state.symbol = identity.Symbol
	state.base = identity.Base
	state.lane = identity.Lane

	if state.tickSize <= 0 {
		state.tickSize = universe.fluidTickSize
	}

	if state.halfWidth <= 0 {
		state.halfWidth = universe.fluidHalfWidth
	}

	if state.bookDepth <= 0 {
		state.bookDepth = universe.defaultBookDepth
	}

	return state
}

func (universe *Universe) loadSymbol(symbol string) *UniverseState {
	identity, err := SpotIdentityFromPair(symbol)

	if err != nil {
		return nil
	}

	return universe.loadIdentity(identity)
}

func (universe *Universe) registerSymbols(symbols []string) {
	for _, symbol := range symbols {
		spotIdentity, err := SpotIdentityFromPair(symbol)

		if err != nil {
			continue
		}

		universe.loadIdentity(spotIdentity)
	}

	universe.recomputeRanks()
}

func (universe *Universe) recomputeRanks() {
	type rankedBase struct {
		base   string
		energy float64
	}

	ranked := make([]rankedBase, 0)
	seen := make(map[string]struct{})

	universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != InstrumentLaneSpot {
			return true
		}

		if _, exists := seen[state.base]; exists {
			return true
		}

		seen[state.base] = struct{}{}

		ranked = append(ranked, rankedBase{
			base:   state.base,
			energy: medianAbsolute(state.GetReturns()),
		})

		return true
	})

	sort.Slice(ranked, func(left, right int) bool {
		if ranked[left].energy == ranked[right].energy {
			return ranked[left].base < ranked[right].base
		}

		return ranked[left].energy > ranked[right].energy
	})

	newRanks := make(map[string]uint32, len(ranked))

	symbolCount := len(ranked)
	gridZ := universe.config.GridZ

	for index, row := range ranked {
		var rank uint32

		switch {
		case symbolCount <= 1:
			rank = 0
		case symbolCount <= int(gridZ):
			rank = uint32(index)
		default:
			rank = uint32(uint64(index) * uint64(gridZ-1) / uint64(symbolCount-1))
		}

		newRanks[row.base] = rank
	}

	universe.ranks.Store(&newRanks)
	universe.rankVersion++
}

func (universe *Universe) coords(state *UniverseState, priceOffsetTicks float64) Coords {
	var rank uint32
	ranksPtr := universe.ranks.Load()
	if ranksPtr != nil {
		if r, ok := (*ranksPtr)[state.base]; ok {
			rank = r
		}
	}

	state.rank = rank

	cellX := wrapCell(int(math.Round(priceOffsetTicks))+state.halfWidth, int(universe.config.GridX))
	cellY := wrapCell(int(state.lane), int(universe.config.GridY))
	cellZ := wrapCell(int(rank), int(universe.config.GridZ))

	posX := float64(cellX) + 0.5

	if cellX == 0 {
		posX = 1
	}

	return Coords{
		cellX: uint32(cellX),
		cellY: uint32(cellY),
		cellZ: uint32(cellZ),
		posX:  posX,
		posY:  float64(cellY),
		posZ:  float64(cellZ) + 0.5,
	}
}

func wrapCell(value, modulus int) int {
	if modulus <= 0 {
		return 0
	}

	remainder := value % modulus

	if remainder < 0 {
		remainder += modulus
	}

	return remainder
}

func medianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (math.Abs(sorted[middle-1]) + math.Abs(sorted[middle])) / 2
	}

	return math.Abs(sorted[middle])
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}

func visibleBookQty(state *UniverseState) float64 {
	total := 0.0

	for _, level := range state.book.Bids {
		total += level.Qty
	}

	for _, level := range state.book.Asks {
		total += level.Qty
	}

	return total
}

func (universe *Universe) tickSizeFallback() float64 {
	if universe.defaultTickSize > 0 {
		return universe.defaultTickSize
	}

	return universe.fluidTickSize
}

func (state *UniverseState) configureTickFromBook(
	bids, asks []BookLevel,
	tickFallback float64,
) error {
	bidPrices := make([]float64, len(bids))
	askPrices := make([]float64, len(asks))

	for index, level := range bids {
		bidPrices[index] = level.Price
	}

	for index, level := range asks {
		askPrices[index] = level.Price
	}

	fallback := state.tickSize

	if fallback <= 0 {
		fallback = tickFallback
	}

	tickSize, err := resolveBookTickSize(bidPrices, askPrices, fallback)

	if err != nil {
		return fmt.Errorf("manifold: tick size is zero")
	}

	state.tickSize = tickSize

	return nil
}

func (state *UniverseState) GetReturns() []float64 {
	ptr := state.returns.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

func (state *UniverseState) SetReturns(returns []float64) {
	state.returns.Store(&returns)
}

func (state *UniverseState) AppendReturn(val float64, capacity int) {
	var old []float64
	ptr := state.returns.Load()
	if ptr != nil {
		old = *ptr
	}

	newSlice := make([]float64, 0, len(old)+1)
	newSlice = append(newSlice, old...)
	newSlice = append(newSlice, val)

	if len(newSlice) > capacity {
		newSlice = newSlice[len(newSlice)-capacity:]
	}

	state.returns.Store(&newSlice)
}

func (state *UniverseState) GetTradeQtys() []float64 {
	ptr := state.tradeQtys.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

func (state *UniverseState) SetTradeQtys(qtys []float64) {
	state.tradeQtys.Store(&qtys)
}

func (state *UniverseState) AppendTradeQty(val float64, capacity int) {
	if val <= 0 {
		return
	}
	var old []float64
	ptr := state.tradeQtys.Load()
	if ptr != nil {
		old = *ptr
	}

	newSlice := make([]float64, 0, len(old)+1)
	newSlice = append(newSlice, old...)
	newSlice = append(newSlice, val)

	if len(newSlice) > capacity {
		newSlice = newSlice[len(newSlice)-capacity:]
	}

	state.tradeQtys.Store(&newSlice)
}

func (state *UniverseState) whaleQtyThreshold() float64 {
	tradeQtys := state.GetTradeQtys()
	returns := state.GetReturns()

	if len(tradeQtys) < 3 {
		return math.Inf(1)
	}

	reference := median(tradeQtys)

	if state.bookReady && state.midPrice > 0 {
		visible := visibleBookQty(state)

		if visible > 0 && state.bookDepth > 0 {
			reference = math.Max(reference, visible/float64(state.bookDepth))
		}
	}

	surge := 1.0 + medianAbsolute(returns)

	return reference * surge
}

func (state *UniverseState) recordTradeQty(qty float64, capacity int) {
	if capacity <= 0 {
		capacity = 64
	}
	state.AppendTradeQty(qty, capacity)
}
