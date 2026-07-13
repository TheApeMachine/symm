package websocket

import (
	"encoding/json"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
	Books() *spot.BookManager
}

/*
API is the single Kraken transport surface for symm.
Callers subscribe, order, and listen through named methods only.
*/
type API struct {
	status         types.Status
	public         Conn
	private        Conn
	level3         Conn
	paper          *Paper
	live           bool
	normalizer     *spot.Normalizer
	normalizerOnce sync.Once
	normalizerErr  error
}

func NewAPI(public, private, level3 Conn, paper *Paper) *API {
	api := &API{
		status:     types.INITIALIZING,
		public:     public,
		private:    private,
		level3:     level3,
		paper:      paper,
		live:       viper.GetViper().GetString("trading.model") == "live",
		normalizer: spot.NewNormalizer(),
	}

	return api
}

func (api *API) Initialize() error {
	errnie.Info("initializing API")
	api.status = types.READY
	return nil
}

func (api *API) Close() {
	api.public.Close()
	api.private.Close()

	if api.level3 != api.private {
		api.level3.Close()
	}
}

func (api *API) On(channel string, action func([]byte)) {
	switch channel {
	case "balances", "executions", "add_order":
		if api.live {
			api.private.On(channel, action)
			return
		}

		api.paper.On(channel, action)
	case "level3":
		api.level3.On(channel, action)
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

func (api *API) TradesHistory() (*kraken.TradesHistory, error) {
	if !api.live {
		return api.paper.TradesHistory()
	}

	response, err := api.private.Client().REST.TradesHistory(&spot.TradesHistoryRequest{
		Type:             "all",
		Trades:           true,
		ConsolidateTaker: true,
		Ledgers:          false,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	return &kraken.TradesHistory{
		Result: kraken.TradesHistoryResult{
			Trades: response.Result.Trades,
		},
	}, nil
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

func (api *API) Books() *spot.BookManager {
	return api.level3.Books()
}

func (api *API) SubscribeBook(pairs []string) error {
	return errnie.Error(api.public.Client().SubBook(
		pairs, viper.GetInt("market.book.depth"), nil,
	))
}

func (api *API) SubscribeLevel3(pairs []string) error {
	depth := viper.GetInt("market.l3_depth")

	// SubL3 accepts depth but does not place it on the wire, so the
	// BookManager would otherwise fall back to its own default depth.
	return errnie.Error(api.level3.Client().SubL3(
		pairs, depth, map[string]any{
			"params": map[string]any{"depth": depth},
		},
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
