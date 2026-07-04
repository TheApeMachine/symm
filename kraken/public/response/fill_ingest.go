package response

import (
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/datura"
)

func (fillSimulator *FillSimulator) quoteForSymbol(symbol string) (*datura.Artifact, bool) {
	if fillSimulator == nil || fillSimulator.tree == nil || symbol == "" {
		return nil, false
	}

	ticker, tickerOK := fillSimulator.latestIngest("ticker", symbol)

	if !tickerOK {
		return nil, false
	}

	defer ticker.Release()

	rowIndex := fillSimulator.tickerRowIndex(ticker, symbol)

	if rowIndex < 0 {
		return nil, false
	}

	last := fillSimulator.payloadNumber(ticker, "data", rowIndex, "last")
	bid := fillSimulator.payloadNumber(ticker, "data", rowIndex, "bid")
	ask := fillSimulator.payloadNumber(ticker, "data", rowIndex, "ask")

	if last <= 0 && (bid <= 0 || ask <= 0) {
		return nil, false
	}

	quote := datura.Acquire("paper", datura.Artifact_Type_json)
	quote.WithRole("quote")
	quote.WithScope(symbol)

	body := datura.Map[any]{
		"last": last,
		"bid":  bid,
		"ask":  ask,
	}
	if ticker.Timestamp() > 0 {
		body["updated_at"] = time.Unix(0, ticker.Timestamp()).UTC().Format(time.RFC3339Nano)
	}

	quote.WithPayload(body.Marshal())

	book, bookOK := fillSimulator.latestIngest("book", symbol)

	if bookOK {
		fillSimulator.attachBookDepth(quote, book)
		book.Release()
	}

	return quote, true
}

func (fillSimulator *FillSimulator) payloadNumber(
	artifact *datura.Artifact,
	path ...any,
) float64 {
	if artifact == nil {
		return 0
	}

	if value := datura.Peek[float64](artifact, path...); value != 0 {
		return value
	}

	if value := datura.Peek[int64](artifact, path...); value != 0 {
		return float64(value)
	}

	raw := datura.Peek[string](artifact, path...)

	if raw == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return 0
	}

	return parsed
}

func (fillSimulator *FillSimulator) latestIngest(role, scope string) (*datura.Artifact, bool) {
	var latest *datura.Artifact

	// Frames are keyed role-first with the timestamp ahead of the scope, so we
	// seek the role and keep the last artifact whose payload matches the symbol:
	// iteration is in key order, so the final match is the freshest.
	for candidate := range fillSimulator.tree.Seek([]byte(role + "/")) {
		if !fillSimulator.ingestMatches(candidate, scope) {
			candidate.Release()

			continue
		}

		if latest != nil {
			latest.Release()
		}

		latest = candidate
	}

	if latest == nil {
		return nil, false
	}

	return latest, true
}

func (fillSimulator *FillSimulator) attachBookDepth(
	quote *datura.Artifact,
	book *datura.Artifact,
) {
	if quote == nil || book == nil {
		return
	}

	fillSimulator.copyDepthSide(quote, book, "bids")
	fillSimulator.copyDepthSide(quote, book, "asks")
}

func (fillSimulator *FillSimulator) copyDepthSide(
	quote *datura.Artifact,
	book *datura.Artifact,
	side string,
) {
	for index := 0; index < 256; index++ {
		price := fillSimulator.payloadNumber(book, "data", 0, side, index, "price")
		qty := fillSimulator.payloadNumber(book, "data", 0, side, index, "qty")

		if price <= 0 {
			price = fillSimulator.payloadNumber(book, "data", 0, side, index, 0)
			qty = fillSimulator.payloadNumber(book, "data", 0, side, index, 1)
		}

		if price <= 0 {
			break
		}

		if qty <= 0 {
			continue
		}

		quote.Poke(price, side, index, 0)
		quote.Poke(qty, side, index, 1)
	}
}

/*
ingestMatches reports whether an ingest artifact contains the requested symbol.
*/
func (fillSimulator *FillSimulator) ingestMatches(
	artifact *datura.Artifact,
	symbol string,
) bool {
	if artifact == nil || symbol == "" {
		return false
	}

	scope, _ := artifact.Scope()
	if strings.EqualFold(scope, symbol) {
		return true
	}

	return fillSimulator.tickerRowIndex(artifact, symbol) >= 0 ||
		strings.EqualFold(datura.Peek[string](artifact, "symbol"), symbol) ||
		strings.EqualFold(datura.Peek[string](artifact, "wsname"), symbol)
}

/*
tickerRowIndex returns the payload row carrying symbol in a live ticker frame.
*/
func (fillSimulator *FillSimulator) tickerRowIndex(
	artifact *datura.Artifact,
	symbol string,
) int {
	if artifact == nil || symbol == "" {
		return -1
	}

	for rowIndex := 0; ; rowIndex++ {
		current := datura.Peek[string](artifact, "data", rowIndex, "symbol")
		if current == "" {
			break
		}

		if strings.EqualFold(current, symbol) {
			return rowIndex
		}
	}

	return -1
}
