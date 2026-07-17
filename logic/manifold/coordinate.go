package manifold

import (
	"math"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"gonum.org/v1/gonum/stat"
)

var goldenRatio = (1 + math.Sqrt(5)) / 2

/*
mappedOrder carries one L3 order in dimensionless manifold coordinates.
*/
type mappedOrder struct {
	mass     float64
	posX     float64
	posY     float64
	posZ     float64
	velX     float64
	velY     float64
	velZ     float64
	omega    float64
	phase    float64
	heat     float64
	sequence uint64
}

/*
coordinateEpoch tracks prior order coordinates so event-time velocity is causal.
*/
type coordinateEpoch struct {
	at           time.Time
	midPrice     float64
	spread       float64
	buyCapacity  float64
	sellCapacity float64
	positions    map[string][3]float64
}

/*
mapOrders transforms resting L3 orders into dimensionless coordinates and spectral
modes using market-derived scales from the active population.
*/
func mapOrders(
	config pmanifold.Config,
	orders []physicalOrder,
	midPrice float64,
	at time.Time,
	prior *coordinateEpoch,
) ([]mappedOrder, *coordinateEpoch, bool) {
	if midPrice <= 0 || len(orders) == 0 || at.IsZero() {
		return nil, prior, false
	}

	logPrices := make([]float64, 0, len(orders))
	logSizes := make([]float64, 0, len(orders))
	ages := make([]float64, 0, len(orders))
	totalMass := 0.0
	bestBid := 0.0
	bestAsk := 0.0
	bestBidQuantity := 0.0
	bestAskQuantity := 0.0

	for _, order := range orders {
		if order.quantity <= 0 || order.price <= 0 {
			continue
		}

		signedLogPrice := math.Log(order.price / midPrice)

		logPrices = append(logPrices, signedLogPrice)
		logSizes = append(logSizes, math.Log(1+order.quantity))
		ages = append(ages, at.Sub(order.timestamp).Seconds())
		totalMass += order.quantity

		if order.side == book.Bid && order.price >= bestBid {
			if order.price > bestBid {
				bestBid = order.price
				bestBidQuantity = 0
			}

			bestBidQuantity += order.quantity
		}

		if order.side == book.Ask && (bestAsk == 0 || order.price <= bestAsk) {
			if bestAsk == 0 || order.price < bestAsk {
				bestAsk = order.price
				bestAskQuantity = 0
			}

			bestAskQuantity += order.quantity
		}
	}

	if totalMass <= 0 || len(logPrices) == 0 || bestBid <= 0 ||
		bestAsk <= bestBid || bestBidQuantity <= 0 || bestAskQuantity <= 0 {
		return nil, prior, false
	}

	priceScale := stat.StdDev(logPrices, nil)

	if priceScale <= 0 {
		priceScale = (math.Abs(stat.Quantile(0, stat.Empirical, logPrices, nil)) +
			math.Abs(stat.Quantile(1, stat.Empirical, logPrices, nil))) / 2
	}

	if priceScale <= 0 {
		return nil, prior, false
	}

	sizeMean := stat.Mean(logSizes, nil)
	sizeScale := stat.StdDev(logSizes, nil)

	if sizeScale <= 0 {
		sizeScale = sizeMean
	}

	if sizeScale <= 0 {
		return nil, prior, false
	}

	sortedAges := append([]float64(nil), ages...)
	sort.Float64s(sortedAges)

	omegaMin := config.GateWidthMin()
	omegaMax := config.GateWidthMax()

	if omegaMax <= omegaMin {
		return nil, prior, false
	}

	nextEpoch := &coordinateEpoch{
		at:           at,
		midPrice:     midPrice,
		spread:       (bestAsk - bestBid) / midPrice,
		buyCapacity:  bestAsk * bestAskQuantity,
		sellCapacity: bestBid * bestBidQuantity,
		positions:    make(map[string][3]float64, len(orders)),
	}

	deltaT := 0.0

	if prior != nil && !prior.at.IsZero() && at.After(prior.at) {
		deltaT = at.Sub(prior.at).Seconds()
	}

	mapped := make([]mappedOrder, 0, len(orders))

	for index, order := range orders {
		if order.quantity <= 0 || order.price <= 0 {
			continue
		}

		signedLogPrice := math.Log(order.price / midPrice)

		logSize := math.Log(1 + order.quantity)
		age := at.Sub(order.timestamp).Seconds()
		posX := domainCoordinate(signedLogPrice/priceScale, config.DomainX)
		posY := domainCoordinate((logSize-sizeMean)/sizeScale, config.DomainY)
		posZ := survivalCoordinate(age, sortedAges) * config.DomainZ

		velX, velY, velZ := 0.0, 0.0, 0.0

		if prior != nil && deltaT > 0 {
			if previous, ok := prior.positions[order.orderID]; ok {
				velX = (posX - previous[0]) / deltaT
				velY = (posY - previous[1]) / deltaT
				velZ = (posZ - previous[2]) / deltaT
			}
		}

		nextEpoch.positions[order.orderID] = [3]float64{posX, posY, posZ}

		// Near-touch size updates and cancels faster than deep book; map that
		// urgency onto the configured gate-width band without consulting IDs.
		touchProximity := 1 - math.Abs(math.Tanh(signedLogPrice/priceScale))
		omega := omegaMin + touchProximity*(omegaMax-omegaMin)
		// Bid/ask opposition plus sequence diversity so co-located cohorts
		// interfere instead of sharing one global phase.
		phase := goldenPhase(uint64(index) + 1)

		if order.side == book.Ask {
			phase += math.Pi
		}

		mass := order.quantity / totalMass

		mapped = append(mapped, mappedOrder{
			mass:     mass,
			posX:     posX,
			posY:     posY,
			posZ:     posZ,
			velX:     velX,
			velY:     velY,
			velZ:     velZ,
			omega:    omega,
			phase:    phase,
			heat:     mass,
			sequence: uint64(index) + 1,
		})
	}

	if len(mapped) == 0 {
		return nil, prior, false
	}

	return mapped, nextEpoch, true
}

/*
domainCoordinate maps an unbounded z-score onto (0, domain) centered at
domain/2 via tanh, so the ±sigma bulk of orders spreads across the whole grid
axis instead of collapsing into a stripe around the domain center. A z-score of
0 (an order at the mid) lands exactly at domain/2, preserving the buy/sell split
inject.go derives from PosX >= DomainX/2.
*/
func domainCoordinate(zscore float64, domain float64) float64 {
	return (math.Tanh(zscore) + 1) / 2 * domain
}

func survivalCoordinate(age float64, sortedAges []float64) float64 {
	if len(sortedAges) == 0 {
		return 0
	}

	rank := sort.Search(len(sortedAges), func(index int) bool {
		return sortedAges[index] > age
	})

	return float64(rank) / float64(len(sortedAges))
}

func goldenPhase(sequence uint64) float64 {
	fraction := float64(sequence)*goldenRatio - math.Floor(float64(sequence)*goldenRatio)

	return 2 * math.Pi * fraction
}
