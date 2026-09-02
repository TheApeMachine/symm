package websocket

import (
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
IngestEnvelopes builds the Workspace envelopes a single parsed raw frame
produces, together with the EnvelopeManifest for each. It is the one place the
raw-frame → envelope fan-out happens, so a frame yielding N semantic records
yields N envelopes carrying the same CaptureIdentity and deterministic ordinals
(§12), plus N manifests keyed by the same EnvelopeRef — the exact join the
historical record relies on.

kind is the channel/feed name (ticker/trade/level3); parsed is the *kraken
entity the entityMap produced for that channel; captureID is the identity the
capture sink minted for the raw frame this parse came from.
*/
func IngestEnvelopes(
	kind string,
	parsed any,
	captureID hindsight.CaptureIdentity,
) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	switch kind {
	case "ticker":
		return fromTicker(parsed, captureID)
	case "trade":
		return fromTrade(parsed, captureID)
	case "level3":
		return fromLevel3(parsed, captureID)
	case "executions":
		return fromExecution(parsed, captureID)
	case "futures.ticker":
		return fromFuturesTicker(parsed, captureID)
	case "futures.trade":
		return fromFuturesTrade(parsed, captureID)
	default:
		return nil, nil
	}
}

func fromTicker(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	ticker, valid := parsed.(*kraken.Ticker)

	if !valid || ticker == nil {
		return nil, nil
	}

	envelopes := make([]*types.Envelope, 0, len(ticker.Data))
	manifests := make([]hindsight.EnvelopeManifest, 0, len(ticker.Data))

	for ordinal, data := range ticker.Data {
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData = data
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(ordinal)

		envelopes = append(envelopes, envelope)
		manifests = append(manifests, manifestFor(envelope, captureID, uint64(ordinal), "ticker", data.Symbol))
	}

	return envelopes, manifests
}

func fromTrade(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	trade, valid := parsed.(*kraken.Trade)

	if !valid || trade == nil {
		return nil, nil
	}

	envelopes := make([]*types.Envelope, 0, len(trade.Data))
	manifests := make([]hindsight.EnvelopeManifest, 0, len(trade.Data))

	for ordinal, data := range trade.Data {
		envelope := types.NewEnvelope(types.EnvelopeTrade)
		envelope.TradeData = data
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(ordinal)

		envelopes = append(envelopes, envelope)
		manifests = append(manifests, manifestFor(envelope, captureID, uint64(ordinal), "trade", data.Symbol))
	}

	return envelopes, manifests
}

func fromLevel3(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	level3, valid := parsed.(*kraken.Level3)

	if !valid || level3 == nil {
		return nil, nil
	}

	envelopes := make([]*types.Envelope, 0, len(level3.Data))
	manifests := make([]hindsight.EnvelopeManifest, 0, len(level3.Data))

	for ordinal, data := range level3.Data {
		envelope := types.NewEnvelope(types.EnvelopeLevel3)
		envelope.Level3Data = data
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(ordinal)

		envelopes = append(envelopes, envelope)
		manifests = append(manifests, manifestFor(envelope, captureID, uint64(ordinal), "level3", data.Symbol))
	}

	return envelopes, manifests
}

func fromExecution(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	execution, valid := parsed.(*kraken.Execution)

	if !valid || execution == nil {
		return nil, nil
	}

	envelopes := make([]*types.Envelope, 0, len(execution.Data))
	manifests := make([]hindsight.EnvelopeManifest, 0, len(execution.Data))

	for ordinal, data := range execution.Data {
		envelope := types.NewEnvelope(types.EnvelopeExecution)
		envelope.ExecutionData = data
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(ordinal)

		envelopes = append(envelopes, envelope)
		manifests = append(manifests, manifestFor(envelope, captureID, uint64(ordinal), "executions", data.Symbol))
	}

	return envelopes, manifests
}

func fromFuturesTicker(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	ticker, valid := parsed.(*kraken.FuturesTicker)

	if !valid || ticker == nil {
		return nil, nil
	}

	// The futures ticker feed carries one record per frame, where the spot feed
	// carries a slice, so this fan-out is always a single envelope.
	envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
	envelope.FuturesTickerData = ticker.Data
	envelope.CaptureID = captureID

	return []*types.Envelope{envelope}, []hindsight.EnvelopeManifest{
		manifestFor(envelope, captureID, 0, "futures.ticker", ticker.Data.Symbol),
	}
}

func fromFuturesTrade(parsed any, captureID hindsight.CaptureIdentity) ([]*types.Envelope, []hindsight.EnvelopeManifest) {
	trade, valid := parsed.(*kraken.FuturesTrade)

	if !valid || trade == nil {
		return nil, nil
	}

	envelopes := make([]*types.Envelope, 0, len(trade.Data))
	manifests := make([]hindsight.EnvelopeManifest, 0, len(trade.Data))

	for ordinal := range trade.Data {
		dataIndex := ordinal

		// A futures trade snapshot is a bounded reverse-causal history: newest
		// first. Expand it oldest first so the streaming workload observes the
		// venue sequence causally; live single-trade frames retain wire order.
		if trade.Feed == "trade_snapshot" {
			dataIndex = len(trade.Data) - ordinal - 1
		}

		data := trade.Data[dataIndex]
		envelope := types.NewEnvelope(types.EnvelopeFuturesTrade)
		envelope.FuturesTradeData = data
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(ordinal)

		envelopes = append(envelopes, envelope)
		manifests = append(manifests, manifestFor(envelope, captureID, uint64(ordinal), "futures.trade", data.Symbol))
	}

	return envelopes, manifests
}

/*
manifestFor records how one envelope entered Workspace: its EnvelopeRef (the
capture identity plus its deterministic ordinal) and the domain facts that make
the raw frame → semantic ingress relationship immutable (§13).
*/
func manifestFor(
	envelope *types.Envelope,
	captureID hindsight.CaptureIdentity,
	ordinal uint64,
	workload, symbol string,
) hindsight.EnvelopeManifest {
	// Derived envelopes and parser-synthetic futures clocks do not carry a
	// protocol-supplied venue time, so their manifest field remains zero.
	venueAt := time.Time{}

	switch envelope.TypeID {
	case types.EnvelopeTicker:
		venueAt = envelope.TickerData.Timestamp
	case types.EnvelopeTrade:
		venueAt = envelope.TradeData.Timestamp
	case types.EnvelopeLevel3:
		venueAt = envelope.Level3Data.Timestamp
	case types.EnvelopeExecution:
		venueAt = envelope.ExecutionData.Timestamp
	case types.EnvelopeFuturesTicker:
		if !envelope.FuturesTickerData.SyntheticTimestamp {
			venueAt = envelope.FuturesTickerData.Timestamp
		}
	case types.EnvelopeFuturesTrade:
		if !envelope.FuturesTradeData.SyntheticTimestamp {
			venueAt = envelope.FuturesTradeData.Timestamp
		}
	}

	return hindsight.EnvelopeManifest{
		Envelope: hindsight.EnvelopeRef{
			Origin:  captureID,
			Ordinal: ordinal,
		},
		Workload:   workload,
		DomainKind: workload,
		Symbol:     symbol,
		VenueAt:    venueAt,
	}
}
