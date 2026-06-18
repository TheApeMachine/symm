package market

/*
BookParams is the Kraken WebSocket v2 subscribe payload for the book channel.
*/
type BookParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
}

/*
TickerParams is the Kraken WebSocket v2 subscribe payload for the ticker channel.
*/
type TickerParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
}

/*
TradeParams is the Kraken WebSocket v2 subscribe payload for the trade channel.
*/
type TradeParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
}

/*
InstrumentParams is the Kraken WebSocket v2 subscribe payload for the instrument channel.
*/
type InstrumentParams struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
}

func NewBookParams(symbols []string, depth int) BookParams {
	return BookParams{
		Channel:  "book",
		Symbol:   symbols,
		Depth:    depth,
		Snapshot: true,
	}
}

func NewTickerParams(symbols []string) TickerParams {
	return TickerParams{
		Channel:  "ticker",
		Symbol:   symbols,
		Snapshot: true,
	}
}

func NewTradeParams(symbols []string) TradeParams {
	return TradeParams{
		Channel:  "trade",
		Symbol:   symbols,
		Snapshot: true,
	}
}

func NewInstrumentParams() InstrumentParams {
	return InstrumentParams{
		Channel:  "instrument",
		Snapshot: true,
	}
}
