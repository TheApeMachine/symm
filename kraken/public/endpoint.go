package public

import "strings"

type EndpointType string

const (
	PublicBaseURL  EndpointType = "https://api.kraken.com/0/public"
	PrivateBaseURL EndpointType = "https://api.kraken.com/0/private"

	BaseURL = PublicBaseURL

	EndpointTypeAssetPairs  EndpointType = PublicBaseURL + "/AssetPairs"
	EndpointTypeTicker      EndpointType = PublicBaseURL + "/Ticker"
	EndpointTypeOHLC        EndpointType = PublicBaseURL + "/OHLC"
	EndpointTypeDepth       EndpointType = PublicBaseURL + "/Depth"
	EndpointTypeGroupedBook EndpointType = PublicBaseURL + "/GroupedBook"
	EndpointTypeTrades      EndpointType = PublicBaseURL + "/Trades"
	EndpointTypeSpread      EndpointType = PublicBaseURL + "/Spread"
	EndpointTypePostTrade   EndpointType = PublicBaseURL + "/PostTrade"

	EndpointAddOrder        EndpointType = PrivateBaseURL + "/AddOrder"
	EndpointAmendOrder      EndpointType = PrivateBaseURL + "/AmendOrder"
	EndpointCancelOrder     EndpointType = PrivateBaseURL + "/CancelOrder"
	EndpointWebSocketsToken EndpointType = PrivateBaseURL + "/GetWebSocketsToken"
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
	ExecutionsChannel                    = "executions"
	BalancesChannel                      = "balances"
)

/*
SignPath returns the URI path Kraken uses when computing API-Sign.
*/
func (endpoint EndpointType) SignPath() string {
	return strings.TrimPrefix(string(endpoint), "https://api.kraken.com")
}
