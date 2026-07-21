package manifold

import (
	"hash/fnv"
	"math"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"gonum.org/v1/gonum/stat"
)

/*
coordinateEpoch retains empirical scales and prior positions for velocity.
*/
type coordinateEpoch struct {
	at           time.Time
	midPrice     float64
	reference    *decimal.Decimal
	spread       float64
	buyCapacity  *decimal.Decimal
	sellCapacity *decimal.Decimal
	positions    map[string]pfluid.Vector
}

/*
Map converts L3 price, size, survival, identity, and side into observations.
*/
func (prior *coordinateEpoch) Map(
	config pfluid.Config,
	candidate intensityCandidate,
) ([]pfluid.Particle, *coordinateEpoch, bool) {
	orders := candidate.orders
	midPrice := candidate.midPrice
	at := candidate.outcome.At

	if midPrice <= 0 || len(orders) == 0 || at.IsZero() {
		return nil, prior, false
	}

	logPrices, logSizes, ages, touch, ready := marketScales(orders, midPrice, at)

	if !ready {
		return nil, prior, false
	}

	priceScale := empiricalScale(logPrices)
	sizeMean := stat.Mean(logSizes, nil)
	sizeScale := empiricalScale(logSizes)

	if priceScale <= 0 || sizeScale <= 0 {
		return nil, prior, false
	}

	sort.Float64s(ages)
	reference := touch.reference()
	next := &coordinateEpoch{
		at:        at,
		midPrice:  midPrice,
		reference: reference,
		spread:    touch.spread(reference),
		buyCapacity: touch.notional(
			touch.askPriceMoney,
			touch.askQuantity,
		),
		sellCapacity: touch.notional(
			touch.bidPriceMoney,
			touch.bidQuantity,
		),
		positions: make(map[string]pfluid.Vector, len(orders)),
	}
	particles := make([]pfluid.Particle, 0, len(orders))

	for _, order := range orders {
		heat, thermal := candidate.heat(order.side, touch)

		if !thermal {
			return nil, prior, false
		}

		particle, ok := prior.mapOrder(
			config,
			candidate.symbol,
			order,
			midPrice,
			priceScale,
			sizeMean,
			sizeScale,
			ages,
			at,
			heat,
		)

		if !ok {
			continue
		}

		next.positions[order.orderID] = particle.Position
		particles = append(particles, particle)
	}

	return particles, next, len(particles) > 0
}

/*
heat returns expected aggressive arrivals per resting order over the observed
Hawkes horizon. Buys deposit thermal energy into asks and sells into bids, so
the gas receives the measured event flow without assigning every market a
fixed temperature or allowing book population size to multiply total energy.
*/
func (candidate intensityCandidate) heat(
	side book.BookDirection,
	touch marketTouch,
) (float32, bool) {
	horizon := candidate.outcome.Horizon.Seconds()
	buyIntensity, sellIntensity := intensities(candidate.outcome)
	arrivalRate := sellIntensity
	orderCount := touch.bidOrders

	if side == book.Ask {
		arrivalRate = buyIntensity
		orderCount = touch.askOrders
	}

	if orderCount <= 0 || horizon <= 0 || arrivalRate < 0 {
		return 0, false
	}

	heat := arrivalRate * horizon / float64(orderCount)

	if math.IsNaN(heat) || math.IsInf(heat, 0) {
		return 0, false
	}

	return float32(heat), true
}

/*
marketScales collects the empirical distributions and exact two-sided touch
needed to map a complete book without assuming a universal symbol scale.
*/
func marketScales(
	orders []physicalOrder,
	midPrice float64,
	at time.Time,
) ([]float64, []float64, []float64, marketTouch, bool) {
	logPrices := make([]float64, 0, len(orders))
	logSizes := make([]float64, 0, len(orders))
	ages := make([]float64, 0, len(orders))
	touch := marketTouch{
		bidQuantity: decimal.NewFromInt64(0),
		askQuantity: decimal.NewFromInt64(0),
	}

	for _, order := range orders {
		if order.quantity <= 0 || order.price <= 0 {
			continue
		}

		logPrices = append(logPrices, math.Log(order.price/midPrice))
		logSizes = append(logSizes, math.Log1p(order.quantity))
		ages = append(ages, at.Sub(order.timestamp).Seconds())
		touch.observe(order)
	}

	ready := len(logPrices) > 0 && touch.bidPrice > 0 &&
		touch.askPrice > touch.bidPrice && touch.bidPriceMoney != nil &&
		touch.askPriceMoney != nil && touch.bidQuantity.Sign() > 0 &&
		touch.askQuantity.Sign() > 0
	return logPrices, logSizes, ages, touch, ready
}

