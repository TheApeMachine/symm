package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
Instrument owns venue pair facts and the subscription universe. Decimal values
are shared as immutable SDK values; arithmetic returns a new Decimal.
*/
type Instrument struct {
	*runtime.System
	public  *network.WebsocketClient
	Level3  *sync.Map
	cache   *sync.Map
	quote   string
	symbols []string

	// products maps a spot symbol to the venue's tradeable perpetual product
	// identifier, and symbolsByProduct maps every listed product back to its
	// spot symbol. Both are built once from the venue's instrument list, which
	// is the only authority on which contracts exist and what they are called.
	products         map[string]string
	symbolsByProduct map[string]string

	// token caches the Level3 websockets token and when it was issued, so
	// every batch shares one fetch and a late reconnect gets a fresh token.
	tokenMu sync.Mutex
	token   string
	tokenAt time.Time

	// fetchToken asks the venue for a websockets token; nil is the
	// authenticated process REST client. Tests replace it.
	fetchToken func() (string, error)

	// level3Stale receives a batch's symbols when its Level3 socket drops,
	// so the book stops serving state that no longer tracks the venue.
	level3Stale func([]string)

	// paceMu serializes Level3 subscribe frames across every socket and
	// reconnect hook; pacedAt is when the last frame went out. The venue's
	// snapshot rate counter is per client, not per socket.
	paceMu  sync.Mutex
	pacedAt time.Time

	// level3Reader starts reading a Level3 socket. Subscribe calls it as soon
	// as the socket opens, before any subscribe frame: the venue rejects
	// snapshot requests on a socket whose earlier snapshots sit unread.
	level3Reader func(*network.WebsocketClient)
}

/*
SetLevel3Reader connects each Level3 socket to its ingress reader. It is
required before Subscribe.
*/
func (instrument *Instrument) SetLevel3Reader(read func(*network.WebsocketClient)) {
	instrument.level3Reader = read
}

/*
SetLevel3Stale connects Level3 disconnects to the book (Book.Stale). It is
required before Subscribe: without it a dropped Level3 socket would leave the
book serving frozen depth until the resubscribe snapshot arrives.
*/
func (instrument *Instrument) SetLevel3Stale(stale func([]string)) {
	instrument.level3Stale = stale
}

/*
NewInstrument creates the market-instrument registry
used by subscriptions and order validation.
*/
func NewInstrument(
	public *network.WebsocketClient,
) *Instrument {
	instrument := &Instrument{
		public:           public,
		Level3:           &sync.Map{},
		cache:            &sync.Map{},
		symbols:          []string{},
		quote:            system.Cfg.Market.QuoteCurrency,
		products:         make(map[string]string),
		symbolsByProduct: make(map[string]string),
	}

	instrument.System = runtime.NewSystem(
		public.Context(), "instrument", instrument,
	)

	instrument.Transition(runtime.BUSY)

	message, err := kraken.NewInstrumentSubscription().MarshalJSON()

	if err != nil {
		instrument.Error(errnie.Err(
			errnie.IO,
			"[instrument] failed to marshal subscription request",
			err,
		))

		return instrument
	}

	if err := public.Write(message); err != nil {
		instrument.Error(errnie.Err(
			errnie.IO,
			"[instrument] snapshot subscription failed",
			err,
		))

		return instrument
	}

	var snapshot *kraken.Instrument

	for {
		buf, err := public.Read()

		if err != nil {
			instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] snapshot unavailable",
				err,
			))

			return instrument
		}

		node, _ := sonic.Get(buf, "channel")
		var str string

		if str, err = node.String(); err != nil || str != "instrument" {
			continue
		}

		snapshot = kraken.NewInstrument(buf)
		errnie.Info("[instrument] received valid instrument snapshot")
		break
	}

	for _, pair := range snapshot.Data.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" || slices.Contains(
			system.Cfg.Market.Instrument.Excluded, pair.Base,
		) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}

	instrument.Transition(runtime.READY)
	return instrument
}

func (instrument *Instrument) Cache(pairs []kraken.InstrumentPair) {
	for _, pair := range pairs {
		if pair.Quote != instrument.quote ||
			pair.Status != "online" ||
			slices.Contains(system.Cfg.Market.Instrument.Excluded, pair.Base) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}
}

