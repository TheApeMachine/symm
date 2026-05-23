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
ParseTopBook extracts the top bid and ask from a Kraken v2 book snapshot or delta.
It reads only data[0].bids[0] and data[0].asks[0].
*/
func ParseTopBook(payload []byte) (BookTop, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return BookTop{}, err
	}

	if !isBookChannel(channel) {
		return BookTop{}, fmt.Errorf("not a book event: channel=%q", channel)
	}

	bestBid, bidOK, err := parseTopLevel(payload, "bids")
	if err != nil {
		return BookTop{}, err
	}

	bestAsk, askOK, err := parseTopLevel(payload, "asks")
	if err != nil {
		return BookTop{}, err
	}

	if !bidOK && !askOK {
		return BookTop{}, fmt.Errorf("book frame has no top-of-book levels")
	}

	symbol, err := jsonparser.GetUnsafeString(payload, "data", "[0]", "symbol")
	if err != nil {
		return BookTop{}, fmt.Errorf("parse book symbol: %w", err)
	}

	return BookTop{
		Symbol:  string(symbol),
		BestBid: bestBid,
		BestAsk: bestAsk,
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
