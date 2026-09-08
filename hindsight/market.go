package hindsight

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"gocloud.dev/blob"
)

// ReadObservations decodes market facts from the original captured frames in
// capture order. It never reads the agent's decisions or reconstructed state.
func ReadObservations(ctx context.Context, bucket *blob.Bucket, run RunID) ([]Observation, error) {
	observations := []Observation{}
	err := store.Scan(ctx, bucket, run.Prefix("captures"), func(frame RawFrame) (bool, error) {
		decoded, err := frame.Observations()
		if err != nil {
			return false, err
		}
		observations = append(observations, decoded...)
		return true, nil
	})
	return observations, err
}

// Observations uses the protocol's existing data types; the stored RawFrame is
// unchanged and retains the exact received bytes and their capture identity.
func (frame RawFrame) Observations() ([]Observation, error) {
	observations := []Observation{}
	domain := "spot"
	if strings.Contains(frame.Endpoint, "futures") {
		domain = "futures"
	}
	base := Observation{Domain: domain, Capture: frame.Identity, Kind: frame.Kind, ReceivedAt: frame.ReceivedAt}
	if domain == "futures" {
		return frame.futuresObservations(base)
	}
	switch frame.Kind {
	case "ticker":
		var ticker kraken.Ticker
		if err := json.Unmarshal(frame.Payload, &ticker); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "hindsight: decode ticker", err))
		}
		for ordinal, entry := range ticker.Data {
			observation := base
			observation.Ordinal, observation.Symbol, observation.VenueAt = uint64(ordinal), entry.Symbol, entry.Timestamp
			observation.HasBid, observation.HasAsk, observation.HasLast = entry.Bid != nil, entry.Ask != nil, entry.Last != nil
			if entry.Bid != nil {
				observation.Bid = entry.Bid.Float64()
			}
			if entry.Ask != nil {
				observation.Ask = entry.Ask.Float64()
			}
			if entry.Last != nil {
				observation.Last = entry.Last.Float64()
			}
			observation.BidQty, observation.AskQty = entry.BidQty, entry.AskQty
			observations = append(observations, observation)
		}
	case "trade":
		var trades kraken.Trade
		if err := json.Unmarshal(frame.Payload, &trades); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "hindsight: decode trade", err))
		}
		for ordinal, entry := range trades.Data {
			observation := base
			observation.Ordinal, observation.Symbol, observation.VenueAt = uint64(ordinal), entry.Symbol, entry.Timestamp
			observation.HasTrade, observation.TradePrice, observation.TradeQty, observation.TradeSide = true, entry.Price.Float64(), entry.Qty, entry.Side
			observations = append(observations, observation)
		}
	case "l3_touch":
		var touches kraken.Level3TouchFrame
		if err := json.Unmarshal(frame.Payload, &touches); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "hindsight: decode L3 touch", err))
		}
		for ordinal, entry := range touches.Data {
			observation := base
			observation.Ordinal, observation.Symbol, observation.VenueAt = uint64(ordinal), entry.Symbol, entry.Timestamp
			observation.HasBid, observation.HasAsk = entry.Bid != nil, entry.Ask != nil
			if entry.Bid != nil {
				observation.Bid = entry.Bid.Float64()
			}
			if entry.Ask != nil {
				observation.Ask = entry.Ask.Float64()
			}
			if entry.BidQty != nil {
				observation.BidQty = entry.BidQty.Float64()
			}
			if entry.AskQty != nil {
				observation.AskQty = entry.AskQty.Float64()
			}
			observations = append(observations, observation)
		}
	}
	return observations, nil
}

func (frame RawFrame) futuresObservations(base Observation) ([]Observation, error) {
	observations := []Observation{}
	if frame.Kind != "ticker" && frame.Kind != "trade" && frame.Kind != "trade_snapshot" {
		return observations, nil
	}
	if !json.Valid(frame.Payload) {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "hindsight: malformed futures frame", nil))
	}
	if frame.Kind == "ticker" {
		entry := kraken.NewFuturesTicker(frame.Payload).Data
		base.Symbol = entry.Symbol
		if !entry.SyntheticTimestamp {
			base.VenueAt = entry.Timestamp
		}
		base.HasBid, base.HasAsk, base.HasLast = entry.Bid != nil, entry.Ask != nil, entry.Last != nil
		if entry.Bid != nil {
			base.Bid = entry.Bid.Float64()
		}
		if entry.Ask != nil {
			base.Ask = entry.Ask.Float64()
		}
		if entry.Last != nil {
			base.Last = entry.Last.Float64()
		}
		base.BidQty, base.AskQty = entry.BidSize, entry.AskSize
		return append(observations, base), nil
	}
	for ordinal, entry := range kraken.NewFuturesTrade(frame.Payload).Data {
		observation := base
		observation.Ordinal, observation.Symbol = uint64(ordinal), entry.Symbol
		if !entry.SyntheticTimestamp {
			observation.VenueAt = entry.Timestamp
		}
		observation.HasTrade, observation.TradePrice, observation.TradeQty, observation.TradeSide = true, entry.Price.Float64(), entry.Qty, entry.Side
		observations = append(observations, observation)
	}
	return observations, nil
}
