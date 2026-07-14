package websocket

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
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
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	public    Conn
	private   Conn
	paper     *Paper
	live      bool
	bookConns *sync.Map
	level3    []func([]byte)
}

func NewAPI(
	ctx context.Context, public, private Conn, paper *Paper,
) *API {
	ctx, cancel := context.WithCancel(ctx)

	api := &API{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.INITIALIZING,
		public:    public,
		private:   private,
		paper:     paper,
		live:      viper.GetViper().GetString("trading.model") == "live",
		bookConns: &sync.Map{},
		level3:    make([]func([]byte), 0),
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
}

/*
On registers a channel consumer on the transport that actually owns the
channel. Level3 consumers are retained because those transports are created
later, in batches, after the instrument universe arrives.
*/
func (api *API) On(channel string, action func([]byte)) {
	switch channel {
	case "balances", "executions", "add_order":
		if api.live {
			api.private.On(channel, action)
			return
		}

		api.paper.On(channel, action)
	case "level3":
		api.level3 = append(api.level3, action)
		api.bookConns.Range(func(_, value any) bool {
			value.(*Live).On(channel, action)

			return true
		})
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

func (api *API) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		api.bookConns.Range(func(key, value any) bool {
			if !yield(value.(*Live).Books()) {
				return false
			}

			return true
		})
	}
}

func (api *API) SubscribeBook(pairs []string) error {
	return errnie.Error(api.public.Client().SubBook(
		pairs, viper.GetInt("market.book.depth"), nil,
	))
}

/*
SubscribeLevel3 assigns each symbol batch its own authenticated book transport.
The transport subscribes after authentication and repeats that same request
after reconnect, so this method must not send a second competing subscription.
*/
func (api *API) SubscribeLevel3(pairs []string) error {
	key := strings.Join(pairs, "|")

	if _, loaded := api.bookConns.Load(key); loaded {
		return nil
	}

	live := New(
		api.ctx, nil, true, Level3WebSocketURL,
	)

	live.symbols = pairs

	for _, action := range api.level3 {
		live.On("level3", action)
	}

	if err := live.Initialize(); err != nil {
		live.Close()
		return errnie.Error(err)
	}

	_, loaded := api.bookConns.LoadOrStore(key, live)

	if loaded {
		live.Close()
	}

	return nil
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
