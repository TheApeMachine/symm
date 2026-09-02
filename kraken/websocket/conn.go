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

const (
	TradeBalanceEndpoint = "/0/private/TradeBalance"
	TradeVolumeEndpoint  = "/0/private/TradeVolume"
)

/*
Conn is the internal websocket and REST transport.
*/
type Conn interface {
	Status() runtime.Stage
	MarkReady()
	SubInstrument(chan any)
	SubTicker([]string)
	SubTrades([]string)
	SubL3([]string)
	UnsubTicker([]string)
	UnsubTrades([]string)
	UnsubL3([]string)
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
failureSource is implemented by required live transports. Test and paper
connections need not manufacture lifecycle machinery, while API can still bind
every production session to the same fail-fast supervisor.
*/
type failureSource interface {
	Error() error
	SetFailure(func(error))
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
	ctx         context.Context
	cancel      context.CancelFunc
	status      *runtime.Status
	errMu       sync.RWMutex
	err         error
	failures    chan error
	failureOnce sync.Once
	normalizer  *spot.Normalizer
	public      Conn
	private     Conn
	futures     *FuturesLive
}

func NewAPI(
	ctx context.Context, public, private Conn,
) *API {
	ctx, cancel := context.WithCancel(ctx)
	normalizer := spot.NewNormalizer()

	api := &API{
		ctx:        ctx,
		cancel:     cancel,
		status:     runtime.NewStatus(),
		normalizer: normalizer,
		failures:   make(chan error, 1),
		public:     public,
		private:    private,
	}
	api.status.Transition(runtime.WAITING)
	api.bindFailureSource(public)
	api.bindFailureSource(private)

	if private == nil {
		return api
	}

	client := private.Client()

	if client == nil || client.REST == nil {
		return api
	}

	if err := normalizer.Use(client.REST); err != nil {
		api.reportFailure(errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket api: failed to initialize normalizer",
			err,
		)))
	}

	return api
}

func (api *API) SetFutures(futures *FuturesLive) {
	api.futures = futures
	api.bindFailureSource(futures)
}

func (api *API) Futures() *FuturesLive {
	return api.futures
}

func (api *API) Name() string { return "kraken" }

func (api *API) Error() error {
	if api == nil {
		return nil
	}

	api.errMu.RLock()
	defer api.errMu.RUnlock()

	return api.err
}

/*
Done closes when the API supervisor is canceled or any required transport
fails. Blocking boot operations use it instead of waiting forever for a reply.
*/
func (api *API) Done() <-chan struct{} {
	return api.ctx.Done()
}

/*
Context is the lifecycle parent for required transport consumers created during
boot.
*/
func (api *API) Context() context.Context {
	return api.ctx
}

func (api *API) Run() error {
	if err := api.Error(); err != nil {
		return err
	}

	select {
	case <-api.ctx.Done():
		if err := api.Error(); err != nil {
			return err
		}

		return api.ctx.Err()
	case err := <-api.failures:
		return err
	}
}

func (api *API) reportFailure(err error) {
	if api == nil || err == nil {
		return
	}

	api.failureOnce.Do(func() {
		api.errMu.Lock()
		api.err = err
		api.errMu.Unlock()

		if api.status != nil {
			api.status.Transition(runtime.ERROR)
		}

		api.cancel()
		api.failures <- err
	})
}

func (api *API) bindFailureSource(source any) {
	failing, ok := source.(failureSource)

	if !ok || failing == nil {
		return
	}

	failing.SetFailure(api.reportFailure)
}

/*
Status returns the API lifecycle state used by ordered system boot stages.
*/
func (api *API) Status() runtime.Stage {
	if api == nil {
		return runtime.INIT
	}

	if api.Error() != nil {
		return runtime.ERROR
	}

	if api.public == nil || api.private == nil {
		return runtime.INIT
	}

	if api.public.Status() != runtime.READY || api.private.Status() != runtime.READY {
		return runtime.INIT
	}

	if api.futures == nil || api.futures.Status() != runtime.READY {
		return runtime.INIT
	}

	if api.status == nil {
		return runtime.READY
	}

	return api.status.Current()
}

/*
Normalizer returns the internal [spot.Normalizer] used to normalize asset names.
*/
func (api *API) Normalizer() *spot.Normalizer {
	return api.normalizer
}

/*
MarkReady releases every configured market-data session after the complete
consumer graph has been admitted. Market subscriptions are issued only after
this boundary, so their authoritative snapshots cannot arrive while an ingress
workload is still waiting.
*/
func (api *API) MarkReady() {
	if api == nil {
		return
	}

	if api.Error() != nil {
		return
	}

	if api.public == nil || api.private == nil || api.futures == nil {
		api.reportFailure(errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: every required transport must be configured before readiness",
			nil,
		)))

		return
	}

	// Private is released last because its READY transition wakes the execution
	// subscription. At that instant every other required transport and the API
	// lifecycle itself must already be ready.
	api.public.MarkReady()
	api.futures.MarkReady()

	if api.Error() != nil {
		return
	}

	if api.status != nil {
		api.status.Transition(runtime.READY)
	}

	api.private.MarkReady()
}

func (api *API) SubInstrument(callback chan any)                  { api.public.SubInstrument(callback) }
func (api *API) SubTicker(symbols []string)                       { api.public.SubTicker(symbols) }
func (api *API) SubL3(symbols []string)                           { api.private.SubL3(symbols) }
func (api *API) SubTrades(symbols []string)                       { api.public.SubTrades(symbols) }
func (api *API) UnsubTicker(symbols []string)                     { api.public.UnsubTicker(symbols) }
func (api *API) UnsubTrades(symbols []string)                     { api.public.UnsubTrades(symbols) }
func (api *API) UnsubL3(symbols []string)                         { api.private.UnsubL3(symbols) }
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

func (api *API) SubFuturesTicker(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for ticker subscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.SubFuturesTicker(productIDs)
}

func (api *API) SubFuturesTrades(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for trade subscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.SubFuturesTrades(productIDs)
}

func (api *API) SubFuturesBook(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for book subscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.SubFuturesBook(productIDs)
}

func (api *API) UnsubFuturesTicker(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for ticker unsubscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.UnsubFuturesTicker(productIDs)
}

func (api *API) UnsubFuturesTrades(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for trade unsubscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.UnsubFuturesTrades(productIDs)
}

func (api *API) UnsubFuturesBook(productIDs []string) error {
	if api.futures == nil {
		err := errnie.Error(errnie.Err(
			errnie.NotFound,
			"websocket api: futures transport is required for book unsubscription",
			nil,
		))
		api.reportFailure(err)

		return err
	}

	return api.futures.UnsubFuturesBook(productIDs)
}

func (api *API) Close() {
	if api == nil {
		return
	}

	api.cancel()

	if api.public != nil {
		api.public.Close()
	}

	if api.private != nil {
		api.private.Close()
	}

	if api.futures != nil {
		errnie.Error(api.futures.Close())
	}

	if api.Error() == nil && api.status != nil {
		api.status.Transition(runtime.DONE)
	}
}
