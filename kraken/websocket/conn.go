package websocket

import (
	"encoding/json"
	"fmt"
	"sync"

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
		public:     public,
		private:    private,
		level3:     level3,
		paper:      paper,
		live:       viper.GetViper().GetString("trading.model") == "live",
		normalizer: spot.NewNormalizer(),
	}

	return api
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
	if err := api.useNormalizer(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to initialize Kraken symbol normalizer",
			err,
		))
	}

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

	tradeVolume := kraken.NewTradeVolume(response)

	if err := kraken.Validate(tradeVolume); err != nil {
		return nil, err
	}

	fees := make(map[string]kraken.TradeVolumeFees, len(tradeVolume.Result.Fees))

	for canonical, fee := range tradeVolume.Result.Fees {
		symbol := api.normalizer.Name(canonical)

		if _, duplicate := fees[symbol]; duplicate {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"duplicate normalized trade volume symbol "+symbol,
				nil,
			))
		}

		fees[symbol] = fee
	}

	makerFees := make(map[string]kraken.TradeVolumeFees, len(tradeVolume.Result.FeesMaker))

	for canonical, fee := range tradeVolume.Result.FeesMaker {
		symbol := api.normalizer.Name(canonical)

		if _, duplicate := makerFees[symbol]; duplicate {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"duplicate normalized maker fee symbol "+symbol,
				nil,
			))
		}

		makerFees[symbol] = fee
	}

	tradeVolume.Result.Fees = fees
	tradeVolume.Result.FeesMaker = makerFees
	return tradeVolume, nil
}

func (api *API) useNormalizer() error {
	api.normalizerOnce.Do(func() {
		client := api.public.Client()

		if client == nil || client.REST == nil {
			api.normalizerErr = fmt.Errorf("Kraken public REST client required")
			return
		}

		api.normalizerErr = api.normalizer.Use(client.REST)
	})

	return api.normalizerErr
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

func (api *API) SymbolForAsset(asset string, quote string) (string, error) {
	if err := api.useNormalizer(); err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to initialize Kraken symbol normalizer",
			err,
		))
	}

	base, ok := api.normalizer.AssetName(asset)

	if !ok {
		return "", errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset not found for "+asset,
			nil,
		))
	}

	quoteAsset, ok := api.normalizer.AssetName(quote)

	if !ok {
		return "", errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote asset not found for "+quote,
			nil,
		))
	}

	return base.Name + "/" + quoteAsset.Name, nil
}

func (api *API) PairName(pair string) string {
	if err := api.useNormalizer(); err != nil {
		return pair
	}

	return api.normalizer.Name(pair)
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
		"params": map[string]any{
			"interval": viper.GetInt("market.ohlc.interval"),
		},
	}))
}

func (api *API) SubscribeLevel3(pairs []string) error {
	return errnie.Error(api.level3.Client().SubPrivate("level3", map[string]any{
		"params": map[string]any{
			"symbol": pairs,
			"depth":  viper.GetInt("market.l3_depth"),
		},
	}))
}

func (api *API) UnsubscribeLevel3(pairs []string) error {
	return errnie.Error(api.level3.Client().SendPrivate(map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "level3",
			"symbol":  pairs,
		},
	}))
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