/*
Pairs returns the cached instrument values.
*/
func (instrument *Instrument) Pairs() []kraken.InstrumentPair {
	pairs := make([]kraken.InstrumentPair, 0)

	instrument.cache.Range(func(key, value any) bool {
		pair, ok := value.(kraken.InstrumentPair)

		if !ok {
			return true
		}

		pairs = append(pairs, pair)
		return true
	})

	return pairs
}

/*
Has reports whether the instrument is in the cached universe.
*/
func (instrument *Instrument) Has(symbol string) bool {
	if instrument == nil || instrument.cache == nil {
		return false
	}

	_, ok := instrument.cache.Load(symbol)
	return ok
}

/*
Pair returns the cached instrument value for the symbol.
*/
func (instrument *Instrument) Pair(symbol string) kraken.InstrumentPair {
	value, ok := instrument.cache.Load(symbol)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader: instrument pair not found for "+symbol,
			nil,
		))

		return kraken.InstrumentPair{}
	}

	return value.(kraken.InstrumentPair)
}

/*
LoadFuturesProducts maps each online spot symbol onto one tradeable Futures
perpetual product_id. Without this map, futures subscriptions using spot names
are silent no-ops and derivatives never see index/mark/OI.
*/
func (instrument *Instrument) LoadFuturesProducts(ctx context.Context) error {
	if instrument == nil {
		return nil
	}

	listed, err := kraken.FetchFuturesInstruments(ctx)
	if err != nil {
		return err
	}

	bySpot := make(map[string][]kraken.FuturesInstrument)
	for _, item := range listed {
		spot := kraken.SpotSymbolForFutures(item)
		if spot == "" || !instrument.Has(spot) {
			continue
		}
		bySpot[spot] = append(bySpot[spot], item)
	}

	instrument.products = make(map[string]string, len(bySpot))
	instrument.symbolsByProduct = make(map[string]string, len(bySpot))

	for spot, candidates := range bySpot {
		product := kraken.PreferPerpetual(candidates)
		if product == "" {
			continue
		}
		instrument.products[spot] = product
		instrument.symbolsByProduct[product] = spot
	}

	errnie.Info(fmt.Sprintf(
		"[instrument] mapped %d spot symbols onto futures perpetuals",
		len(instrument.products),
	))

	return nil
}

/*
FuturesProductIDs returns the subscribed perpetual product identifiers.
*/
func (instrument *Instrument) FuturesProductIDs() []string {
	if instrument == nil || len(instrument.products) == 0 {
		return nil
	}

	ids := make([]string, 0, len(instrument.products))
	for _, product := range instrument.products {
		ids = append(ids, product)
	}
	slices.Sort(ids)
	return ids
}

/*
SpotForProduct returns the spot WS symbol for a Futures product_id, or "".
*/
func (instrument *Instrument) SpotForProduct(productID string) string {
	if instrument == nil || productID == "" {
		return ""
	}
	return instrument.symbolsByProduct[productID]
}

/*
level3TokenReuse bounds how long one websockets token is presented for a new
Level3 subscription. Kraken honours a token for establishing a subscription
only within 15 minutes of issue; an established connection keeps it, but a
reconnect after that window must present a fresh token or the venue rejects
the subscription and the book goes dark.
*/
const level3TokenReuse = 10 * time.Minute

/*
level3Token returns a websockets token young enough to subscribe with,
fetching a fresh one when the cached token is missing or past
level3TokenReuse. A transient fetch failure (an invalid nonce after another
process signed with the same key, a rate limit, a network error) is retried
with backoff until it succeeds or the instrument closes; only a permanent
credential error is returned. Other batches wait on tokenMu and then share the
fetched token.
*/
func (instrument *Instrument) level3Token() (string, error) {
	instrument.tokenMu.Lock()
	defer instrument.tokenMu.Unlock()

	if instrument.token != "" && time.Since(instrument.tokenAt) < level3TokenReuse {
		return instrument.token, nil
	}

	fetch := instrument.fetchToken

	if fetch == nil {
		fetch = restLevel3Token
	}

	token, err := kraken.RetryToken(instrument.System.Context(), "level3", fetch)

	if err != nil {
		return "", errnie.Err(
			errnie.IO,
			"[instrument] level3 websocket token unavailable",
			err,
		)
	}

	instrument.token = token
	instrument.tokenAt = time.Now()

	return instrument.token, nil
}

