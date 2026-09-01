package store

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
)

/*
captureMarketIndex accelerates the per-run market read. It contains only ticker
and trade rows because those are the raw coordinates Episode discovery decodes;
book/L3/protocol rows remain exactly captured without inflating this index.
*/
const captureMarketIndex = `
CREATE INDEX IF NOT EXISTS idx_events_market_run_capture
ON events(run_id, capture_seq)
WHERE kind IN ('ticker', 'trade');
`

/*
ListMarketObservations reads one Run's raw capture tape and returns the
external market observations it carried, in CaptureSequence order.

It reads the RAW frames — never a witness, never a persisted EnvelopeState —
because Episode discovery must be able to see the market without consulting
anything SYMM decided about it (§27). The raw payload is the irreducible replay
substrate (§10), so the coordinate series a selector measures is derived from
the same bytes the live process received.

One raw frame may carry zero, one, or many observations; each observation keeps
the deterministic ordinal it had inside its frame, so it is addressable by the
same EnvelopeRef identity the pipeline assigned (§12).
*/
func (store *SQLite) ListMarketObservations(runID string) ([]hindsight.Observation, error) {
	if store == nil || store.database == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	if runID == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: run identity required",
			nil,
		))
	}

	rows, err := store.database.Query(
		`SELECT capture_seq, stream, stream_epoch, stream_seq, kind, endpoint, at, data, encoding
		 FROM events
		 WHERE run_id = ? AND kind IN ('ticker', 'trade')
		 ORDER BY capture_seq ASC`,
		runID,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: list market observations failed",
			err,
		))
	}

	defer rows.Close()

	observations := make([]hindsight.Observation, 0, 4096)

	for rows.Next() {
		var (
			identity   hindsight.CaptureIdentity
			kind       string
			endpoint   string
			receivedAt string
			payload    []byte
			encoding   string
		)

		identity.Run = hindsight.RunID(runID)

		if err := rows.Scan(
			&identity.Sequence,
			&identity.Stream,
			&identity.StreamEpoch,
			&identity.StreamSequence,
			&kind,
			&endpoint,
			&receivedAt,
			&payload,
			&encoding,
		); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: scan market observation row",
				err,
			))
		}

		received, _ := time.Parse(time.RFC3339Nano, receivedAt)
		payload, err = store.decodePayload(payload, encoding)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"store: decode market observation payload",
				err,
			))
		}

		observations = append(
			observations,
			decodeObservations(identity, kind, endpoint, received, payload)...,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"store: iterate market observation rows",
			err,
		))
	}

	return observations, nil
}

/*
decodeObservations turns one raw frame into the observations it carried. The
endpoint decides the protocol dialect — the spot v2 socket batches records in a
data array, the futures v1 socket sends one flat record per frame — so the same
"ticker" kind is never assumed to have one shape. A frame carrying no market
record (a subscription acknowledgement, a heartbeat) yields none, which is the
zero-envelope case the model explicitly allows (§12, §65.2).
*/
func decodeObservations(
	identity hindsight.CaptureIdentity,
	kind, endpoint string,
	receivedAt time.Time,
	payload []byte,
) []hindsight.Observation {
	if len(payload) == 0 {
		return nil
	}

	futures := strings.Contains(endpoint, "futures")

	switch {
	case kind == "ticker" && !futures:
		return spotTickerObservations(identity, receivedAt, payload)
	case kind == "trade" && !futures:
		return spotTradeObservations(identity, receivedAt, payload)
	case kind == "ticker" && futures:
		return futuresTickerObservations(identity, receivedAt, payload)
	case kind == "trade" && futures:
		return futuresTradeObservations(identity, receivedAt, payload)
	default:
		return nil
	}
}

/*
spotTickerObservations decodes a spot v2 ticker frame with the same kraken
parser the live ingest uses, so the inspection view reads the capture exactly
as the running system read it rather than through a second dialect (§41 keeps
validators independent; parsing the same bytes twice differently would only
manufacture disagreement about what the venue said).
*/
func spotTickerObservations(
	identity hindsight.CaptureIdentity,
	receivedAt time.Time,
	payload []byte,
) []hindsight.Observation {
	if !hasArrayData(payload) {
		return nil
	}

	ticker := kraken.NewTicker(payload)

	if ticker == nil || len(ticker.Data) == 0 {
		return nil
	}

	observations := make([]hindsight.Observation, 0, len(ticker.Data))

	for ordinal, data := range ticker.Data {
		if data.Symbol == "" {
			continue
		}

		observation := hindsight.Observation{
			Capture:    identity,
			Ordinal:    uint64(ordinal),
			Symbol:     data.Symbol,
			Kind:       "ticker",
			ReceivedAt: receivedAt,
			VenueAt:    data.Timestamp,
		}

		if data.Bid != nil {
			observation.HasBid = true
			observation.Bid = data.Bid.Float64()
			observation.BidQty = data.BidQty
		}

		if data.Ask != nil {
			observation.HasAsk = true
			observation.Ask = data.Ask.Float64()
			observation.AskQty = data.AskQty
		}

		if data.Last != nil {
			observation.HasLast = true
			observation.Last = data.Last.Float64()
		}

		observations = append(observations, observation)
	}

	return observations
}

