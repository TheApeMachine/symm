package toxicity

import (
	"math"
	"strconv"

	"github.com/theapemachine/symm/kraken/market"
)

const priceKeyScale = 100_000

/*
priceKey discretizes a price into a stable integer for map lookups. When the
pair carries a tick size, prices snap to tick boundaries; otherwise a fixed
scale avoids float64 map-key equality misses.
*/
func priceKey(price float64, pair market.Pair) int64 {
	tickSize, err := strconv.ParseFloat(pair.TickSize, 64)

	if err != nil || tickSize <= 0 {
		return int64(math.Round(price * priceKeyScale))
	}

	rounded := math.Round(price / tickSize)

	if rounded > float64(math.MaxInt64) {
		return math.MaxInt64
	}

	if rounded < float64(math.MinInt64) {
		return math.MinInt64
	}

	return int64(rounded)
}

func priceFromKey(key int64, pair market.Pair) float64 {
	tickSize, err := strconv.ParseFloat(pair.TickSize, 64)

	if err != nil || tickSize <= 0 {
		return float64(key) / priceKeyScale
	}

	return float64(key) * tickSize
}
