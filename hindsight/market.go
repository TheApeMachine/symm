package hindsight

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
)

/*
ReadObservations decodes market facts from the original captured frames in
capture order after the supplied sequence. The returned cursor includes frames
with no market projection and advances only after every selected frame decodes.
It never reads the agent's decisions or reconstructed state.

Capture order comes from the sequence column, not from storage layout: Iceberg
guarantees no row order, so the reader sorts rather than trusting the order
files happen to arrive in.
*/
func ReadObservations(ctx context.Context, catalog *tables.Catalog, run RunID, after int64) ([]Observation, int64, error) {
	rows, err := catalog.Captures(ctx, string(run), after)

	if err != nil {
		return nil, after, err
	}

	observations := []Observation{}

	for _, row := range rows {
		decoded, err := FrameFromRow(row).Observations()

		if err != nil {
			return nil, after, err
		}

		observations = append(observations, decoded...)
	}

	if len(rows) > 0 {
		after = rows[len(rows)-1].Sequence
	}

	return observations, after, nil
}

// FrameFromRow rebuilds a RawFrame from its stored row, so the decoders below
// keep working against the record shape they were written for.
func FrameFromRow(row tables.CaptureRow) RawFrame {
	return RawFrame{
		Identity: CaptureIdentity{
			Run:            RunID(row.Run),
			Sequence:       CaptureSequence(row.Sequence),
			Stream:         Stream(row.Stream),
			StreamEpoch:    StreamEpoch(row.StreamEpoch),
			StreamSequence: uint64(row.StreamSequence),
		},
		ReceivedAt:  row.ReceivedAt,
		Endpoint:    row.Endpoint,
		Kind:        row.Kind,
		PayloadHash: row.PayloadHash,
		Payload:     row.Payload,
	}
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
