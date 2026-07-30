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
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
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
Root exposes the Actor fan-out every transport embeds so Boot and desk
wiring never type-assert Live versus MockConn versus Paper.
*/
type Conn interface {
	Client() *spot.WebSocket
	Books() *spot.BookManager
	PeekBook(symbol string, fn func(*book.Book)) bool
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
	level3Mu          sync.RWMutex
	level3Conns       map[string]Conn
	level3Index       map[string]Conn
	level3Conn        Conn
	publicResubscribe func() error
	wantBalances      bool
	wantExecutions    bool
}

func NewAPI(
	ctx context.Context, public, private, paper Conn,
) *API {
	ctx, cancel := context.WithCancel(ctx)

	api := &API{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		normalizer:  spot.NewNormalizer(),
		public:      public,
		private:     private,
		paper:       paper,
		live:        viper.GetViper().GetString("trading.model") == "live",
		level3Conns: make(map[string]Conn),
		level3Index: make(map[string]Conn),
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

	api.level3Mu.RLock()
	conns := slices.Collect(maps.Values(api.level3Conns))
	api.level3Mu.RUnlock()

	for _, conn := range conns {
		if conn == nil || conn == api.public || conn == api.private || conn == api.paper {
			continue
		}

		conn.Close()
	}
}

func (api *API) attachLevel3(key string, conn Conn, symbols []string) {
	if api == nil || key == "" || conn == nil {
		return
	}

	api.level3Mu.Lock()
	defer api.level3Mu.Unlock()

	api.level3Conns[key] = conn

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		api.level3Index[symbol] = conn
	}
}

func (api *API) level3BatchSize() (int, error) {
	depth := viper.GetInt("market.l3_depth")
	rateLimit := viper.GetInt("market.l3_rate_limit")
	rateCost := map[int]int{
		10:   5,
		100:  25,
		1000: 100,
	}[depth]

	if rateCost == 0 || rateLimit < rateCost {
		return 0, errnie.Err(
			errnie.Validation,
			"websocket: L3 depth and rate limit cannot admit one symbol",
			nil,
		)
	}

	return min(rateLimit/rateCost, 200), nil
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
AccountBalances loads the shared wallet snapshot for trading and UI reconciliation.
It uses paper balances in paper mode and the private REST balances endpoint in
live mode so the strategy and terminal share one authoritative wallet source.
*/
func (api *API) AccountBalances() (*kraken.Balance, error) {
	if !api.live {
		ledger, ok := api.paper.(interface {
			BalanceSnapshot() (*kraken.Balance, error)
		})

		if !ok {
			return nil, errnie.Error(errnie.Err(
				errnie.NotImplemented,
				"paper transport does not provide account balances",
				nil,
			))
		}

		return ledger.BalanceSnapshot()
	}

	if api.private == nil || api.private.Client() == nil || api.private.Client().REST == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"Kraken private REST client is unavailable",
			nil,
		))
	}

	response, err := api.private.Client().REST.Balances()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get account balances",
			err,
		))
	}

	balance := &kraken.Balance{
		Channel: "balances",
		Type:    "snapshot",
		Data:    make([]kraken.BalanceData, 0, len(response.Result)),
	}

	for asset, total := range response.Result {
		if total == nil {
			continue
		}

		name := api.Name(asset)

		if name == "" {
			name = asset
		}

		balance.Data = append(balance.Data, kraken.BalanceData{
			Asset:      name,
			AssetClass: "currency",
			Balance:    total.Copy(),
			Wallets: []kraken.Wallet{{
				Type:    "spot",
				ID:      "main",
				Balance: total.Copy(),
			}},
		})
	}

	return balance, nil
}