/*
mapOrder maps one valid order into coupled geometry, thermodynamics, and wave
coordinates while preserving the original Sensorium unit-energy convention.
*/
func (prior *coordinateEpoch) mapOrder(
	config pfluid.Config,
	symbol string,
	order physicalOrder,
	midPrice float64,
	priceScale float64,
	sizeMean float64,
	sizeScale float64,
	ages []float64,
	at time.Time,
	heat float32,
) (pfluid.Particle, bool) {
	if order.quantity <= 0 || order.price <= 0 {
		return pfluid.Particle{}, false
	}

	priceCoordinate := math.Log(order.price/midPrice) / priceScale
	position := pfluid.Vector{
		X: float32(unitCoordinate(priceCoordinate)),
		Y: float32(unitCoordinate((math.Log1p(order.quantity) - sizeMean) / sizeScale)),
		Z: float32(survivalCoordinate(at.Sub(order.timestamp).Seconds(), ages)),
	}
	velocity := prior.velocity(order.orderID, position, at)
	phase := stablePhase(symbol, order.orderID)

	if order.side == book.Ask {
		phase += math.Pi
	}

	return pfluid.Particle{
		Position: position,
		Velocity: velocity,
		Mass:     1,
		Heat:     heat,
		Energy:   1,
		Phase:    float32(math.Remainder(phase, 2*math.Pi)),
		Omega: float32(config.OmegaMin) + float32(unitCoordinate(priceCoordinate))*
			(config.OmegaMax-config.OmegaMin),
	}, true
}

/*
velocity derives order motion over real event time and takes the shortest
periodic displacement on each normalized axis.
*/
func (prior *coordinateEpoch) velocity(
	orderID string,
	position pfluid.Vector,
	at time.Time,
) pfluid.Vector {
	if prior == nil || !at.After(prior.at) {
		return pfluid.Vector{}
	}

	previous, ok := prior.positions[orderID]

	if !ok {
		return pfluid.Vector{}
	}

	seconds := float32(at.Sub(prior.at).Seconds())
	return pfluid.Vector{
		X: periodicDelta(position.X, previous.X) / seconds,
		Y: periodicDelta(position.Y, previous.Y) / seconds,
		Z: periodicDelta(position.Z, previous.Z) / seconds,
	}
}

/*
empiricalScale uses observed dispersion, or the observed magnitude when the
population is degenerate, and never inserts a synthetic scale.
*/
func empiricalScale(values []float64) float64 {
	scale := stat.StdDev(values, nil)

	if scale > 0 {
		return scale
	}

	return math.Abs(stat.Mean(values, nil))
}

/*
unitCoordinate maps an unbounded empirical coordinate into the open unit torus.
*/
func unitCoordinate(value float64) float64 {
	return (math.Tanh(value) + 1) / 2
}

/*
survivalCoordinate maps order age to its empirical cumulative rank.
*/
func survivalCoordinate(age float64, sortedAges []float64) float64 {
	rank := sort.Search(len(sortedAges), func(index int) bool {
		return sortedAges[index] > age
	})

	return float64(rank) / float64(len(sortedAges))
}

/*
stablePhase derives repeatable phase identity from symbol and order ID so map
iteration and intensity rank cannot rotate an oscillator between epochs.
*/
func stablePhase(symbol, orderID string) float64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(symbol))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(orderID))
	fraction := float64(hasher.Sum64()) / float64(math.MaxUint64)

	return 2 * math.Pi * fraction
}

/*
periodicDelta returns the shortest displacement on a unit-periodic axis.
*/
func periodicDelta(current, previous float32) float32 {
	delta := current - previous

	if delta > 0.5 {
		return delta - 1
	}

	if delta < -0.5 {
		return delta + 1
	}

	return delta
}
