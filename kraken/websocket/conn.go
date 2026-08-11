package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
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
	Books() *sync.Map
	Book(string, func(*book.Book))
	BookUpdates() <-chan string
	SubInstrument(types.Subscription[any])
	SubTicker([]string)
	SubBook([]string)
	SubTrades([]string)
	SubL3([]string)
	SubCandles([]string)
	Balance() (map[string]*decimal.Decimal, error)
	TradesHistory() (spot.TradesHistoryResult, error)
	TradeBalance() (kraken.TradeBalanceResult, error)
	TradeVolume([]string) (*kraken.TradeVolumeResult, error)
	AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error)
	OpenOrders() (spot.OpenOrdersResult, error)
	CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error)
	Write(json.Marshaler, ...Callback[any]) error
	Post(string, json.Marshaler) ([]byte, error)
	Client() *spot.WebSocket
	Close()
}

/*
BookSource exposes the authoritative live Level 3 cache to measurement stages.
*/
type BookSource interface {
	Book(string, func(*book.Book))
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
	normalizer *spot.Normalizer
	public     Conn
	private    Conn
}

func NewAPI(
	ctx context.Context, public, private Conn,
) *API {
	ctx, cancel := context.WithCancel(ctx)
	normalizer := spot.NewNormalizer()

	if private != nil {
		client := private.Client()

		if client != nil && client.REST != nil {
			errnie.Error(normalizer.Use(client.REST))
		}
	}

	api := &API{
		ctx:        ctx,
		cancel:     cancel,
		normalizer: normalizer,
		public:     public,
		private:    private,
	}

	return api
}

/*
Status returns the API lifecycle state used by ordered system boot stages.
*/
func (api *API) Status() types.Status {
	if api.public == nil || api.private == nil {
		return types.PENDING
	}

	if api.public.Status() != types.READY || api.private.Status() != types.READY {
		return types.PENDING
	}

	return types.READY
}

/*
Normalizer returns the internal [spot.Normalizer] used to normalize asset names.
*/
func (api *API) Normalizer() *spot.Normalizer {
	return api.normalizer
}

func (api *API) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	if key == "executions" || key == "add_order" {
		return api.private.Subscribe(key, subscription)
	}

	return api.public.Subscribe(key, subscription)

}

func (api *API) Books() *sync.Map                                 { return api.private.Books() }
func (api *API) Book(symbol string, read func(*book.Book))        { api.private.Book(symbol, read) }
func (api *API) BookUpdates() <-chan string                       { return api.private.BookUpdates() }
func (api *API) SubInstrument(callback types.Subscription[any])   { api.public.SubInstrument(callback) }
func (api *API) SubTicker(symbols []string)                       { api.public.SubTicker(symbols) }
func (api *API) SubBook(symbols []string)                         { api.public.SubBook(symbols) }
func (api *API) SubL3(symbols []string)                           { api.private.SubL3(symbols) }
func (api *API) SubTrades(symbols []string)                       { api.public.SubTrades(symbols) }
func (api *API) SubCandles(symbols []string)                      { api.public.SubCandles(symbols) }
func (api *API) Balance() (map[string]*decimal.Decimal, error)    { return api.private.Balance() }
func (api *API) TradesHistory() (spot.TradesHistoryResult, error) { return api.private.TradesHistory() }
func (api *API) TradeBalance() (kraken.TradeBalanceResult, error) { return api.private.TradeBalance() }

func (api *API) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	normalized := append([]string{}, symbols...)

	for index, symbol := range normalized {
		normalized[index] = api.normalizer.Name(symbol)
	}

	return api.private.TradeVolume(normalized)
}

func (api *API) AddOrder(request *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return api.private.AddOrder(request)
}

func (api *API) OpenOrders() (spot.OpenOrdersResult, error) {
	return api.private.OpenOrders()
}

func (api *API) CancelOrder(request *spot.CancelOrderRequest) (spot.CancelResult, error) {
	return api.private.CancelOrder(request)
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
