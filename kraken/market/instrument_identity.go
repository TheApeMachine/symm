package market

import (
	"fmt"
	"strings"
)

/*
InstrumentLane identifies which Y-axis projection an instrument occupies on the manifold lattice.
*/
type InstrumentLane int

const (
	InstrumentLaneSpot InstrumentLane = iota
	InstrumentLanePerpetual
	InstrumentLaneDatedFuture
)

/*
InstrumentIdentity binds a tradable symbol to its base asset and instrument lane.
*/
type InstrumentIdentity struct {
	Symbol string
	Base   string
	Lane   InstrumentLane
}

/*
SpotIdentityFromPair maps a Kraken spot WebSocket pair to its manifold identity.
*/
func SpotIdentityFromPair(pair string) (InstrumentIdentity, error) {
	parts := strings.Split(pair, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return InstrumentIdentity{}, fmt.Errorf("market: invalid spot pair %q", pair)
	}

	return InstrumentIdentity{
		Symbol: pair,
		Base:   parts[0],
		Lane:   InstrumentLaneSpot,
	}, nil
}

/*
FuturesIdentityFromProduct maps a Kraken futures product id to its manifold identity.
*/
func FuturesIdentityFromProduct(productID string) (InstrumentIdentity, error) {
	switch {
	case strings.HasPrefix(productID, "PI_"):
		base, baseErr := futuresBaseFromProductBody(strings.TrimPrefix(productID, "PI_"))

		if baseErr != nil {
			return InstrumentIdentity{}, baseErr
		}

		return InstrumentIdentity{
			Symbol: productID,
			Base:   base,
			Lane:   InstrumentLanePerpetual,
		}, nil
	case strings.HasPrefix(productID, "FI_"):
		rest := strings.TrimPrefix(productID, "FI_")
		underscore := strings.Index(rest, "_")

		if underscore <= 0 {
			return InstrumentIdentity{}, fmt.Errorf("market: invalid dated futures product %q", productID)
		}

		base, baseErr := futuresBaseFromProductBody(rest[:underscore])

		if baseErr != nil {
			return InstrumentIdentity{}, baseErr
		}

		return InstrumentIdentity{
			Symbol: productID,
			Base:   base,
			Lane:   InstrumentLaneDatedFuture,
		}, nil
	default:
		return InstrumentIdentity{}, fmt.Errorf("market: unknown futures product %q", productID)
	}
}

func futuresBaseFromProductBody(body string) (string, error) {
	for _, quote := range []string{"USDT", "USD", "EUR", "GBP"} {
		if strings.HasSuffix(body, quote) {
			base := strings.TrimSuffix(body, quote)

			if base == "" {
				return "", fmt.Errorf("market: futures product body %q missing base asset", body)
			}

			return base, nil
		}
	}

	return "", fmt.Errorf("market: futures product body %q missing quote suffix", body)
}
