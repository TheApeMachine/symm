package websocket

import (
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

const TradeVolumeEndpoint = "/0/private/TradeVolume"

/*
Conn is the internal websocket and REST transport.
*/
type Conn interface {
	Client() *spot.WebSocket
	On(channel string, action func([]byte))
	Write(params json.Marshaler) error
	Close()
	Post(path string, params json.Marshaler) ([]byte, error)
}

/*
API is the single Kraken transport surface for symm.
Callers subscribe, order, and listen through named methods only.
*/
type API struct {
	public  Conn
	private Conn
	paper   *Paper
	live    bool
}

func NewAPI(public, private Conn, paper *Paper) *API {
	api := &API{
		public:  public,
		private: private,
		paper:   paper,
		live:    viper.GetViper().GetString("trading.model") == "live",
	}

	return api
}

func (api *API) Close() {
	api.public.Close()
	api.private.Close()
}

func (api *API) On(channel string, action func([]byte)) {
	switch channel {
	case "balances", "executions", "add_order", "level3":
		api.private.On(channel, action)
	default:
		api.public.On(channel, action)
	}
}

func (api *API) TradeVolume(symbols []string) (*kraken.TradeVolume, error) {
	response, err := api.private.Post(
		TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade volume",
			err,
		))
	}

	return kraken.NewTradeVolume(response), nil
}

func (api *API) SubscribeInstruments() error {
	return errnie.Error(api.public.Client().SubInstruments())
}

func (api *API) SubscribeTicker(pairs []string) error {
	return errnie.Error(api.public.Client().SubTicker(pairs))
}

func (api *API) SubscribeTrade(pairs []string) error {
	return errnie.Error(api.public.Client().SubTrades(pairs))
}

func (api *API) SubscribeBook(pairs []string) error {
	return errnie.Error(api.public.Client().SubBook(
		pairs, viper.GetInt("market.book.depth"), nil,
	))
}

func (api *API) SubscribeOHLC(pairs []string) error {
	return errnie.Error(api.public.Client().SubCandles(pairs, map[string]any{
		"interval": viper.GetInt("market.ohlc.interval"),
	}))
}

func (api *API) SubscribeLevel3(pairs []string) error {
	return errnie.Error(api.private.Client().SubL3(
		pairs,
		viper.GetInt("market.l3_depth"),
		nil,
	))
}

func (api *API) SubscribeBalance() error {
	if api.live {
		return errnie.Error(api.private.Client().SubBalances())
	}

	return api.paper.SubBalances()
}

func (api *API) SubscribeExecutions(symbols []string) error {
	if api.live {
		return errnie.Error(api.private.Client().SubExecutions(map[string]any{
			"symbols": symbols,
		}))
	}

	return api.paper.SubExecutions(map[string]any{
		"symbols": symbols,
	})
}

func (api *API) AddOrder(order *kraken.MarketOrder) error {
	if api.live {
		return api.private.Write(order)
	}

	return api.paper.AddOrder(order)
}
