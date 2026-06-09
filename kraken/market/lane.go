package market

import (
	"fmt"
	"strings"
)

/*
InstrumentLane is the Y-axis fragmentation axis: spot, perpetual, or dated future
on the same underlying within one venue (Kraken spot + Kraken derivatives).
*/
type InstrumentLane uint32

const (
	InstrumentLaneSpot InstrumentLane = iota
	InstrumentLanePerpetual
	InstrumentLaneDatedFuture
)

/*
InstrumentIdentity binds a feed symbol to its torus coordinates: base asset and lane.
*/
type InstrumentIdentity struct {
	Symbol string
	Base   string
	Lane   InstrumentLane
}

/*
SpotIdentityFromPair derives manifold coordinates from a spot v2 pair symbol.
*/
func SpotIdentityFromPair(symbol string) (InstrumentIdentity, error) {
	base, _, err := splitPairSymbol(symbol)

	if err != nil {
		return InstrumentIdentity{}, err
	}

	return InstrumentIdentity{
		Symbol: symbol,
		Base:   normalizeBaseAsset(base),
		Lane:   InstrumentLaneSpot,
	}, nil
}

/*
FuturesIdentityFromProduct derives manifold coordinates from a Kraken Futures product id.
*/
func FuturesIdentityFromProduct(productID string) (InstrumentIdentity, error) {
	productID = strings.ToUpper(strings.TrimSpace(productID))

	if productID == "" {
		return InstrumentIdentity{}, fmt.Errorf("market: futures product id is empty")
	}

	parts := strings.Split(productID, "_")

	if len(parts) < 2 {
		return InstrumentIdentity{}, fmt.Errorf("market: invalid futures product id %q", productID)
	}

	prefix := parts[0]
	base := baseFromFuturesSymbol(parts[1])

	switch prefix {
	case "PI", "PF":
		return InstrumentIdentity{
			Symbol: productID,
			Base:   base,
			Lane:   InstrumentLanePerpetual,
		}, nil
	case "FI":
		return InstrumentIdentity{
			Symbol: productID,
			Base:   base,
			Lane:   InstrumentLaneDatedFuture,
		}, nil
	default:
		return InstrumentIdentity{}, fmt.Errorf("market: unsupported futures product prefix %q", prefix)
	}
}

/*
PerpetualProductFromSpotPair maps a spot pair to the inverse perpetual product id.
*/
func PerpetualProductFromSpotPair(symbol string) (string, error) {
	base, quote, err := splitPairSymbol(symbol)

	if err != nil {
		return "", err
	}

	if strings.ToUpper(quote) != "USD" {
		return "", fmt.Errorf("market: futures mapping requires USD quote for %q", symbol)
	}

	return fmt.Sprintf("PI_%sUSD", normalizeBaseAsset(base)), nil
}

/*
SplitPairSymbol splits a spot pair symbol into base and quote.
*/
func SplitPairSymbol(symbol string) (string, string, error) {
	return splitPairSymbol(symbol)
}

func splitPairSymbol(symbol string) (string, string, error) {
	parts := strings.Split(symbol, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("market: invalid pair symbol %q", symbol)
	}

	return parts[0], parts[1], nil
}

func normalizeBaseAsset(base string) string {
	base = strings.ToUpper(strings.TrimSpace(base))

	if base == "BTC" {
		return "XBT"
	}

	return base
}

func baseFromFuturesSymbol(symbolPart string) string {
	symbolPart = strings.ToUpper(strings.TrimSpace(symbolPart))

	if strings.HasSuffix(symbolPart, "USD") {
		return normalizeBaseAsset(strings.TrimSuffix(symbolPart, "USD"))
	}

	return normalizeBaseAsset(symbolPart)
}
