package manifold

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/futures"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/physics"
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
	lane        krakenmarket.InstrumentLane
	rank        uint32
	midPrice    float64
	tickSize    float64
	halfWidth   int
	lastPrice   float64
	lastEventAt time.Time
	returns     []float64
	tradeQtys   []float64
	bookReady   bool
	book        krakenmarket.Book
	bookDepth   int
}

type universe struct {
	states      sync.Map
	config      physics.Config
	ranks       map[string]uint32
	rankVersion uint64
}

func newUniverse(config physics.Config) (*universe, error) {
	tickSize := viper.GetFloat64("signals.manifold.tick_size")

	if tickSize <= 0 {
		tickSize = viper.GetFloat64("signals.fluid.tick_size")
	}

	if tickSize <= 0 {
		return nil, fmt.Errorf("manifold: tick_size must be positive")
	}

	halfWidth := viper.GetInt("signals.manifold.grid_half_width")

	if halfWidth <= 0 {
		halfWidth = viper.GetInt("signals.fluid.grid_half_width")
	}

	if halfWidth <= 0 {
		return nil, fmt.Errorf("manifold: grid_half_width must be positive")
	}

	depth := viper.GetInt("market.book_depth_levels")

	if depth <= 0 {
		return nil, fmt.Errorf("manifold: market.book_depth_levels must be positive")
	}

	return &universe{
		config: config,
		ranks:  make(map[string]uint32),
	}, nil
}

func (universe *universe) stateKey(identity krakenmarket.InstrumentIdentity) string {
	return fmt.Sprintf("%s:%d", identity.Base, identity.Lane)
}

func (universe *universe) loadIdentity(identity krakenmarket.InstrumentIdentity) *UniverseState {
	key := universe.stateKey(identity)

	raw, _ := universe.states.LoadOrStore(key, &UniverseState{
		symbol:    identity.Symbol,
		base:      identity.Base,
		lane:      identity.Lane,
		tickSize:  viper.GetFloat64("signals.manifold.tick_size"),
		halfWidth: viper.GetInt("signals.manifold.grid_half_width"),
		bookDepth: viper.GetInt("market.book_depth_levels"),
	})

	state, ok := raw.(*UniverseState)

	if !ok {
		return nil
	}

	state.symbol = identity.Symbol
	state.base = identity.Base
	state.lane = identity.Lane

	if state.tickSize <= 0 {
		state.tickSize = viper.GetFloat64("signals.fluid.tick_size")
	}

	if state.halfWidth <= 0 {
		state.halfWidth = viper.GetInt("signals.fluid.grid_half_width")
	}

	if state.bookDepth <= 0 {
		state.bookDepth = viper.GetInt("market.book_depth_levels")
	}

	return state
}

func (universe *universe) loadSymbol(symbol string) *UniverseState {
	identity, err := krakenmarket.SpotIdentityFromPair(symbol)

	if err != nil {
		return nil
	}

	return universe.loadIdentity(identity)
}

func (universe *universe) registerSymbols(symbols []string) {
	catalog := futures.SharedCatalog()
	catalogLoaded := catalog.EnsureLoaded(context.Background()) == nil

	for _, symbol := range symbols {
		spotIdentity, err := krakenmarket.SpotIdentityFromPair(symbol)

		if err != nil {
			continue
		}

		universe.loadIdentity(spotIdentity)

		if !catalogLoaded {
			continue
		}

		products, productErr := catalog.ProductsForSpotPair(symbol)

		if productErr != nil {
			continue
		}

		for _, productID := range products {
			futuresIdentity, futuresErr := krakenmarket.FuturesIdentityFromProduct(productID)

			if futuresErr != nil {
				continue
			}

			universe.loadIdentity(futuresIdentity)
		}
	}

	universe.recomputeRanks()
}

func (universe *universe) recomputeRanks() {
	type rankedBase struct {
		base   string
		energy float64
	}

	ranked := make([]rankedBase, 0)
	seen := make(map[string]struct{})

	universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != krakenmarket.InstrumentLaneSpot {
			return true
		}

		if _, exists := seen[state.base]; exists {
			return true
		}

		seen[state.base] = struct{}{}

		ranked = append(ranked, rankedBase{
			base:   state.base,
			energy: medianAbsolute(state.returns),
		})

		return true
	})

	sort.Slice(ranked, func(left, right int) bool {
		if ranked[left].energy == ranked[right].energy {
			return ranked[left].base < ranked[right].base
		}

		return ranked[left].energy > ranked[right].energy
	})

	universe.ranks = make(map[string]uint32, len(ranked))

	for index, row := range ranked {
		rank := uint32(index)

		if rank >= universe.config.GridZ {
			rank = universe.config.GridZ - 1
		}

		universe.ranks[row.base] = rank
	}

	universe.rankVersion++
}

func (universe *universe) coords(state *UniverseState, priceOffsetTicks float64) Coords {
	rank, ok := universe.ranks[state.base]

	if !ok {
		rank = 0
	}

	state.rank = rank

	cellX := wrapCell(int(math.Round(priceOffsetTicks))+state.halfWidth, int(universe.config.GridX))
	cellY := wrapCell(int(state.lane), int(universe.config.GridY))
	cellZ := wrapCell(int(rank), int(universe.config.GridZ))

	spacing := universe.config.GridSpacing()

	return Coords{
		cellX: uint32(cellX),
		cellY: uint32(cellY),
		cellZ: uint32(cellZ),
		posX:  float64(cellX) * spacing,
		posY:  float64(cellY) * spacing,
		posZ:  float64(cellZ) * spacing,
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

func spotSymbolForBase(universe *universe, base string) string {
	if base == "" {
		return ""
	}

	key := fmt.Sprintf("%s:%d", base, krakenmarket.InstrumentLaneSpot)
	raw, ok := universe.states.Load(key)

	if !ok {
		return ""
	}

	state, stateOk := raw.(*UniverseState)

	if !stateOk || state.symbol == "" {
		return ""
	}

	return state.symbol
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

func (state *UniverseState) whaleQtyThreshold() float64 {
	if len(state.tradeQtys) < 3 {
		return math.Inf(1)
	}

	reference := median(state.tradeQtys)

	if state.bookReady && state.midPrice > 0 {
		visible := visibleBookQty(state)

		if visible > 0 && state.bookDepth > 0 {
			reference = math.Max(reference, visible/float64(state.bookDepth))
		}
	}

	surge := 1.0 + medianAbsolute(state.returns)

	return reference * surge
}

func (state *UniverseState) recordTradeQty(qty float64, capacity int) {
	if qty <= 0 {
		return
	}

	state.tradeQtys = append(state.tradeQtys, qty)

	if capacity <= 0 {
		capacity = 64
	}

	if len(state.tradeQtys) > capacity {
		state.tradeQtys = state.tradeQtys[len(state.tradeQtys)-capacity:]
	}
}
