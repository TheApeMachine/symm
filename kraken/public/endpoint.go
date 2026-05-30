package public

type EndpointType string

const (
	BaseURL EndpointType = "https://api.kraken.com/0/public"

	EndpointTypeAssetPairs  EndpointType = BaseURL + "/AssetPairs"
	EndpointTypeTicker      EndpointType = BaseURL + "/Ticker"
	EndpointTypeOHLC        EndpointType = BaseURL + "/OHLC"
	EndpointTypeDepth       EndpointType = BaseURL + "/Depth"
	EndpointTypeGroupedBook EndpointType = BaseURL + "/GroupedBook"
	EndpointTypeTrades      EndpointType = BaseURL + "/Trades"
	EndpointTypeSpread      EndpointType = BaseURL + "/Spread"
	EndpointTypePostTrade   EndpointType = BaseURL + "/PostTrade"
	WebSocketURL            EndpointType = "wss://ws.kraken.com/v2"
	WebSocketAuthURL        EndpointType = "wss://ws-auth.kraken.com/v2"
	WebSocketL3URL          EndpointType = "wss://ws-l3.kraken.com/v2"
	TickerChannel                        = "ticker"
	BookChannel                          = "book"
	OrdersChannel                        = "orders"
	CandlesChannel                       = "candles"
	TradesChannel                        = "trades"
	InstrumentsChannel                   = "instruments"
	Level3Channel                        = "level3"
)
