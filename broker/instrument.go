package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
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

	// level3Stale receives a batch's symbols when its Level3 socket drops,
	// so the book stops serving state that no longer tracks the venue.
	level3Stale func([]string)
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
fetching a fresh one through the authenticated process REST client when the
cached token is missing or past level3TokenReuse.
*/
func (instrument *Instrument) level3Token() (string, error) {
	instrument.tokenMu.Lock()
	defer instrument.tokenMu.Unlock()

	if instrument.token != "" && time.Since(instrument.tokenAt) < level3TokenReuse {
		return instrument.token, nil
	}

	restClient, err := kraken.NewAuthenticatedREST()

	if err != nil {
		return "", err
	}

	tokenRes, err := restClient.GetWebSocketsToken()

	if err != nil || tokenRes == nil {
		return "", errnie.Err(
			errnie.IO,
			"[instrument] level3 websocket token unavailable",
			err,
		)
	}

	if tokenRes.Result.Token == "" {
		return "", errnie.Err(
			errnie.IO,
			"[instrument] level3 websocket token empty",
			nil,
		)
	}

	instrument.token = tokenRes.Result.Token
	instrument.tokenAt = time.Now()

	return instrument.token, nil
}

/*
level3Subscription builds the Level3 subscribe frame for one batch with a
currently valid token.
*/
func (instrument *Instrument) level3Subscription(batch []string) ([]byte, error) {
	token, err := instrument.level3Token()

	if err != nil {
		return nil, err
	}

	msg, err := sonic.Marshal(kraken.NewLevel3Subscription(batch, token))

	if err != nil {
		return nil, errnie.Err(
			errnie.IO,
			"[instrument] level3 subscribe marshal failed",
			err,
		)
	}

	return msg, nil
}

/*
halt records a market-data fault the instrument cannot continue past and
closes the instrument. Root watches the instrument context, so the process
stops with this error instead of running on a stream that silently went dark.
*/
func (instrument *Instrument) halt(cause error) error {
	err := instrument.Error(cause)
	instrument.Close()

	return err
}

/*
Subscribe issues paced market-data batches for the online quote universe. The
public socket carries trades only, the sole frame type the workspace pipeline
consumes. Level3 runs on its own authenticated socket per batch and feeds the
BookManager only. A failed resubscribe after a reconnect halts the instrument,
exactly like a failed initial subscribe.
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

		l3Msg, err := instrument.level3Subscription(batch)

		if err != nil {
			return instrument.Error(err)
		}

		l3Client := network.NewWebsocketClient(instrument.System.Context())

		if err := l3Client.Open(system.Cfg.WebSocket.Endpoints.Level3); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 open failed for "+batchKey,
				err,
			))
		}

		if err := l3Client.Write(l3Msg); err != nil {
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

			msg, err := instrument.level3Subscription(batch)

			if err == nil {
				err = l3Client.Write(msg)
			}

			if err != nil {
				return instrument.halt(errnie.Err(
					errnie.IO,
					"[instrument] level3 resubscribe failed for "+batchKey,
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
				return instrument.halt(errnie.Err(
					errnie.IO,
					"[instrument] trade resubscribe failed",
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
