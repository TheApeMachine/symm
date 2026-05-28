package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
)

/*
BookLevel is one order book price level.
*/
type BookLevel struct {
	Price  float64
	Volume float64
}

/*
BookTop holds the best bid and ask from a Kraken v2 book frame.
*/
type BookTop struct {
	Symbol  string
	BestBid BookLevel
	BestAsk BookLevel
}

/*
BookTopDelta holds optional bid and ask updates from one book frame.
*/
type BookTopDelta struct {
	Symbol  string
	BestBid BookLevel
	BestAsk BookLevel
	BidOK   bool
	AskOK   bool
}

/*
ParseBookTopDelta extracts optional top bid and ask updates from a book frame.
*/
func ParseBookTopDelta(payload []byte) (BookTopDelta, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return BookTopDelta{}, err
	}

	if !Channel(channel).IsBook() {
		return BookTopDelta{}, fmt.Errorf("not a book event: channel=%q", channel)
	}

	bestBid, bidOK, err := parseTopLevel(payload, "bids")
	if err != nil {
		return BookTopDelta{}, err
	}

	bestAsk, askOK, err := parseTopLevel(payload, "asks")
	if err != nil {
		return BookTopDelta{}, err
	}

	if !bidOK && !askOK {
		return BookTopDelta{}, fmt.Errorf("book frame has no top-of-book levels")
	}

	symbol, err := jsonparser.GetUnsafeString(payload, "data", "[0]", "symbol")
	if err != nil {
		return BookTopDelta{}, fmt.Errorf("parse book symbol: %w", err)
	}

	return BookTopDelta{
		Symbol:  string(symbol),
		BestBid: bestBid,
		BestAsk: bestAsk,
		BidOK:   bidOK,
		AskOK:   askOK,
	}, nil
}

/*
ParseTopBook extracts the top bid and ask from a complete Kraken v2 book frame.
*/
func ParseTopBook(payload []byte) (BookTop, error) {
	delta, err := ParseBookTopDelta(payload)
	if err != nil {
		return BookTop{}, err
	}

	if !delta.BidOK || !delta.AskOK {
		return BookTop{}, fmt.Errorf("incomplete top-of-book frame")
	}

	return BookTop{
		Symbol:  delta.Symbol,
		BestBid: delta.BestBid,
		BestAsk: delta.BestAsk,
	}, nil
}

func parseTopLevel(payload []byte, side string) (BookLevel, bool, error) {
	price, err := jsonparser.GetFloat(payload, "data", "[0]", side, "[0]", "price")
	if err != nil {
		if err == jsonparser.KeyPathNotFoundError {
			return BookLevel{}, false, nil
		}

		return BookLevel{}, false, fmt.Errorf("parse %s price: %w", side, err)
	}

	volume, err := jsonparser.GetFloat(payload, "data", "[0]", side, "[0]", "qty")
	if err != nil {
		if err == jsonparser.KeyPathNotFoundError {
			return BookLevel{}, false, nil
		}

		return BookLevel{}, false, fmt.Errorf("parse %s qty: %w", side, err)
	}

	return BookLevel{Price: price, Volume: volume}, true, nil
}
