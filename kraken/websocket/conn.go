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
	TradeVolumeEndpoint           = "/0/private/TradeVolume"
	level3MaxSymbolsPerConnection = 200
)

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
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	normalizer *spot.Normalizer
	public     Conn
	private    Conn
	paper      *Paper
	live       bool
	bookConns  *sync.Map
}

func NewAPI(
	ctx context.Context, public, private Conn, paper *Paper,
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
		bookConns:  &sync.Map{},
	}

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

func (api *API) Close() {
	api.public.Close()
	api.private.Close()
}

/*
On registers a raw channel consumer on the transport that owns the channel.
Level3 is deliberately absent because the SDK BookManager consumes those frames
and exposes its books through Books.
*/
func (api *API) On(channel string, action func([]byte)) {
	switch channel {
	case "balances", "executions", "add_order":
		if api.live {
			api.private.On(channel, action)
			return
		}

		api.paper.On(channel, action)
	default:
		api.public.On(channel, action)
	}
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
		return api.paper.TradesHistory()
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
	return errnie.Error(api.public.Client().SubInstruments())
}

func (api *API) SubscribeTicker(pairs []string) error {
	return errnie.Error(api.public.Client().SubTicker(pairs))
}

func (api *API) SubscribeTrade(pairs []string) error {
	return errnie.Error(api.public.Client().SubTrades(
		pairs,
		map[string]any{
			"params": map[string]any{
				"snapshot": true,
			},
		},
	))
}

/*
Books yields the SDK BookManager owned by each Level3 transport. Callers that
read book contents must use PeekBook instead — the managers are live and
mutated under a write lease during websocket updates.
*/
func (api *API) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		api.bookConns.Range(func(key, value any) bool {
			live := value.(*Live)
			keepGoing := yield(live.books)

			return keepGoing
		})
	}
}

/*
AttachLevel3 registers a Level3 Live transport so PeekBook and Session harness
tests can feed SDK-managed books without opening a real L3 websocket.
*/
func (api *API) AttachLevel3(live *Live) {
	if api == nil || live == nil || live.books == nil {
		return
	}

	api.bookConns.Store("session-level3", live)
}

/*
PeekBook invokes fn under the Level3 read lease for symbol so Side.Levels and
order queues cannot be mutated mid-read by updateLevel3.
*/
func (api *API) PeekBook(symbol string, fn func(*book.Book)) bool {
	if api == nil || fn == nil || symbol == "" {
		return false
	}

	found := false

	api.bookConns.Range(func(_, value any) bool {
		live := value.(*Live)

		if !live.peekBook(symbol, fn) {
			return true
		}

		found = true

		return false
	})

	return found
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
	batchSize, err := api.level3BatchSize()

	if err != nil {
		return err
	}

	for batch := range slices.Chunk(pairs, batchSize) {
		key := strings.Join(batch, "|")

		if _, loaded := api.bookConns.Load(key); loaded {
			continue
		}

		live := New(api.ctx, nil, true, Level3WebSocketURL)
		live.symbols = batch

		if err := live.Initialize(); err != nil {
			live.Close()
			return errnie.Error(err)
		}

		_, loaded := api.bookConns.LoadOrStore(key, live)

		if loaded {
			live.Close()
		}
	}

	return nil
}

/*
level3BatchSize derives the number of symbols that fit in one Kraken L3
subscription request from the configured client-tier budget and book depth.
*/
func (api *API) level3BatchSize() (int, error) {
	depth := viper.GetInt("market.l3_depth")
	rateLimit := viper.GetInt("market.l3_rate_limit")
	rateCost := map[int]int{
		10:   5,
		100:  25,
		1000: 100,
	}[depth]

	if rateCost == 0 || rateLimit < rateCost {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: L3 depth and rate limit cannot admit one symbol",
			nil,
		))
	}

	return min(rateLimit/rateCost, level3MaxSymbolsPerConnection), nil
}

func (api *API) SubscribeBalance() error {
	if api.live {
		return errnie.Error(api.private.Client().SubBalances())
	}

	return api.paper.SubBalances()
}

func (api *API) SubscribeExecutions() error {
	if api.live {
		return errnie.Error(api.private.Client().SubExecutions(map[string]any{
			"params": map[string]any{
				"snap_orders": false,
				"snap_trades": false,
			},
		}))
	}

	return api.paper.SubExecutions()
}

func (api *API) AddOrder(order *kraken.MarketOrder) error {
	if api.live {
		return api.private.Write(order)
	}

	return api.paper.AddOrder(order)
}
