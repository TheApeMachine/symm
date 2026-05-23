package market

import "github.com/theapemachine/symm/kraken/core"

/*
SubscribeParams is the Kraken WebSocket v2 public channel subscribe payload.
*/
type SubscribeParams struct {
	Channel                string   `json:"channel"`
	Symbol                 []string `json:"symbol,omitempty"`
	Depth                  int      `json:"depth,omitempty"`
	Snapshot               bool     `json:"snapshot,omitempty"`
	EventTrigger           string   `json:"event_trigger,omitempty"`
	ExecutionVenue         string   `json:"execution_venue,omitempty"`
	IncludeTokenizedAssets bool     `json:"include_tokenized_assets,omitempty"`
}

/*
Instrument builds instrument channel subscription params.
*/
func (subscribeParams SubscribeParams) Instrument() SubscribeParams {
	return SubscribeParams{
		Channel:  core.ChannelInstrument,
		Snapshot: true,
	}
}

/*
Trades builds trade channel subscription params.
*/
func (subscribeParams SubscribeParams) Trades(symbols []string) SubscribeParams {
	return SubscribeParams{
		Channel:  "trade",
		Symbol:   symbols,
		Snapshot: true,
	}
}

/*
Book builds book channel subscription params.
*/
func (subscribeParams SubscribeParams) Book(symbols []string, depth int) SubscribeParams {
	if depth <= 0 {
		depth = 10
	}

	return SubscribeParams{
		Channel:  core.ChannelBook,
		Symbol:   symbols,
		Depth:    depth,
		Snapshot: true,
	}
}

/*
Ticker builds ticker channel subscription params.
*/
func (subscribeParams SubscribeParams) Ticker(symbols []string) SubscribeParams {
	return SubscribeParams{
		Channel:      core.ChannelTicker,
		Symbol:       symbols,
		Snapshot:     true,
		EventTrigger: "trades",
	}
}
