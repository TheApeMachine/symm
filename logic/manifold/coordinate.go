package manifold

import (
	"hash/fnv"
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
	orderID  string
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

		if order.side == book.Ask {
			signedLogPrice = -signedLogPrice
		}

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

		if order.side == book.Ask {
			signedLogPrice = -signedLogPrice
		}

		logSize := math.Log(1 + order.quantity)
		age := at.Sub(order.timestamp).Seconds()
		posX := signedLogPrice/priceScale + config.DomainX/2
		posY := (logSize-sizeMean)/sizeScale + config.DomainY/2
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

		contentByte := contentByte(order.orderID)
		omega := omegaMin + float64(contentByte)/255*(omegaMax-omegaMin)
		phase := goldenPhase(uint64(index) + 1)
		mass := order.quantity / totalMass

		mapped = append(mapped, mappedOrder{
			orderID:  order.orderID,
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

func survivalCoordinate(age float64, sortedAges []float64) float64 {
	if len(sortedAges) == 0 {
		return 0
	}

	rank := sort.SearchFloat64s(sortedAges, age) + 1

	return float64(rank) / float64(len(sortedAges))
}

func goldenPhase(sequence uint64) float64 {
	fraction := float64(sequence)*goldenRatio - math.Floor(float64(sequence)*goldenRatio)

	return 2 * math.Pi * fraction
}

func contentByte(orderID string) uint8 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(orderID))

	return uint8(hasher.Sum32())
}
