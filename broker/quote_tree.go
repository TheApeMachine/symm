package broker

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/theapemachine/datura"
)

type tickerRow struct {
	last float64
	bid  float64
	ask  float64
	at   time.Time
}

type bookRow struct {
	bids []BookLevel
	asks []BookLevel
	at   time.Time
}

func parseTickerArtifact(artifact *datura.Artifact) (tickerRow, bool) {
	payload, ok := artifact.PayloadQuiet()

	if !ok || len(payload) == 0 {
		return tickerRow{}, false
	}

	row, parsed := parseTickerPayload(payload)

	if !parsed {
		scope, _ := artifact.Scope()

		if scope == "" {
			return tickerRow{}, false
		}

		row = parseTickerElement(payload)
	}

	row.at = quoteTimeFromArtifact(row.at, artifact)

	return row, row.last > 0 || row.bid > 0 || row.ask > 0
}

func parseBookArtifact(artifact *datura.Artifact) (bookRow, bool) {
	payload, ok := artifact.PayloadQuiet()

	if !ok || len(payload) == 0 {
		return bookRow{}, false
	}

	row, parsed := parseBookPayload(payload)

	if !parsed {
		scope, _ := artifact.Scope()

		if scope == "" {
			return bookRow{}, false
		}

		row = parseBookElement(payload)
	}

	row.at = quoteTimeFromArtifact(row.at, artifact)

	return row, len(row.bids) > 0 || len(row.asks) > 0
}

func parseTickerPayload(payload []byte) (tickerRow, bool) {
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}

	if json.Unmarshal(payload, &envelope) != nil || len(envelope.Data) == 0 {
		return tickerRow{}, false
	}

	row := parseTickerElement(envelope.Data[0])

	return row, row.last > 0 || row.bid > 0 || row.ask > 0
}

func parseBookPayload(payload []byte) (bookRow, bool) {
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}

	if json.Unmarshal(payload, &envelope) != nil || len(envelope.Data) == 0 {
		return bookRow{}, false
	}

	row := parseBookElement(envelope.Data[0])

	return row, len(row.bids) > 0 || len(row.asks) > 0
}

func quoteTimeFromArtifact(at time.Time, artifact *datura.Artifact) time.Time {
	if !at.IsZero() {
		return at
	}

	if artifact == nil {
		return time.Time{}
	}

	ts := artifact.Timestamp()

	if ts <= 0 {
		return time.Time{}
	}

	return time.Unix(0, ts).UTC()
}

func parseTickerElement(element []byte) tickerRow {
	row := tickerRow{}

	if last, ok := peekJSONFloat(element, "last"); ok {
		row.last = last
	}

	if bid, ok := peekJSONFloat(element, "bid"); ok {
		row.bid = bid
	}

	if ask, ok := peekJSONFloat(element, "ask"); ok {
		row.ask = ask
	}

	if at, ok := peekJSONTime(element, "timestamp"); ok {
		row.at = at
	}

	return row
}

func parseBookElement(element []byte) bookRow {
	row := bookRow{}

	var wrapper struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}

	if json.Unmarshal(element, &wrapper) != nil {
		return row
	}

	row.bids = parseBookLevels(wrapper.Bids)
	row.asks = parseBookLevels(wrapper.Asks)

	if at, ok := peekJSONTime(element, "timestamp"); ok {
		row.at = at
	}

	return row
}

func parseBookLevels(raw [][]string) []BookLevel {
	levels := make([]BookLevel, 0, len(raw))

	for _, pair := range raw {
		if len(pair) < 2 {
			continue
		}

		price, priceErr := strconv.ParseFloat(pair[0], 64)
		qty, qtyErr := strconv.ParseFloat(pair[1], 64)

		if priceErr != nil || qtyErr != nil || price <= 0 || qty <= 0 {
			continue
		}

		levels = append(levels, BookLevel{Price: price, Qty: qty})
	}

	return levels
}

func peekJSONFloat(payload []byte, field string) (float64, bool) {
	var wrapper map[string]json.RawMessage

	if json.Unmarshal(payload, &wrapper) != nil {
		return 0, false
	}

	raw, ok := wrapper[field]

	if !ok {
		return 0, false
	}

	var number float64

	if json.Unmarshal(raw, &number) == nil {
		return number, number > 0
	}

	var text string

	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}

	parsed, err := strconv.ParseFloat(text, 64)

	if err != nil || parsed <= 0 {
		return 0, false
	}

	return parsed, true
}

func peekJSONTime(payload []byte, field string) (time.Time, bool) {
	var wrapper map[string]json.RawMessage

	if json.Unmarshal(payload, &wrapper) != nil {
		return time.Time{}, false
	}

	raw, ok := wrapper[field]

	if !ok {
		return time.Time{}, false
	}

	var text string

	if json.Unmarshal(raw, &text) != nil || text == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339Nano, text)

	if err != nil {
		parsed, err = time.Parse(time.RFC3339, text)
	}

	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}
