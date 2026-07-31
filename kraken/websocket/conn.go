package websocket

import (
	"context"
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

const (
	TradeBalanceEndpoint = "/0/private/TradeBalance"
	TradeVolumeEndpoint  = "/0/private/TradeVolume"
)

/*
Conn is the internal websocket and REST transport.
*/
type Conn interface {
	Status() types.Status
	Subscribe(string, *types.Subscription[any]) *types.Subscription[any]
	Books() map[string]*book.Book
	Book(string) *book.Book
	SubInstrument(types.Subscription[any])
	SubTicker([]string)
	SubBook([]string)
	SubTrades([]string)
	SubL3([]string)
	SubCandles([]string)
	Balance() (map[string]*decimal.Decimal, error)
	TradeBalance() (spot.TradesHistoryResult, error)
	TradeVolume([]string) (*kraken.TradeVolumeResult, error)
	AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error)
	Write(json.Marshaler, ...Callback[any]) error
	Post(string, json.Marshaler) ([]byte, error)
	Client() *spot.WebSocket
	Close()
}

type Callback[T any] struct {
	Channel      string
	Subscription types.Subscription[T]
}

func (callback Callback[T]) Send(message T) {
	defer close(callback.Subscription.Channel)
	callback.Subscription.Send(message)
}

/*
API is the single Kraken transport surface for symm.
Callers subscribe, order, and listen through named methods only.
*/
type API struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	normalizer *spot.Normalizer
	public     Conn
	private    Conn
}

func NewAPI(
	ctx context.Context, public, private Conn,
) *API {
	ctx, cancel := context.WithCancel(ctx)

	api := &API{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.INITIALIZING,
		normalizer: spot.NewNormalizer(),
		public:     public,
		private:    private,
	}

	return api
}

/*
Status returns the API lifecycle state used by ordered system boot stages.
*/
func (api *API) Status() types.Status {
	return api.status
}

func (api *API) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	switch key {
	case "ticker":
		return api.public.Subscribe(key, subscription)
	case "instrument":
		return api.public.Subscribe(key, subscription)
	}

	return nil
}

func (api *API) Books() map[string]*book.Book                    { return api.private.Books() }
func (api *API) Book(symbol string) *book.Book                   { return api.private.Book(symbol) }
func (api *API) SubInstrument(callback types.Subscription[any])  { api.public.SubInstrument(callback) }
func (api *API) SubTicker(symbols []string)                      { api.public.SubTicker(symbols) }
func (api *API) SubBook(symbols []string)                        { api.public.SubBook(symbols) }
func (api *API) SubTrades(symbols []string)                      { api.public.SubTrades(symbols) }
func (api *API) SubL3(symbols []string)                          { api.private.SubL3(symbols) }
func (api *API) SubCandles(symbols []string)                     { api.public.SubCandles(symbols) }
func (api *API) Balance() (map[string]*decimal.Decimal, error)   { return api.private.Balance() }
func (api *API) TradeBalance() (spot.TradesHistoryResult, error) { return api.private.TradeBalance() }

func (api *API) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	return api.private.TradeVolume(symbols)
}

func (api *API) AddOrder(request *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return api.private.AddOrder(request)
}

func (api *API) Close() {
	if api.public != nil {
		api.public.Close()
	}

	if api.private != nil {
		api.private.Close()
	}

	api.cancel()
}
