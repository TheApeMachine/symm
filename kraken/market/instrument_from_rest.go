package market

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

/*
InstrumentPairFromREST maps GET /public/AssetPairs metadata to the instrument
rules shape used by broker.InstrumentRulesCache.
*/
func InstrumentPairFromREST(pair *Pair) (InstrumentPair, error) {
	if pair == nil {
		return InstrumentPair{}, fmt.Errorf("kraken/market: asset pair is nil")
	}

	symbol := strings.TrimSpace(pair.Wsname)

	if symbol == "" {
		return InstrumentPair{}, fmt.Errorf("kraken/market: asset pair wsname is required")
	}

	qtyMin, err := parseRestDecimal(pair.Ordermin)

	if err != nil {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s ordermin: %w", symbol, err)
	}

	costMin, err := parseRestDecimal(pair.Costmin)

	if err != nil {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s costmin: %w", symbol, err)
	}

	priceIncrement, err := parseRestDecimal(pair.TickSize)

	if err != nil {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s tick_size: %w", symbol, err)
	}

	qtyIncrement := quantityIncrementFromLotDecimals(pair.LotDecimals)

	return InstrumentPair{
		Symbol:         symbol,
		Base:           pair.Base,
		Quote:          pair.Quote,
		Status:         pair.Status,
		QtyPrecision:   pair.LotDecimals,
		QtyIncrement:   qtyIncrement,
		PricePrecision: pair.PairDecimals,
		CostPrecision:  pair.CostDecimals,
		CostMin:        costMin,
		PriceIncrement: priceIncrement,
		QtyMin:         qtyMin,
	}, nil
}

func parseRestDecimal(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return 0, nil
	}

	value, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return 0, err
	}

	return value, nil
}

func quantityIncrementFromLotDecimals(lotDecimals int) float64 {
	if lotDecimals <= 0 {
		return 1
	}

	return math.Pow(10, -float64(lotDecimals))
}
