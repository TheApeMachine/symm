package resonance

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/statistic"
)

const bookDepthLevels = 5

type marketFacts struct {
	lastPrice       float64
	volume          float64
	spreadBps       float64
	elapsed         float64
	changeAbs       float64
	changePct       float64
	buyPressure     float64
	tradeRate       float64
	tradeNotional   float64
	touchImbalance  float64
	depthImbalance  float64
	spreadWideRatio float64
	tickCadence     float64
	midDriftBps     float64
}

func touchImbalance(element []byte) float64 {
	bidQty, bidOK := peekElementOK[float64](element, "bids.0.qty")
	askQty, askOK := peekElementOK[float64](element, "asks.0.qty")

	if !bidOK || !askOK {
		return 0
	}

	total := bidQty + askQty

	if total <= 0 {
		return 0
	}

	return (bidQty - askQty) / total
}

func depthImbalance(element []byte, levels int) float64 {
	if len(element) == 0 || levels <= 0 {
		return 0
	}

	bidQty := 0.0
	askQty := 0.0
	bidIndex := 0
	askIndex := 0

	eachBookLevelElement(element, "bids", func(price float64, qty float64) {
		if bidIndex >= levels {
			return
		}

		if qty > 0 {
			bidQty += qty
		}

		bidIndex++
	})

	eachBookLevelElement(element, "asks", func(price float64, qty float64) {
		if askIndex >= levels {
			return
		}

		if qty > 0 {
			askQty += qty
		}

		askIndex++
	})

	total := bidQty + askQty

	if total <= 0 {
		return 0
	}

	return (bidQty - askQty) / total
}

func eachBookLevelElement(
	element []byte,
	key string,
	visit func(price float64, qty float64),
) {
	for index := 0; ; index++ {
		price, priceOK := peekElementOK[float64](element, fmt.Sprintf("%s.%d.price", key, index))
		qty, qtyOK := peekElementOK[float64](element, fmt.Sprintf("%s.%d.qty", key, index))

		if !priceOK || !qtyOK {
			break
		}

		visit(price, qty)
	}
}

func spreadWideRatio(currentSpreadBps float64, spreads []float64) float64 {
	if currentSpreadBps <= 0 || len(spreads) == 0 {
		return 0
	}

	reference, referenceOK := statistic.QuantileOf(0.75, spreads)

	if !referenceOK || reference <= 1e-12 {
		return 1
	}

	return currentSpreadBps / reference
}

func midDriftBps(lastPrice float64, element []byte) float64 {
	bidPrice, bidOK := peekElementOK[float64](element, "bids.0.price")
	askPrice, askOK := peekElementOK[float64](element, "asks.0.price")

	if !bidOK || !askOK || lastPrice <= 0 {
		return 0
	}

	mid := (bidPrice + askPrice) / 2
	spread := askPrice - bidPrice

	if spread <= 0 {
		return 0
	}

	return ((lastPrice - mid) / spread) * 10000
}

type tradeFlowStats struct {
	buyPressure  float64
	tradeRate    float64
	notionalRate float64
}

func tradeFlow(symbol string, trade *marketTrade) tradeFlowStats {
	stats := tradeFlowStats{}

	if !trade.Scan(symbol, func(element []byte) {
		price, priceOK := peekElementOK[float64](element, "price")
		qty, qtyOK := peekElementOK[float64](element, "qty")

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return
		}

		stats.tradeRate++
		notional := price * qty
		stats.notionalRate += notional

		side, _ := peekElementOK[string](element, "side")

		if side == "buy" {
			stats.buyPressure += notional
		}

		if side == "sell" {
			stats.buyPressure -= notional
		}
	}) {
		return stats
	}

	window, ok := trade.Window(symbol)

	if !ok || window.Elapsed <= 0 {
		return stats
	}

	stats.tradeRate /= window.Elapsed
	stats.notionalRate /= window.Elapsed

	gross := math.Abs(stats.buyPressure)

	if gross <= 0 {
		stats.buyPressure = 0

		return stats
	}

	stats.buyPressure /= gross

	return stats
}
