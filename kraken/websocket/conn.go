package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

const (
	TradeVolumeEndpoint = "/0/private/TradeVolume"
)

/*
Conn is the internal websocket and REST transport.
Root exposes the Actor fan-out every transport embeds so Boot and desk
wiring never type-assert Live versus MockConn versus Paper.
*/
type Conn interface {
	Client() *spot.WebSocket
	Write(params json.Marshaler) error
	Post(path string, params json.Marshaler) ([]byte, error)
	Close()
	Root() *types.Actor
}

/*
API is the single Kraken transport surface for symm.
Callers subscribe, order, and listen through named methods only.
*/
type API struct {
	ctx               context.Context
	cancel            context.CancelFunc
	status            types.Status
	normalizer        *spot.Normalizer
	public            Conn
	private           Conn
	paper             Conn
	live              bool
	level3            *Level3Registry
	level3Conn        Conn
	level3Feed        *types.Actor
	publicResubscribe func() error
	wantBalances      bool
	wantExecutions    bool
}

func NewAPI(
	ctx context.Context, public, private, paper Conn,
) *API {
	ctx, cancel := context.WithCancel(ctx)

	api := &API{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.INITIALIZING,
		normalizer: spot.NewNormalizer(),
		public:     public,
		private:    private,
		paper:      paper,
		live:       viper.GetViper().GetString("trading.model") == "live",
		level3:     NewLevel3Registry(),
	}
	api.bindReconnect()

	return api
}

func (api *API) Initialize() error {
	errnie.Info("initializing API")

	if api.public == nil || api.public.Client() == nil {
		api.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cannot initialize Kraken pair normalizer without public REST",
			nil,
		))
	}

	if api.live && (api.private == nil || api.private.Client() == nil ||
		api.private.Client().REST == nil) {
		api.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cannot initialize Kraken private transport without REST",
			nil,
		))
	}

	if viper.GetString("trading.model") == "paper" && api.paper == nil {
		api.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cannot initialize paper trading without a paper transport",
			nil,
		))
	}

	if err := api.normalizer.Use(api.public.Client().REST); err != nil {
		api.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to initialize Kraken pair normalizer",
			err,
		))
	}

	api.status = types.READY
	return nil
}

/*
Status returns the API lifecycle state used by ordered system boot stages.
*/
func (api *API) Status() types.Status {
	return api.status
}

/*
Name returns the canonical WS v2 asset or pair identity for a Kraken name,
including compact paper/history forms like NEARUSD → NEAR/USD. It is the single
surface over the SDK spot.Normalizer.
*/
func (api *API) Name(symbol string) string {
	return api.normalizer.Name(symbol)
}

func (api *API) Close() {
	api.public.Close()
	api.private.Close()

	if api.paper != nil {
		api.paper.Close()
	}

	if api.level3 != nil {
		api.level3.Close()
	}
}

/*
account is the private venue socket in live mode and the paper transport otherwise.
*/
func (api *API) account() Conn {
	if api.live {
		return api.private
	}

	return api.paper
}

/*
Account exposes the account transport Conn for Boot Actor wiring.
*/
func (api *API) Account() Conn {
	return api.account()
}

func (api *API) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	response, err := api.private.Post(
		TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"websocket.API.TradeVolume[%s]: failed to get trade volume: %s",
				response,
				err.Error(),
			),
			err,
		))
	}

	tradeVolume := kraken.NewTradeVolume(response)

	if tradeVolume == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"Kraken returned an invalid trade volume response",
			nil,
		))
	}

	fees := make(map[string]kraken.TradeVolumeFee, len(tradeVolume.Fees))
	requested := make(map[string]string, len(symbols))

	for _, symbol := range symbols {
		requested[strings.ReplaceAll(symbol, "/", "")] = symbol
	}

	for symbol, fee := range tradeVolume.Fees {
		name := api.normalizer.Name(symbol)

		if requestedName, ok := requested[symbol]; ok {
			name = requestedName
		}

		fees[name] = fee
	}

	tradeVolume.Fees = fees
	fees = make(map[string]kraken.TradeVolumeFee, len(tradeVolume.FeesMaker))

	for symbol, fee := range tradeVolume.FeesMaker {
		name := api.normalizer.Name(symbol)

		if requestedName, ok := requested[symbol]; ok {
			name = requestedName
		}

		fees[name] = fee
	}

	tradeVolume.FeesMaker = fees

	return tradeVolume, nil
}

