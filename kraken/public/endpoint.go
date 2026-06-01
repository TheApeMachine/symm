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
	EndpointAddOrder        EndpointType = BaseURL + "/AddOrder"
	EndpointAmendOrder      EndpointType = BaseURL + "/AmendOrder"
	EndpointCancelOrder     EndpointType = BaseURL + "/CancelOrder"
	EndpointWebSocketsToken EndpointType = BaseURL + "/GetWebSocketsToken"
	WebSocketURL            EndpointType = "wss://ws.kraken.com/v2"
	WebSocketAuthURL        EndpointType = "wss://ws-auth.kraken.com/v2"
	WebSocketL3URL          EndpointType = "wss://ws-l3.kraken.com/v2"
	TickerChannel                        = "ticker"
	BookChannel                          = "book"
	OrdersChannel                        = "orders"
	CandlesChannel                       = "ohlc"
	TradesChannel                        = "trade"
	InstrumentsChannel                   = "instrument"
	Level3Channel                        = "level3"
)
