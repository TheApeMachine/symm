package broker

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
BookLevel is one price/qty level from a tree book artifact payload.
*/
type BookLevel struct {
	Price float64
	Qty   float64
}

/*
BookQuote holds bid and ask depth from the latest book artifact.
*/
type BookQuote struct {
	Bids []BookLevel
	Asks []BookLevel
}

/*
Quote is the merged ticker and book view for one symbol.
*/
type Quote struct {
	Symbol    string
	Last      float64
	Bid       float64
	Ask       float64
	UpdatedAt time.Time
	Book      BookQuote
}

func (quote Quote) Mark() (float64, bool) {
	if quote.Last > 0 {
		return quote.Last, true
	}

	if quote.Bid > 0 && quote.Ask > 0 {
		return (quote.Bid + quote.Ask) / 2, true
	}

	return 0, false
}

/*
QuoteCache seeks ticker and book rows already on the shared tree.
*/
type QuoteCache struct {
	tree *dmt.Tree
}

func NewQuoteCache(tree *dmt.Tree) *QuoteCache {
	return &QuoteCache{tree: tree}
}

func (cache *QuoteCache) QuoteForSymbol(symbol string) (Quote, bool) {
	if cache == nil || cache.tree == nil || symbol == "" {
		return Quote{}, false
	}

	quote := Quote{Symbol: symbol}

	for artifact := range cache.tree.Seek([]byte("ticker/" + symbol)) {
		cache.mergeTicker(artifact, &quote)
	}

	for artifact := range cache.tree.Seek([]byte("book/" + symbol)) {
		cache.mergeBook(artifact, &quote)
	}

	if quote.Bid <= 0 && quote.Ask <= 0 && quote.Last <= 0 {
		return Quote{}, false
	}

	return quote, true
}

func (cache *QuoteCache) mergeTicker(artifact *datura.Artifact, quote *Quote) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var frame struct {
		Data []struct {
			Last float64 `json:"last"`
			Bid  float64 `json:"bid"`
			Ask  float64 `json:"ask"`
		} `json:"data"`
	}

	if json.Unmarshal(payload, &frame) != nil || len(frame.Data) == 0 {
		return
	}

	row := frame.Data[0]
	quote.Last = row.Last
	quote.Bid = row.Bid
	quote.Ask = row.Ask
	quote.UpdatedAt = quoteTimeFromArtifact(artifact, payload)
}

func (cache *QuoteCache) mergeBook(artifact *datura.Artifact, quote *Quote) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var frame struct {
		Data []struct {
			Bids [][]any `json:"bids"`
			Asks [][]any `json:"asks"`
		} `json:"data"`
	}

	if json.Unmarshal(payload, &frame) != nil || len(frame.Data) == 0 {
		return
	}

	row := frame.Data[0]
	quote.Book.Bids = parseBookSide(row.Bids)
	quote.Book.Asks = parseBookSide(row.Asks)

	if quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = quoteTimeFromArtifact(artifact, payload)
	}
}

func parseBookSide(levels [][]any) []BookLevel {
	parsed := make([]BookLevel, 0, len(levels))

	for _, level := range levels {
		if len(level) < 2 {
			continue
		}

		price, priceOK := level[0].(float64)
		qty, qtyOK := level[1].(float64)

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			continue
		}

		parsed = append(parsed, BookLevel{Price: price, Qty: qty})
	}

	return parsed
}

func quoteTimeFromArtifact(artifact *datura.Artifact, payload []byte) time.Time {
	var frame struct {
		Timestamp string `json:"timestamp"`
	}

	if json.Unmarshal(payload, &frame) == nil && frame.Timestamp != "" {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, frame.Timestamp); parseErr == nil {
			return parsed
		}

		if parsed, parseErr := time.Parse(time.RFC3339, frame.Timestamp); parseErr == nil {
			return parsed
		}
	}

	if artifact == nil {
		return time.Time{}
	}

	stamp := artifact.Timestamp()

	if stamp <= 0 {
		return time.Time{}
	}

	return time.Unix(0, stamp)
}