/*
restLevel3Token fetches one websockets token through the authenticated
process REST client.
*/
func restLevel3Token() (string, error) {
	restClient, err := kraken.NewAuthenticatedREST()

	if err != nil {
		return "", err
	}

	tokenRes, err := restClient.GetWebSocketsToken()

	if err != nil || tokenRes == nil {
		return "", errnie.Err(
			errnie.IO,
			"[instrument] level3 websocket token request failed",
			err,
		)
	}

	return tokenRes.Result.Token, nil
}

var level3SnapshotCost = map[int]int{10: 5, 100: 25, 1000: 100}

/*
level3RateWindow returns the pause between Level3 subscribe frames. The venue's
snapshot rate counter budget is market.l3_rate_limit (default 200), and its
sustained drain rate is 50 counter units per second. A frame carrying cost
size*cost requires at least (size*cost)/50 seconds to drain, with a minimum
pause of 2 seconds to absorb venue clock jitter and discrete sampling.
*/
func level3RateWindow() time.Duration {
	if window := viper.GetDuration("market.l3_rate_window"); window > 0 {
		return window
	}

	size, err := level3FrameSize()

	if err != nil {
		return 2 * time.Second
	}

	return time.Duration(max(
		((size*level3SnapshotCost[level3Depth()])+49)/50, 2,
	)) * time.Second
}

/*
level3Depth is the depth subscribed and maintained: market.l3_depth, with the
venue's default of 10 when unset (the same default the book enforces).
*/
func level3Depth() int {
	if depth := viper.GetInt("market.l3_depth"); depth > 0 {
		return depth
	}

	return 10
}

/*
level3FrameSize is how many symbols one subscribe frame may carry so its
snapshots fit one rate window: market.l3_rate_limit (the account tier's
counter budget) divided by the per-symbol cost at the subscribed depth.
*/
func level3FrameSize() (int, error) {
	depth := level3Depth()
	cost, ok := level3SnapshotCost[depth]

	if !ok {
		return 0, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[instrument] market.l3_depth %d is not a Kraken level3 depth (10, 100, 1000)",
				depth,
			),
			nil,
		)
	}

	size := viper.GetInt("market.l3_rate_limit") / cost

	if size < 1 {
		return 0, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[instrument] market.l3_rate_limit must cover one symbol at depth %d (cost %d)",
				depth, cost,
			),
			nil,
		)
	}

	return size, nil
}

/*
subscribeLevel3 writes the Level3 subscription for one batch with a currently
valid token, split into frames that each fit the venue's snapshot rate
budget, one frame per rate window across all sockets.
*/
func (instrument *Instrument) subscribeLevel3(write func([]byte) error, batch []string) error {
	size, err := level3FrameSize()

	if err != nil {
		return err
	}

	token, err := instrument.level3Token()

	if err != nil {
		return err
	}

	for frame := range slices.Chunk(batch, size) {
		msg, err := sonic.Marshal(kraken.NewLevel3Subscription(frame, token, level3Depth()))

		if err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe marshal failed",
				err,
			)
		}

		if err := instrument.paceLevel3(func() error { return write(msg) }); err != nil {
			return err
		}
	}

	return nil
}

/*
paceLevel3 runs write once a full rate window has passed since the previous
Level3 subscribe frame, or returns when the instrument closes.
*/
func (instrument *Instrument) paceLevel3(write func() error) error {
	instrument.paceMu.Lock()
	defer instrument.paceMu.Unlock()

	if wait := time.Until(instrument.pacedAt.Add(level3RateWindow())); wait > 0 {
		select {
		case <-instrument.System.Context().Done():
			return instrument.System.Context().Err()
		case <-time.After(wait):
		}
	}

	err := write()
	instrument.pacedAt = time.Now()

	return err
}