func (api *API) TradesHistory() (*kraken.TradesHistory, error) {
	if !api.live {
		ledger, ok := api.paper.(interface {
			TradesHistory() (*kraken.TradesHistory, error)
		})

		if !ok {
			return nil, errnie.Error(errnie.Err(
				errnie.NotImplemented,
				"paper transport does not provide trade history",
				nil,
			))
		}

		return ledger.TradesHistory()
	}

	trades, err := api.fetchLiveTradesHistory()

	if err != nil {
		return nil, err
	}

	return &kraken.TradesHistory{
		Result: kraken.TradesHistoryResult{
			Trades: trades,
		},
	}, nil
}

/*
Live reports whether this API speaks to the venue rather than the paper
Simulator. Tick-path callers use it to keep live REST off the cut goroutine.
*/
func (api *API) Live() bool {
	return api != nil && api.live
}

/*
OpenOrders returns currently resting orders keyed by their venue order id. Live
boots read the private REST ledger; paper fills synchronously through Place so
it never leaves an order resting and answers with an empty set.
*/
func (api *API) OpenOrders() (map[string]spot.Order, error) {
	if !api.Live() {
		ledger, ok := api.paper.(interface {
			OpenOrders() (map[string]spot.Order, error)
		})

		if !ok {
			return nil, errnie.Error(errnie.Err(
				errnie.NotImplemented,
				"paper transport does not provide open orders",
				nil,
			))
		}

		return ledger.OpenOrders()
	}

	return api.fetchLiveOpenOrders()
}

/*
fetchLiveOpenOrders reads the private REST open-order ledger so boot reconcile
can match durable pending intents against orders the venue still holds.
*/
func (api *API) fetchLiveOpenOrders() (map[string]spot.Order, error) {
	if api.private == nil || api.private.Client() == nil ||
		api.private.Client().REST == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"Kraken private REST client is unavailable",
			nil,
		))
	}

	response, err := api.private.Client().REST.OpenOrders(&spot.OpenOrdersRequest{
		Trades: true,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get open orders",
			err,
		))
	}

	return response.Result.Open, nil
}

/*
fetchLiveTradesHistory walks Kraken trade-history pages until the reported
count is covered or a short page is returned. Boot-time reconciliation needs
the full ledger, not just the first REST page.
*/
func (api *API) fetchLiveTradesHistory() (map[string]spot.Trade, error) {
	if api.private == nil || api.private.Client() == nil ||
		api.private.Client().REST == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"Kraken private REST client is unavailable",
			nil,
		))
	}

	merged := make(map[string]spot.Trade)
	offset := 0

	for {
		response, err := api.private.Client().REST.TradesHistory(&spot.TradesHistoryRequest{
			Type:             "all",
			Trades:           true,
			ConsolidateTaker: true,
			Ofs:              offset,
		})

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get trades history",
				err,
			))
		}

		page := response.Result.Trades

		if len(page) == 0 {
			break
		}

		previousCount := len(merged)
		maps.Copy(merged, page)

		if len(merged) == previousCount {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"Kraken trades history pagination made no progress",
				nil,
			))
		}

		totalCount, countErr := strconv.ParseInt(response.Result.Count.String(), 10, 64)

		if countErr == nil && int64(len(merged)) >= totalCount {
			break
		}

		if len(page) < 50 {
			break
		}

		offset += len(page)
	}

	return merged, nil
}

func (api *API) SubscribeInstruments() error {
	// Conn.Write so mock producers and live sockets share one subscribe path.
	return errnie.Error(api.public.Write(kraken.NewInstrumentSubscription()))
}

func (api *API) SubscribeTicker(pairs []string) error {
	return errnie.Error(api.public.Write(kraken.NewTickerSubscription(pairs)))
}

func (api *API) SubscribeTrade(pairs []string) error {
	return errnie.Error(api.public.Write(kraken.NewTradeSubscription(pairs)))
}

/*
Books yields the SDK BookManager owned by each Level3 transport. Callers that
read book contents must use PeekBook instead — the managers are live and
mutated under a write lease during websocket updates.
*/
func (api *API) Books() iter.Seq[*spot.BookManager] {
	return api.level3.Books()
}

