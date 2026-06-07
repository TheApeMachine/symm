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

	symbol := normalizeWsname(strings.TrimSpace(pair.Wsname))

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

	if strings.TrimSpace(pair.Base) == "" {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s base is required", symbol)
	}

	if strings.TrimSpace(pair.Quote) == "" {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s quote is required", symbol)
	}

	if strings.TrimSpace(pair.Status) == "" {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s status is required", symbol)
	}

	if qtyMin <= 0 {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s ordermin must be positive", symbol)
	}

	if costMin <= 0 {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s costmin must be positive", symbol)
	}

	if priceIncrement <= 0 {
		return InstrumentPair{}, fmt.Errorf("kraken/market: %s tick_size must be positive", symbol)
	}

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

// legacyAssetCodes maps Kraken's REST-era asset codes to the v2 websocket
// names the rest of the system speaks. REST AssetPairs wsnames still say
// XBT/EUR and XDG/EUR while every v2 tape row says BTC/EUR and DOGE/EUR — a
// rules cache seeded from REST alone answered "missing instrument rules for
// BTC/EUR" on the most liquid pair on the exchange (every tune entry blocked;
// live only survived because the websocket instrument snapshot overlaid the
// v2 names).
var legacyAssetCodes = map[string]string{
	"XBT": "BTC",
	"XDG": "DOGE",
}

func normalizeWsname(wsname string) string {
	slash := strings.IndexByte(wsname, '/')

	if slash <= 0 {
		return wsname
	}

	base := wsname[:slash]
	quote := wsname[slash+1:]

	if modern, legacy := legacyAssetCodes[base]; legacy {
		base = modern
	}

	if modern, legacy := legacyAssetCodes[quote]; legacy {
		quote = modern
	}

	return base + "/" + quote
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
