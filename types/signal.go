package types

/*
StreamInterest names which public feeds a Signal needs before Measure is worth
running on a cut. Bits compose; zero means the signal opted out of this cut.
*/
type StreamInterest uint8

const (
	StreamTicker StreamInterest = 1 << iota
	StreamTrade
	StreamBook
	StreamAll = StreamTicker | StreamTrade | StreamBook
)

/*
Signal conditions one market input into numerical measurements. Market
interpretations are deliberately absent because they belong to logic.
*/
type Signal interface {
	Measure(*Thesis) []*Measurement
}

/*
Interested optionally advertises which feeds a Signal consumes so Planner can
skip workers when a cut lacks those streams.
*/
type Interested interface {
	Interest() StreamInterest
}

/*
SignalInterest returns a Signal's feed mask, defaulting to StreamAll when the
concrete type does not implement Interested.
*/
func SignalInterest(signal Signal) StreamInterest {
	if interested, ok := signal.(Interested); ok {
		return interested.Interest()
	}

	return StreamAll
}

/*
FrameInterest derives the streams present in a market cut.
*/
func FrameInterest(frame *MarketFrame) StreamInterest {
	if frame == nil {
		return 0
	}

	var mask StreamInterest

	if len(frame.Tickers) > 0 {
		mask |= StreamTicker
	}

	if len(frame.Trades) > 0 {
		mask |= StreamTrade
	}

	if len(frame.Books) > 0 {
		mask |= StreamBook
	}

	return mask
}
