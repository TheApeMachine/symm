package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Conn is the internal websocket and REST transport.
*/
type Conn interface {
	Status() runtime.Stage
	Transition(runtime.Stage)
	Books() *sync.Map
	Book(string, func(*book.Book))
	SubInstrument(chan any)
	SubTicker([]string)
	SubTrades([]string)
	SubL3([]string)
	UnsubTicker([]string)
	UnsubTrades([]string)
	UnsubL3([]string)
	Balance() (*kraken.Balance, error)
	TradesHistory() (spot.TradesHistoryResult, error)
	TradeBalance() (*kraken.TradeBalanceResult, error)
	TradeVolume([]string) (*kraken.TradeVolumeResult, error)
	AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error)
	OpenOrders() (spot.OpenOrdersResult, error)
	CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error)
	Write(json.Marshaler, ...Callback[any]) error
	Post(string, json.Marshaler) ([]byte, error)
	Client() *spot.WebSocket
	Close() error
}

/*
BookSource exposes the authoritative live Level 3 cache to measurement stages.
*/
type BookSource interface {
	Book(string, func(*book.Book))
}

/*
Callback pairs one one-shot response channel with the wire channel name that
produces it. The instrument snapshot is the only one-shot callback left; market
frames now fan out onto the thesis symbol queues.
*/
type Callback[T any] struct {
	Channel string
	Message chan T
}

func (callback Callback[T]) Send(message T) {
	defer close(callback.Message)
	callback.Message <- message
}

/*
API is the single Kraken transport surface for symm.
Callers subscribe, order, and listen through named methods only.
*/
type API struct {
	*runtime.System
	normalizer *spot.Normalizer
	public     Conn
	private    Conn
	futures    *FuturesLive
}

func NewAPI(
	ctx context.Context, public, private Conn, futures *FuturesLive,
) *API {
	normalizer := spot.NewNormalizer()

	api := &API{
		System:     runtime.NewSystem(ctx, "kraken", public, private, futures),
		normalizer: normalizer,
		public:     public,
		private:    private,
		futures:    futures,
	}

	if err := errnie.Require(map[string]any{
		"public":  public,
		"private": private,
		"futures": futures,
	}); err != nil {
		api.Error(err)
		return api
	}

	api.Transition(runtime.WAITING)

	client := private.Client()

	if client == nil || client.REST == nil {
		return api
	}

	if err := normalizer.Use(client.REST); err != nil {
		api.Error(errnie.Err(
			errnie.Validation,
			"websocket api: failed to initialize normalizer",
			err,
		))
	}

	return api
}

func (api *API) Futures() *FuturesLive {
	return api.futures
}

func (api *API) Name() string { return "kraken" }


/*
Normalizer returns the internal [spot.Normalizer] used to normalize asset names.
*/
func (api *API) Normalizer() *spot.Normalizer {
	return api.normalizer
}

func (api *API) Transition(stage runtime.Stage) {
	api.System.Transition(stage)

	if api.public != nil {
		api.public.Transition(stage)
	}

	if api.futures != nil && api.futures.System != nil {
		api.futures.Transition(stage)
	}

	if api.private != nil {
		api.private.Transition(stage)
	}
}


func (api *API) Private() Conn                             { return api.private }
func (api *API) Books() *sync.Map                          { return api.private.Books() }
func (api *API) Book(symbol string, read func(*book.Book)) { api.private.Book(symbol, read) }
func (api *API) SubInstrument(callback chan any)           { api.public.SubInstrument(callback) }
func (api *API) SubTicker(symbols []string)                { api.public.SubTicker(symbols) }
func (api *API) SubL3(symbols []string)                    { api.private.SubL3(symbols) }
func (api *API) SubTrades(symbols []string)                { api.public.SubTrades(symbols) }
func (api *API) UnsubTicker(symbols []string)              { api.public.UnsubTicker(symbols) }
func (api *API) UnsubTrades(symbols []string)              { api.public.UnsubTrades(symbols) }
func (api *API) UnsubL3(symbols []string)                  { api.private.UnsubL3(symbols) }
func (api *API) Balance() (*kraken.Balance, error) {
	balance, err := api.private.Balance()

	if err != nil {
		return nil, api.Error(err)
	}

	assets := make(map[string]*decimal.Decimal, len(balance.Data))

	for _, row := range balance.Data {
		assets[api.normalizer.Name(row.Asset)] = row.Balance
	}

	return kraken.NewBalanceFromMap(assets), nil
}
func (api *API) TradesHistory() (spot.TradesHistoryResult, error)  { return api.private.TradesHistory() }
func (api *API) TradeBalance() (*kraken.TradeBalanceResult, error) { return api.private.TradeBalance() }

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

func (api *API) ResetPaper() error {
	return ResetPaperAccount(api.Context())
}

func (api *API) SubFuturesTicker(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for ticker subscription",
			nil,
		))
	}

	return api.futures.SubFuturesTicker(productIDs)
}

func (api *API) SubFuturesTrades(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for trade subscription",
			nil,
		))
	}

	return api.futures.SubFuturesTrades(productIDs)
}

func (api *API) SubFuturesBook(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for book subscription",
			nil,
		))
	}

	return api.futures.SubFuturesBook(productIDs)
}

func (api *API) UnsubFuturesTicker(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for ticker unsubscription",
			nil,
		))
	}

	return api.futures.UnsubFuturesTicker(productIDs)
}

func (api *API) UnsubFuturesTrades(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for trade unsubscription",
			nil,
		))
	}

	return api.futures.UnsubFuturesTrades(productIDs)
}

func (api *API) UnsubFuturesBook(productIDs []string) error {
	if api.futures == nil {
		return api.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for book unsubscription",
			nil,
		))
	}

	return api.futures.UnsubFuturesBook(productIDs)
}
