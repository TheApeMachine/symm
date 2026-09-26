package store

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/* manifoldInput is one immutable full venue queue with observed forcing. */
type manifoldInput struct {
	symbol          string
	epoch, sequence int64
	orders          []manifoldOrder
	book            bool
	excitation      [2]float64
	known           [2]bool
}

type manifoldOrder struct {
	id              string
	bid             bool
	price, quantity float64
	rank            uint32
}

/*
	manifoldMarket retains resident identities and the observed coordinate

bounds for one symbol. Bounds define geometry, never market probability.
*/
type manifoldMarket struct {
	identities                             map[string]int64
	priceLow, priceHigh, sizeLow, sizeHigh float64
	initialized                            bool
}

func (input *manifoldInput) read(args Manifold_write_Params) (bool, error) {
	changed := false
	excitation, err := args.Excitation()

	if err != nil {
		return false, errnie.Error(err)
	}
	present, err := args.Present()

	if err != nil {
		return false, errnie.Error(err)
	}
	if excitation.Len() != present.Len() || excitation.Len() != 0 && excitation.Len() != 2 {
		return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: buy/sell excitation and presence must have equal width two", nil))
	}
	for index := range excitation.Len() {
		if !present.At(index) {
			continue
		}
		if excitation.At(index) < 0 {
			return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: excitation must not be negative", nil))
		}
		changed = changed || !input.known[index] || input.excitation[index] != excitation.At(index)
		input.excitation[index], input.known[index] = excitation.At(index), true
	}
	if !args.HasMarket() {
		return changed, nil
	}
	market, err := args.Market()

	if err != nil {
		return false, errnie.Error(err)
	}
	if !market.Updated() {
		return changed, nil
	}
	symbol, err := market.Symbol()

	if err != nil {
		return false, errnie.Error(err)
	}
	if symbol != input.symbol {
		return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: queue symbol differs from observation", nil))
	}
	orders, err := market.Orders()

	if err != nil {
		return false, errnie.Error(err)
	}
	input.orders = make([]manifoldOrder, orders.Len())
	seen := make(map[string]bool, orders.Len())
	var ranks [2]uint32
	for index := range orders.Len() {
		entry := orders.At(index)
		side := 0

		if !entry.Bid() {
			side = 1
		}
		if entry.Rank() != ranks[side] {
			return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: order queue rank is discontinuous", nil))
		}
		ranks[side]++
		identity, err := entry.Id()

		if err != nil {
			return false, errnie.Error(err)
		}
		if identity == "" || seen[identity] {
			return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: resting order identity missing or duplicated", nil))
		}
		seen[identity] = true
		priceText, err := entry.Price()

		if err != nil {
			return false, errnie.Error(err)
		}
		price, err := core.ReadDecimal([]byte(priceText), "manifold order price")

		if err != nil {
			return false, errnie.Error(err)
		}
		quantityText, err := entry.Quantity()

		if err != nil {
			return false, errnie.Error(err)
		}
		quantity, err := core.ReadDecimal([]byte(quantityText), "manifold order quantity")

		if err != nil {
			return false, errnie.Error(err)
		}
		if price.Sign() <= 0 || quantity.Sign() <= 0 {
			return false, errnie.Error(errnie.Err(errnie.Validation, "manifold: resting price and size must be positive", nil))
		}
		input.orders[index] = manifoldOrder{identity, entry.Bid(), price.Float64(), quantity.Float64(), entry.Rank()}
	}
	input.book = true
	return true, nil
}

/*
	project seeds order geometry from observed log-price/log-size extents and

queue mid-ranks. Cell-centre margins prevent periodic-domain boundary aliasing.
Mass is size relative to the current queue mean; opposite-side Hawkes forcing
adds oscillator energy. Equal wave/thermal allocation is the original initial
equipartition condition, not a fitted market threshold.
*/
func (market *manifoldMarket) project(input manifoldInput, grid [3]uint32, nextID *int64) (*sensorium.State, []int64) {
	count := len(input.orders)
	state := &sensorium.State{N: count, Bytes: make([]int64, count), Seqs: make([]int64, count), TokenIDs: make([]int64, count), ContentIDs: make([]int64, count),
		Phase: make([]float32, count), Omega: make([]float32, count), Energy: make([]float32, count), Mass: make([]float32, count), Heat: make([]float32, count), Amp: make([]float32, count),
		Pos: make([]float32, 3*count), Vel: make([]float32, 3*count), PilotVel: make([]float32, 3*count), PhasePotential: make([]float32, count), Clamped: make([]bool, count), Dark: make([]bool, count)}
	var sideCount [2]uint32
	quantityMean := 0.0
	for index, order := range input.orders {
		market.observe(math.Log(order.price), math.Log(order.quantity))
		quantityMean += (order.quantity - quantityMean) / float64(index+1)
		side := 0

		if !order.bid {
			side = 1
		}
		sideCount[side]++
	}
	current := make(map[string]int64, count)
	for index, order := range input.orders {
		identity, found := market.identities[order.id]

		if !found {
			*nextID++
			identity = *nextID
		}
		current[order.id], state.ContentIDs[index], state.Seqs[index] = identity, identity, input.sequence
		side := 0

		if !order.bid {
			side = 1
		}
		price := market.coordinate(math.Log(order.price), market.priceLow, market.priceHigh, grid[0])
		size := market.coordinate(math.Log(order.quantity), market.sizeLow, market.sizeHigh, grid[1])
		priority := (float64(order.rank) + 0.5) / float64(sideCount[side])
		mass := order.quantity / quantityMean
		energy := mass * (1 + input.excitation[1-side])
		maximumAxis := float64(max(grid[0], grid[1], grid[2]))
		state.Pos[3*index], state.Pos[3*index+1], state.Pos[3*index+2] = float32(price*float64(grid[0])/maximumAxis), float32(size*float64(grid[1])/maximumAxis), float32(priority*float64(grid[2])/maximumAxis)
		state.Mass[index], state.Energy[index], state.Heat[index], state.Amp[index] = float32(mass), float32(energy), float32(energy/2), float32(math.Sqrt(energy))
		// Phase uses disjoint half-circles for queue side; frequency is a
		// dimensionless angular coordinate across the observed price extent.
		state.Phase[index] = float32(math.Pi * (float64(side) + priority))
		state.Omega[index] = float32(2 * math.Pi * (price - 0.5))
	}
	departed := make([]int64, 0)
	for identity, contentID := range market.identities {
		if _, found := current[identity]; !found {
			departed = append(departed, contentID)
		}
	}
	market.identities = current
	return state, departed
}

func (market *manifoldMarket) observe(price, size float64) {
	if !market.initialized {
		market.priceLow, market.priceHigh, market.sizeLow, market.sizeHigh = price, price, size, size
		market.initialized = true
		return
	}
	market.priceLow, market.priceHigh = math.Min(market.priceLow, price), math.Max(market.priceHigh, price)
	market.sizeLow, market.sizeHigh = math.Min(market.sizeLow, size), math.Max(market.sizeHigh, size)
}

func (market *manifoldMarket) coordinate(value, low, high float64, cells uint32) float64 {
	if high == low {
		return 0.5
	}
	margin := 0.5 / float64(cells)
	return margin + (1-2*margin)*(value-low)/(high-low)
}
