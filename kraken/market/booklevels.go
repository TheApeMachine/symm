package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
)

// maxBookParseLevels caps the static buffer size; the actual parse cap is
// passed by the caller (from config.System.BookDepthLevels) so the parser
// always honours what the subscription frame requested. Without that, the
// subscriber could ask for 100 levels and the parser would silently
// truncate at 10, breaking the depth-fill VWAP after every reconnect.
// Threading the depth via a parameter avoids a config → market →
// config import cycle.
const maxBookParseLevels = 100

/*
BookLevelsDelta holds optional multi-level bid and ask updates from one
book frame. FrameType records whether the frame was a snapshot or update so
consumers can reset their local book on snapshot and otherwise apply
incrementally. Checksum carries Kraken's CRC32 for downstream
verification; a zero value means the frame did not include one.
*/
type BookLevelsDelta struct {
	Symbol    string
	FrameType string
	Checksum  uint32
	Bids      []BookLevel
	Asks      []BookLevel
	BidOK     bool
	AskOK     bool
}

/*
IsSnapshot reports whether the frame carries a full book state. Consumers
must replace their local book on snapshot and apply updates only between
snapshots.
*/
func (delta BookLevelsDelta) IsSnapshot() bool {
	return delta.FrameType == "snapshot"
}

/*
ParseBookLevelsDelta extracts up to maxBookParseLevels from each side of
a book frame. Callers that want to honour a configured depth should
invoke ParseBookLevelsDeltaWithDepth directly; this wrapper preserves the
original signature for transitional callers.
*/
func ParseBookLevelsDelta(payload []byte) (BookLevelsDelta, error) {
	return ParseBookLevelsDeltaWithDepth(payload, maxBookParseLevels)
}

/*
ParseBookLevelsDeltaWithDepth extracts up to depth levels from each side
of a book frame. depth is bounded above by maxBookParseLevels. A zero or
negative depth falls back to maxBookParseLevels so callers cannot
silently downgrade the parse below the subscription depth.
*/
func ParseBookLevelsDeltaWithDepth(payload []byte, depth int) (BookLevelsDelta, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return BookLevelsDelta{}, err
	}

	if !Channel(channel).IsBook() {
		return BookLevelsDelta{}, fmt.Errorf("not a book event: channel=%q", channel)
	}

	if depth <= 0 || depth > maxBookParseLevels {
		depth = maxBookParseLevels
	}

	bids, bidOK, err := parseSideLevels(payload, "bids", depth)
	if err != nil {
		return BookLevelsDelta{}, err
	}

	asks, askOK, err := parseSideLevels(payload, "asks", depth)
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

	frameType, _ := jsonparser.GetUnsafeString(payload, "type")

	var checksum uint32

	if value, err := jsonparser.GetInt(payload, "data", "[0]", "checksum"); err == nil && value >= 0 {
		checksum = uint32(value)
	}

	return BookLevelsDelta{
		Symbol:    string(symbol),
		FrameType: string(frameType),
		Checksum:  checksum,
		Bids:      bids,
		Asks:      asks,
		BidOK:     bidOK,
		AskOK:     askOK,
	}, nil
}

func parseSideLevels(payload []byte, side string, depth int) ([]BookLevel, bool, error) {
	if depth <= 0 {
		return nil, false, nil
	}

	levels := make([]BookLevel, 0, depth)

	for index := 0; index < depth; index++ {
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