/*
TradeBalance loads current liquidation-like exposure from the paper surface or
Kraken `/0/private/TradeBalance` in live mode.
*/
func (api *API) TradeBalance(asset string) (*kraken.TradeBalanceResult, error) {
	if !api.live {
		ledger, ok := api.paper.(interface {
			TradeBalance(asset string) (*kraken.TradeBalanceResult, error)
		})

		if !ok {
			return nil, errnie.Error(errnie.Err(
				errnie.NotImplemented,
				"paper transport does not provide trade balance",
				nil,
			))
		}

		return ledger.TradeBalance(asset)
	}

	if asset == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trade-balance request requires an asset",
			nil,
		))
	}

	normalized := strings.TrimSpace(api.Name(asset))
	if normalized == "" {
		normalized = strings.TrimSpace(asset)
	}

	if normalized == "USD" {
		normalized = "ZUSD"
	}

	if api.private == nil || api.private.Client() == nil || api.private.Client().REST == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"Kraken private REST client is unavailable",
			nil,
		))
	}

	response, err := api.private.Post(
		TradeBalanceEndpoint,
		kraken.NewTradeBalanceRequest(normalized),
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade balance",
			err,
		))
	}

	balance := kraken.NewTradeBalance(response)

	if balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"Kraken returned an invalid trade balance response",
			nil,
		))
	}

	return balance, nil
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
	return func(yield func(*spot.BookManager) bool) {
		seen := map[*spot.BookManager]struct{}{}

		api.level3Mu.RLock()
		conns := slices.Collect(maps.Values(api.level3Conns))
		api.level3Mu.RUnlock()

		for _, conn := range conns {
			if conn == nil || conn.Books() == nil {
				continue
			}

			books := conn.Books()

			if _, ok := seen[books]; ok {
				continue
			}

			seen[books] = struct{}{}

			if !yield(books) {
				return
			}
		}
	}
}

/*
Book returns the primary SDK BookManager for market book access.
*/
func (api *API) Book() *spot.BookManager {
	if api == nil || api.public == nil {
		return nil
	}

	return api.public.Books()
}

/*
BookManager returns the primary SDK BookManager explicitly for callers that want
direct access to venue-managed books instead of actor-delivered book frames.
*/
func (api *API) BookManager() *spot.BookManager {
	return api.Book()
}

/*
InjectLevel3 registers an externally managed Level3 transport and indexes its
symbols for direct BookManager access.
*/
func (api *API) InjectLevel3(market *types.Actor, conn Conn, symbols []string) {
	if api == nil || conn == nil || market == nil {
		return
	}

	api.level3Conn = conn
	api.attachLevel3("injected-level3", conn, symbols)
}

/*
PeekBook invokes fn under the Level3 read lease for symbol so Side.Levels and
order queues cannot be mutated mid-read by updateLevel3.
*/
func (api *API) PeekBook(symbol string, fn func(*book.Book)) bool {
	if api == nil || symbol == "" || fn == nil {
		return false
	}

	api.level3Mu.RLock()
	conn := api.level3Index[symbol]
	api.level3Mu.RUnlock()

	if conn == nil {
		return false
	}

	return conn.PeekBook(symbol, fn)
}

func (api *API) SubscribeBook(pairs []string) error {
	if live, ok := api.public.(*Live); ok {
		live.symbols = mergeSymbols(live.symbols, pairs)
	}

	return errnie.Error(api.public.Write(kraken.NewBookSubscription(pairs)))
}

/*
SubscribeLevel3 assigns each symbol batch its own authenticated book transport.
The transport subscribes after authentication and repeats that same request
after reconnect, so this method must not send a second competing subscription.
*/
func (api *API) SubscribeLevel3(pairs []string) error {
	if api.level3Conn != nil {
		api.attachLevel3("injected-level3", api.level3Conn, pairs)
		return errnie.Error(api.level3Conn.Write(
			kraken.NewLevel3Subscription(
				pairs,
				viper.GetInt("market.l3_depth"),
			),
		))
	}

	batchSize, err := api.level3BatchSize()

	if err != nil {
		return errnie.Error(err)
	}

	for batch := range slices.Chunk(pairs, batchSize) {
		key := strings.Join(batch, "|")

		api.level3Mu.RLock()
		if _, ok := api.level3Conns[key]; ok {
			api.level3Mu.RUnlock()
			continue
		}
		api.level3Mu.RUnlock()

		live := New(api.ctx, nil, true, Level3WebSocketURL)
		live.symbols = append([]string(nil), batch...)

		if err := live.Initialize(); err != nil {
			live.Close()
			return errnie.Error(err)
		}

		api.attachLevel3(key, live, batch)
	}

	return nil
}

func mergeSymbols(current []string, next []string) []string {
	if len(next) == 0 {
		return current
	}

	seen := make(map[string]struct{}, len(current)+len(next))
	merged := make([]string, 0, len(current)+len(next))

	for _, symbol := range current {
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		merged = append(merged, symbol)
	}

	for _, symbol := range next {
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		merged = append(merged, symbol)
	}

	return merged
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