/*
Subscribe issues paced market-data batches for the online quote universe. The
public socket carries trades only, the sole frame type the workspace pipeline
consumes. Level3 runs on its own authenticated socket per batch and feeds the
BookManager only. A failed resubscribe after a reconnect is logged and
returned to the socket, which drops the connection and redials with backoff
until a resubscribe succeeds; the batch's books stay stale meanwhile. It never
halts the instrument.
*/
func (instrument *Instrument) Subscribe() error {
	errnie.Info("[instrument] subscribing to symbol pairs")

	if instrument.level3Stale == nil {
		return instrument.Error(errnie.Err(
			errnie.Validation,
			"[instrument] level3 stale sink is required before subscribe",
			nil,
		))
	}

	stale := instrument.level3Stale

	if instrument.level3Reader == nil {
		return instrument.Error(errnie.Err(
			errnie.Validation,
			"[instrument] level3 reader is required before subscribe",
			nil,
		))
	}
	var tradeSubs [][]byte

	for chunk := range slices.Chunk(
		instrument.symbols, system.Cfg.Market.Subscribe.Batch,
	) {
		batch := slices.Clone(chunk)
		batchKey := strings.Join(batch, "|")

		tradeMsg, err := sonic.Marshal(kraken.NewTradeSubscription(batch))

		if err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO, "[instrument] trade subscribe marshal failed", err,
			))
		}

		tradeSubs = append(tradeSubs, tradeMsg)

		if err := instrument.public.Write(tradeMsg); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] required trade subscription failed",
				err,
			))
		}

		l3Client := network.NewWebsocketClient(instrument.System.Context())

		if err := l3Client.Open(system.Cfg.WebSocket.Endpoints.Level3); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 open failed for "+batchKey,
				err,
			))
		}

		// Read before subscribing: unread snapshots make the venue reject
		// later snapshot requests (measured: 120 of 621 rejected with readers
		// started after all subscribes, 0 of 621 with readers running).
		instrument.level3Reader(l3Client)

		if err := instrument.subscribeLevel3(l3Client.Write, batch); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe failed for "+batchKey,
				err,
			))
		}

		// The book's view of this batch is only valid while the socket is
		// live; a drop marks it stale until the resubscribe snapshot lands.
		l3Client.OnDisconnect(func() {
			errnie.Warn("[instrument] level3 disconnected, book stale for " + batchKey)
			stale(batch)
		})

		// Open runs any registered reconnect hook, so the hook is registered
		// only after the initial subscribe was written once and checked.
		l3Client.OnReconnect(func() error {
			errnie.Info("[instrument] level3 resubscribe after reconnect for " + batchKey)

			if err := instrument.subscribeLevel3(l3Client.Write, batch); err != nil {
				// The books of this batch were marked stale on disconnect
				// and stay so until a resubscribe snapshot lands.
				stale(batch)

				return errnie.Error(errnie.Err(
					errnie.IO,
					"[instrument] level3 resubscribe failed, books stale, redialing for "+batchKey,
					err,
				))
			}

			return nil
		})

		errnie.Info("[instrument] subscribed to level3 for " + batchKey)
		instrument.Level3.Store(batchKey, l3Client)

		time.Sleep(100 * time.Millisecond)
	}

	instrument.public.OnReconnect(func() error {
		errnie.Info("[instrument] public trade resubscribe after reconnect")

		for _, msg := range tradeSubs {
			if err := instrument.public.Write(msg); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[instrument] trade resubscribe failed, redialing",
					err,
				))
			}
		}

		return nil
	})

	instrument.Transition(runtime.READY)
	return nil
}

/*
Unsubscribe withdraws the trade streams for the online quote universe. It is
the mirror of Subscribe's public side and walks the same batched seam, so a
deliberate teardown leaves the venue with no streams pointed at sockets that
are about to close.
*/
func (instrument *Instrument) Unsubscribe() error {
	errnie.Info("[instrument] unsubscribing from symbol pairs")

	for batch := range slices.Chunk(
		instrument.symbols, system.Cfg.Market.Subscribe.Batch,
	) {
		msg, err := sonic.Marshal(kraken.NewTradeUnsubscription(batch))

		if err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO, "[instrument] trade unsubscribe marshal failed", err,
			))
		}

		if err := instrument.public.Write(msg); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] required trade unsubscription failed",
				err,
			))
		}
	}

	instrument.Transition(runtime.WAITING)
	return nil
}

func (instrument *Instrument) Symbols() []string {
	return slices.Clone(instrument.symbols)
}