/*
InjectLevel3 binds a Conn's Actor level3 root to the production Level3 book processor.
*/
func (api *API) InjectLevel3(market *types.Actor, conn Conn, symbols []string) {
	if api == nil || conn == nil || market == nil {
		return
	}

	live := newLevel3Consumer(
		api.ctx,
		symbols,
		viper.GetInt("market.l3_depth"),
	)
	api.level3Conn = conn
	api.level3.Attach("injected-level3", live)

	feed := types.NewActor(api.ctx, map[string]types.Handler{
		"level3": {
			Topic: "level3",
			Fn: func(message any) any {
				err := live.ApplyLevel3(message.([]byte))

				if reporter, ok := conn.(interface{ Report(error) }); ok && err != nil {
					reporter.Report(err)
				}

				errnie.Error(err)

				return nil
			},
		},
	})
	feed.Initialize(types.Topic{Name: "level3", Actor: market})
	api.level3Feed = feed
}

/*
PeekBook invokes fn under the Level3 read lease for symbol so Side.Levels and
order queues cannot be mutated mid-read by updateLevel3.
*/
func (api *API) PeekBook(symbol string, fn func(*book.Book)) bool {
	if api == nil {
		return false
	}

	return api.level3.PeekBook(symbol, fn)
}

func (api *API) SubscribeBook(pairs []string) error {
	return errnie.Error(api.public.Write(kraken.NewBookSubscription(pairs)))
}

/*
SubscribeLevel3 assigns each symbol batch its own authenticated book transport.
The transport subscribes after authentication and repeats that same request
after reconnect, so this method must not send a second competing subscription.
*/
func (api *API) SubscribeLevel3(pairs []string) error {
	if api.level3Conn != nil {
		return errnie.Error(api.level3Conn.Write(
			kraken.NewLevel3Subscription(
				pairs,
				viper.GetInt("market.l3_depth"),
			),
		))
	}

	return errnie.Error(api.level3.SubscribeAll(api.ctx, pairs))
}

func (api *API) SubscribeBalance() error {
	api.wantBalances = true

	token := ""

	if api.live {
		token = api.private.Client().Token
	}

	return errnie.Error(api.account().Write(kraken.NewBalanceSubscription(token)))
}

func (api *API) SubscribeExecutions() error {
	api.wantExecutions = true

	token := ""

	if api.live {
		token = api.private.Client().Token
	}

	return errnie.Error(api.account().Write(kraken.NewExecutionSubscription(token)))
}

func (api *API) AddOrder(order *kraken.MarketOrder) error {
	return api.account().Write(order)
}

/*
bindReconnect hooks public and private Lives so a new socket session re-subscribes.
Listeners on OnReceived stay registered; only the Sub* RPCs must be repeated.
*/
func (api *API) bindReconnect() {
	if api == nil {
		return
	}

	if live, ok := api.public.(*Live); ok {
		live.ready = api.resubscribePublic
	}

	if live, ok := api.private.(*Live); ok {
		live.ready = api.resubscribePrivate
	}
}

/*
SetPublicResubscribe registers the market-universe resubscribe (Instrument).
*/
func (api *API) SetPublicResubscribe(fn func() error) {
	if api == nil {
		return
	}

	api.publicResubscribe = fn
}

func (api *API) resubscribePublic() error {
	if api.publicResubscribe == nil {
		return nil
	}

	return api.publicResubscribe()
}

func (api *API) resubscribePrivate() error {
	if !api.live {
		return nil
	}

	if api.wantBalances {
		if err := api.SubscribeBalance(); err != nil {
			return err
		}
	}

	if api.wantExecutions {
		if err := api.SubscribeExecutions(); err != nil {
			return err
		}
	}

	return nil
}

func (api *API) subscribeBatchSize() int {
	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize < 1 {
		return 1
	}

	return batchSize
}

/*
ResubscribeMarket re-issues instrument + trade/book/ticker for the cached
symbol universe after reconnect.
*/
func (api *API) ResubscribeMarket(symbols []string) error {
	if err := api.SubscribeInstruments(); err != nil {
		return err
	}

	if len(symbols) == 0 {
		return nil
	}

	batchSize := api.subscribeBatchSize()
	subscribers := []func([]string) error{
		api.SubscribeTrade,
		api.SubscribeBook,
		api.SubscribeTicker,
	}

	for batch := range slices.Chunk(symbols, batchSize) {
		for _, subscribe := range subscribers {
			if err := subscribe(batch); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
ResyncBook forces a public book resnapshot after a local checksum failure.
*/
func (api *API) ResyncBook(pairs []string) error {
	if api == nil || api.public == nil || len(pairs) == 0 {
		return nil
	}

	if err := api.public.Write(kraken.NewBookUnsubscription(pairs)); err != nil {
		return err
	}

	return api.SubscribeBook(pairs)
}