func spotTradeObservations(
	identity hindsight.CaptureIdentity,
	receivedAt time.Time,
	payload []byte,
) []hindsight.Observation {
	if !hasArrayData(payload) {
		return nil
	}

	trade := kraken.NewTrade(payload)

	if trade == nil || len(trade.Data) == 0 {
		return nil
	}

	observations := make([]hindsight.Observation, 0, len(trade.Data))

	for ordinal, data := range trade.Data {
		if data.Symbol == "" {
			continue
		}

		price := data.Price.Float64()

		observations = append(observations, hindsight.Observation{
			Capture:    identity,
			Ordinal:    uint64(ordinal),
			Symbol:     data.Symbol,
			Kind:       "trade",
			ReceivedAt: receivedAt,
			VenueAt:    data.Timestamp,
			HasTrade:   true,
			TradePrice: price,
			TradeQty:   data.Qty,
			TradeSide:  data.Side,
		})
	}

	return observations
}

/*
futuresTickerObservations decodes one Kraken Futures v1 ticker frame. The
futures socket names its instrument product_id and sends one record per frame,
so a decoded record always has ordinal 0.
*/
func futuresTickerObservations(
	identity hindsight.CaptureIdentity,
	receivedAt time.Time,
	payload []byte,
) []hindsight.Observation {
	symbol := productIdentity(payload)

	if symbol == "" {
		return nil
	}

	ticker := kraken.NewFuturesTicker(payload)

	if ticker == nil {
		return nil
	}

	data := ticker.Data

	observation := hindsight.Observation{
		Capture:    identity,
		Ordinal:    0,
		Symbol:     symbol,
		Kind:       "ticker",
		ReceivedAt: receivedAt,
		VenueAt:    data.Timestamp,
	}

	if data.Bid != nil {
		observation.HasBid = true
		observation.Bid = data.Bid.Float64()
		observation.BidQty = data.BidSize
	}

	if data.Ask != nil {
		observation.HasAsk = true
		observation.Ask = data.Ask.Float64()
		observation.AskQty = data.AskSize
	}

	if data.Last != nil {
		observation.HasLast = true
		observation.Last = data.Last.Float64()
	}

	if !observation.HasBid && !observation.HasAsk && !observation.HasLast {
		return nil
	}

	return []hindsight.Observation{observation}
}

func futuresTradeObservations(
	identity hindsight.CaptureIdentity,
	receivedAt time.Time,
	payload []byte,
) []hindsight.Observation {
	symbol := productIdentity(payload)

	if symbol == "" {
		return nil
	}

	var frame struct {
		Price float64 `json:"price"`
		Qty   float64 `json:"qty"`
		Side  string  `json:"side"`
		Time  int64   `json:"time"`
	}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return nil
	}

	if frame.Price <= 0 {
		return nil
	}

	venueAt := time.Time{}

	if frame.Time > 0 {
		venueAt = time.UnixMilli(frame.Time).UTC()
	}

	return []hindsight.Observation{{
		Capture:    identity,
		Ordinal:    0,
		Symbol:     symbol,
		Kind:       "trade",
		ReceivedAt: receivedAt,
		VenueAt:    venueAt,
		HasTrade:   true,
		TradePrice: frame.Price,
		TradeQty:   frame.Qty,
		TradeSide:  frame.Side,
	}}
}

/*
hasArrayData reports whether a spot v2 frame carries a data array at all. A
subscription acknowledgement shares the channel name with the records that
follow it, so the shape — not the kind — decides whether there is anything to
decode.
*/
func hasArrayData(payload []byte) bool {
	var frame struct {
		Data []sonic.NoCopyRawMessage `json:"data"`
	}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return false
	}

	return len(frame.Data) > 0
}

/*
productIdentity returns the futures frame's product_id, which is empty on the
subscription acknowledgements that share the ticker/trade feed name.
*/
func productIdentity(payload []byte) string {
	var frame struct {
		ProductID string `json:"product_id"`
	}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return ""
	}

	return frame.ProductID
}
