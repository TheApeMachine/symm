package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
)

const maxBookParseLevels = 10

/*
BookLevelsDelta holds optional multi-level bid and ask updates from one book frame.
*/
type BookLevelsDelta struct {
	Symbol string
	Bids   []BookLevel
	Asks   []BookLevel
	BidOK  bool
	AskOK  bool
}

/*
ParseBookLevelsDelta extracts up to maxBookParseLevels from each side of a book frame.
*/
func ParseBookLevelsDelta(payload []byte) (BookLevelsDelta, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return BookLevelsDelta{}, err
	}

	if !isBookChannel(channel) {
		return BookLevelsDelta{}, fmt.Errorf("not a book event: channel=%q", channel)
	}

	bids, bidOK, err := parseSideLevels(payload, "bids")
	if err != nil {
		return BookLevelsDelta{}, err
	}

	asks, askOK, err := parseSideLevels(payload, "asks")
	if err != nil {
		return BookLevelsDelta{}, err
	}

	if !bidOK && !askOK {
		return BookLevelsDelta{}, fmt.Errorf("book frame has no levels")
	}

	symbol, err := jsonparser.GetUnsafeString(payload, "data", "[0]", "symbol")
	if err != nil {
		return BookLevelsDelta{}, fmt.Errorf("parse book symbol: %w", err)
	}

	return BookLevelsDelta{
		Symbol: string(symbol),
		Bids:   bids,
		Asks:   asks,
		BidOK:  bidOK,
		AskOK:  askOK,
	}, nil
}

func parseSideLevels(payload []byte, side string) ([]BookLevel, bool, error) {
	levels := make([]BookLevel, 0, maxBookParseLevels)

	for index := range maxBookParseLevels {
		price, err := jsonparser.GetFloat(
			payload, "data", "[0]", side, fmt.Sprintf("[%d]", index), "price",
		)

		if err != nil {
			if err == jsonparser.KeyPathNotFoundError {
				break
			}

			return nil, false, fmt.Errorf("parse %s price: %w", side, err)
		}

		volume, err := jsonparser.GetFloat(
			payload, "data", "[0]", side, fmt.Sprintf("[%d]", index), "qty",
		)

		if err != nil {
			if err == jsonparser.KeyPathNotFoundError {
				break
			}

			return nil, false, fmt.Errorf("parse %s qty: %w", side, err)
		}

		levels = append(levels, BookLevel{Price: price, Volume: volume})
	}

	return levels, len(levels) > 0, nil
}
