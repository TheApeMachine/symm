package broker

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken"
)

/*
captureFeeProfiles records the exact pair rules and active taker and maker fee
rows used by live execution economics.
*/
func (price *Price) captureFeeProfiles(
	symbols []string,
	result *kraken.TradeVolumeResult,
) error {
	if price.capture == nil {
		return nil
	}

	profiles := make([]kraken.MarketProfile, 0, len(symbols))

	for _, symbol := range symbols {
		pair, err := price.normalizer.PairInfo(symbol)

		if err != nil {
			return fmt.Errorf("broker: capture pair metadata for %s: %w", symbol, err)
		}

		taker, err := price.resolveFee(symbol, result.Fees)

		if err != nil {
			return fmt.Errorf("broker: capture taker fee for %s: %w", symbol, err)
		}

		maker, err := price.resolveFee(symbol, result.FeesMaker)

		if err != nil {
			return fmt.Errorf("broker: capture maker fee for %s: %w", symbol, err)
		}

		profiles = append(profiles, kraken.MarketProfile{
			Symbol: symbol,
			Pair:   *pair,
			Taker:  taker,
			Maker:  maker,
		})
	}

	return price.capture.Write(struct {
		Endpoint   string    `json:"endpoint"`
		Payload    any       `json:"payload"`
		ReceivedAt time.Time `json:"received_at"`
	}{
		Endpoint: "symm_metadata",
		Payload: struct {
			Channel string                 `json:"channel"`
			Type    string                 `json:"type"`
			Data    []kraken.MarketProfile `json:"data"`
		}{
			Channel: "symm_metadata",
			Type:    "market_profiles",
			Data:    profiles,
		},
		ReceivedAt: time.Now().UTC(),
	})
}
