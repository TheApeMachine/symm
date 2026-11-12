package broker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/data"

	"github.com/bytedance/sonic"
	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Instrument owns venue pair facts and the subscription universe. Decimal values
are shared as immutable SDK values; arithmetic returns a new Decimal.
*/
type Instrument struct {
	*runtime.System
	public    network.Client
	snapshot  *kraken.Instrument
	Level3    *sync.Map
	cache     *sync.Map
	takerFees *sync.Map
	quote     string
	symbols   []string
	auth      *kraken.Auth
	excluded  []string
	level3URL string
}

/*
Level3 passes desk-owned hooks into Instrument so stale marking and socket read
loops stay in broker/desk, not inside the instrument runtime type.
*/
type Level3 struct {
	Stale  func([]string)
	Reader func(*network.WebsocketClient)
}

/*
NewInstrument blocks startup on the instrument snapshot because every later step
(subscribe, fee load, order mins) depends on that single channel payload.
*/
func NewInstrument(
	ctx context.Context, public network.Client,
) *Instrument {
	instrument := &Instrument{
		public:    public,
		Level3:    &sync.Map{},
		cache:     &sync.Map{},
		takerFees: &sync.Map{},
		symbols:   []string{},
		quote:     system.Cfg.Market.QuoteCurrency,
		auth:      kraken.NewAuth(),
		excluded:  append([]string(nil), system.Cfg.Market.Instrument.Excluded...),
		level3URL: system.Cfg.WebSocket.Endpoints.Level3,
	}

	instrument.System = runtime.NewSystem(
		ctx, "instrument", instrument,
	)

	instrument.Transition(runtime.BUSY)

	if ws, ok := public.(*network.WebsocketClient); ok && ws != nil {
		if err := ws.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
			instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] public websocket open failed",
				err,
			))

			return instrument
		}
	}

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

	if err := instrument.seedSnapshot(message); err != nil {
		instrument.Error(err)
		return instrument
	}

	instrument.Transition(runtime.READY)
	return instrument
}

/*
seedSnapshot waits for the instrument channel snapshot. Soft disconnects
(Kraken 1013 market data unavailable, ping drops) SoftDown to WAITING — the
next Write redials and re-subscribes; only context cancel or a bad decode fail.
*/
func (instrument *Instrument) seedSnapshot(subscribe []byte) error {
	for {
		if err := instrument.Context().Err(); err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] snapshot unavailable",
				err,
			)
		}

		buf, err := instrument.public.Read()

		if err != nil {
			errnie.Warn(
				"[instrument] snapshot wait interrupted, resubscribing: " + err.Error(),
			)

			if writeErr := instrument.public.Write(subscribe); writeErr != nil {
				errnie.Warn(
					"[instrument] snapshot resubscribe failed: " + writeErr.Error(),
				)
			}

			continue
		}

		// Acks and heartbeats carry no instrument channel; a frame whose
		// channel cannot be read is an error, not something to skip past.
		channel, err := data.AttributeOk[string](buf, "channel")

		if errors.Is(err, data.ErrMissing) || (err == nil && channel != "instrument") {
			continue
		}

		if err != nil {
			errnie.Error(err)
			continue
		}

		snapshot := kraken.NewInstrument(buf)

		if snapshot == nil {
			return errnie.Err(
				errnie.UnprocessableContent,
				"[instrument] instrument snapshot did not decode",
				nil,
			)
		}

		instrument.snapshot = snapshot
		errnie.Info("[instrument] received valid instrument snapshot")

		for _, pair := range snapshot.Data.Pairs {
			instrument.cachePair(pair)
		}

		return nil
	}
}

/*
cachePair filters snapshot pairs to configured quote and exclusions before L3
and trade subscribe frames are built — non-online pairs never hit the wire.
*/
func (instrument *Instrument) cachePair(pair kraken.InstrumentPair) {
	if pair.Quote != instrument.quote || pair.Status != "online" || slices.Contains(
		instrument.excluded, pair.Base,
	) {
		return
	}

	if !slices.Contains(instrument.symbols, pair.Symbol) {
		instrument.symbols = append(instrument.symbols, pair.Symbol)
	}

	instrument.cache.Store(pair.Symbol, pair)
}

/*
Cache injects pairs without a live instrument channel so desk tests and
PaperBook can size orders without mocking the full ws snapshot loop.
*/
func (instrument *Instrument) Cache(pairs []kraken.InstrumentPair) {
	for _, pair := range pairs {
		if pair.Quote != instrument.quote ||
			pair.Status != "online" ||
			slices.Contains(instrument.excluded, pair.Base) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}
}

/*
Pairs walks the sync.Map because normalizer seeding and Allow lists need the full
online universe, not just the symbol string slice order.
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
NormalizerFor seeds Kraken's spot.Normalizer from the desk instrument cache so
Price can round Allocate to lot steps and resolve REST pair keys (TradeVolume,
Balance assets, ticker rows) onto ws symbols — the SDK has no hook into Instrument.
*/
func NormalizerFor(instrument *Instrument) *spot.Normalizer {
	normalizer := spot.NewNormalizer()

	if instrument == nil {
		return normalizer
	}

	update := &spot.AssetsManagerUpdate{
		NewAssets: make(map[string]spot.AssetInfo),
		NewPairs:  make(map[string]spot.AssetPair),
	}

	for _, pair := range instrument.Pairs() {
		update.NewAssets[pair.Base] = spot.AssetInfo{AltName: pair.Base}
		update.NewAssets[pair.Quote] = spot.AssetInfo{AltName: pair.Quote}

		var tick *sdkdecimal.Decimal

		if pair.TickSize != nil {
			tick, _ = sdkdecimal.NewFromString(pair.TickSize.String())
		}

		lotDecimals := int(pair.QtyPrecision)

		if pair.QtyIncrement != nil && pair.QtyIncrement.Sign() > 0 {
			if exp := pair.QtyIncrement.Exponent(); exp < 0 {
				lotDecimals = int(-exp)
			}
		}

		update.NewPairs[pair.Base+pair.Quote] = spot.AssetPair{
			WSName:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
			PairDecimals:  pair.PricePrecision,
			LotDecimals:   lotDecimals,
			LotMultiplier: 1,
			TickSize:      tick,
		}
	}

	normalizer.Update(update)
	return normalizer
}

/*
Has guards Allocate and tier enter — symbols outside the quote filter must fail
before a socket subscription exists.
*/
func (instrument *Instrument) Has(symbol string) bool {
	_, ok := instrument.cache.Load(symbol)
	return ok
}

/*
SpotOpen is true when the pair is in the cache with Kraken status online — spot
market and limit orders allowed. Cancel-only, post-only, and delisted pairs are
not managed on REAL.
*/
func (instrument *Instrument) SpotOpen(symbol string) bool {
	if instrument == nil || symbol == "" {
		return false
	}

	raw, ok := instrument.cache.Load(symbol)

	if !ok {
		return false
	}

	pair, ok := raw.(kraken.InstrumentPair)

	if !ok {
		return false
	}

	return strings.EqualFold(pair.Status, "online")
}

/*
Pair is the sizing source of QtyMin, CostMin, and increments; missing entries
log and return empty struct so callers fail loudly on bad symbols.
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
level3FrameSize is how many symbols one subscribe frame may carry: Kraken's
level3 subscription-rate budget per rate window divided by the rate-counter
cost of one symbol at depth (5 at 10, 25 at 100, 100 at 1000).
*/
func level3FrameSize(depth, budget int) (int, error) {
	cost := map[int]int{10: 5, 100: 25, 1000: 100}[depth]

	if cost == 0 {
		return 0, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[instrument] level3 depth %d is not one of 10, 100, 1000", depth),
			nil,
		)
	}

	if budget < cost {
		return 0, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[instrument] market.l3_rate_limit %d cannot fit one level3 symbol at depth %d (cost %d)",
				budget, depth, cost),
			nil,
		)
	}

	return budget / cost, nil
}

/*
SubscribeLevel3 writes the Level3 subscription for one batch with a currently
valid token, split into frames that each fit the venue's snapshot rate
budget, one frame per rate window across all sockets.
*/
func (instrument *Instrument) SubscribeLevel3(write func([]byte) error, batch []string) error {
	token, err := instrument.auth.Token(instrument.Context())

	if err != nil {
		return instrument.Error(errnie.Err(
			errnie.Forbidden,
			"[instrument] level3 auth token unavailable",
			err,
		))
	}

	depth := system.Cfg.Market.Level3.Depth
	frameSize, err := level3FrameSize(depth, system.Cfg.Market.Level3.RateLimit)

	if err != nil {
		return instrument.Error(err)
	}

	for frame := range slices.Chunk(batch, frameSize) {
		msg, err := sonic.Marshal(kraken.NewLevel3Subscription(
			frame, token, depth,
		))

		if err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe marshal failed",
				err,
			)
		}

		time.Sleep(system.Cfg.Market.Subscribe.Pace)

		if err := write(msg); err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe write failed for "+strings.Join(frame, ","),
				err,
			)
		}
	}

	return nil
}

/*
UnsubscribeLevel3 drops Level3 for one batch on a live socket. Required before
re-subscribing after a checksum failure: Kraken does not push a new snapshot
for a symbol that is already subscribed.
*/
func (instrument *Instrument) UnsubscribeLevel3(write func([]byte) error, batch []string) error {
	if len(batch) == 0 {
		return nil
	}

	token, err := instrument.auth.Token(instrument.Context())

	if err != nil {
		return instrument.Error(errnie.Err(
			errnie.Forbidden,
			"[instrument] level3 auth token unavailable",
			err,
		))
	}

	msg, err := sonic.Marshal(kraken.NewLevel3Unsubscription(batch, token))

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[instrument] level3 unsubscribe marshal failed",
			err,
		)
	}

	if err := write(msg); err != nil {
		return errnie.Err(
			errnie.IO,
			"[instrument] level3 unsubscribe write failed for "+strings.Join(batch, ","),
			err,
		)
	}

	return nil
}

/*
SubscribeBooks opens the Level3 sockets for the online quote universe and
issues their paced subscriptions. Level3 runs on its own authenticated socket
per batch of 200 symbols and feeds the BookManager only; it is the upstream
that must be seeded before the trade firehose (SubscribeTrades) is opened. 
A failed resubscribe after a reconnect is logged and returned to the socket, 
which drops the connection and redials with backoff until a resubscribe 
succeeds; the batch's books stay stale meanwhile. It never halts the instrument.
*/
func (instrument *Instrument) SubscribeBooks(level3 Level3) error {
	errnie.Info("[instrument] subscribing to level3 books")

	if level3.Reader == nil {
		return instrument.Error(errnie.Err(
			errnie.Validation,
			"[instrument] level3 reader is required before subscribe",
			nil,
		))
	}

	if level3.Stale == nil {
		return instrument.Error(errnie.Err(
			errnie.Validation,
			"[instrument] level3 stale hook is required before subscribe",
			nil,
		))
	}

	stale := level3.Stale
	for chunk := range slices.Chunk(instrument.symbols, 200) {
		connSymbols := slices.Clone(chunk)
		batchKey := strings.Join(connSymbols, "|")

		l3Client := network.NewWebsocketClient(instrument.System.Context())

		if err := l3Client.Open(instrument.level3URL); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 open failed for "+batchKey,
				err,
			))
		}

		instrument.Level3.Store(batchKey, l3Client)

		// Read before subscribing: unread snapshots make the venue reject
		// later snapshot requests (measured: 120 of 621 rejected with readers
		// started after all subscribes, 0 of 621 with readers running).
		level3.Reader(l3Client)

		if err := instrument.SubscribeLevel3(l3Client.Write, connSymbols); err != nil {
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
			stale(connSymbols)
		})

		// Open runs any registered reconnect hook, so the hook is registered
		// only after the initial subscribe was written once and checked.
		l3Client.OnReconnect(func() error {
			errnie.Warn("[instrument] level3 resubscribe in flight for " + batchKey)

			if err := instrument.SubscribeLevel3(l3Client.Write, connSymbols); err != nil {
				stale(connSymbols)

				return errnie.Err(
					errnie.IO,
					"[instrument] level3 resubscribe failed, books stale, redialing for "+batchKey,
					err,
				)
			}

			errnie.Info("[instrument] level3 stream recovered for " + batchKey)

			return nil
		})

		errnie.Info("[instrument] subscribed to level3 for " + batchKey)
		time.Sleep(100 * time.Millisecond)
	}

	instrument.Transition(runtime.WAITING)
	return nil
}

/*
ResubscribeLevel3 forces a fresh level3 snapshot for one symbol on the socket
batch that already carries it. Kraken's recovery path after a checksum
divergence is unsubscribe then subscribe with snapshot; a second subscribe on
a live channel alone does not replace the book, so the symbol would stay
invalid while trades keep flowing.
*/
func (instrument *Instrument) ResubscribeLevel3(symbol string) error {
	if symbol == "" {
		return nil
	}

	var resErr error

	instrument.Level3.Range(func(key, value any) bool {
		batchKey, ok := key.(string)
		if !ok {
			return true
		}

		if !slices.Contains(strings.Split(batchKey, "|"), symbol) {
			return true
		}

		client, ok := value.(*network.WebsocketClient)
		if !ok || client == nil {
			resErr = errnie.Err(
				errnie.Internal,
				"[instrument] level3 client missing for "+symbol,
				nil,
			)
			return false
		}

		errnie.Info("[instrument] level3 unsubscribe+subscribe after book invalidation for " + symbol)

		if err := instrument.UnsubscribeLevel3(client.Write, []string{symbol}); err != nil {
			resErr = errnie.Err(
				errnie.IO,
				"[instrument] level3 invalidation unsubscribe failed for "+symbol,
				err,
			)
			return false
		}

		if err := instrument.SubscribeLevel3(client.Write, []string{symbol}); err != nil {
			resErr = errnie.Err(
				errnie.IO,
				"[instrument] level3 invalidation resubscribe failed for "+symbol,
				err,
			)
			return false
		}

		return false
	})

	return resErr
}

/*
SubscribeTrades opens the trade firehose on the public socket, the sole frame
type the workspace pipeline consumes. It is the last startup phase: the
workspace and every book it reads are already READY. A failed resubscribe after
a reconnect is returned to the socket, which redials with backoff.
*/
func (instrument *Instrument) SubscribeTrades() error {
	errnie.Info("[instrument] opening the trade firehose")

	var tradeSubs [][]byte

	for chunk := range slices.Chunk(
		instrument.symbols, system.Cfg.Market.Subscribe.Batch,
	) {
		batch := slices.Clone(chunk)

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
	}

	if reconnector, ok := instrument.public.(interface{ OnReconnect(func() error) }); ok {
		reconnector.OnReconnect(func() error {
			errnie.Warn("[instrument] public trade resubscribe in flight")

			for _, msg := range tradeSubs {
				if err := instrument.public.Write(msg); err != nil {
					return errnie.Err(
						errnie.IO,
						"[instrument] trade resubscribe failed, redialing",
						err,
					)
				}
			}

			errnie.Info("[instrument] public trade stream recovered")

			return nil
		})
	}

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

/*
Symbols clones so TradeVolume fee fetch and chunk subscribe cannot mutate the
canonical list built during snapshot ingest.
*/
func (instrument *Instrument) Symbols() []string {
	return slices.Clone(instrument.symbols)
}

/*
WebSocketToken is the private-socket token for this process's API key.
Live order entry and the executions subscription use it. Paper does not.
*/
func (instrument *Instrument) WebSocketToken(ctx context.Context) (string, error) {
	return instrument.auth.Token(ctx)
}

/*
TakerFee is the account rate from TradeVolume, not the venue default — FeeNet
and Allocate budgets would lie if this table were empty.
*/
func (instrument *Instrument) TakerFee(symbol string) *decimal.Decimal {
	return instrument.fee(instrument.takerFees, symbol)
}

/*
fee shares one lookup path for fee tables stored on Instrument without exporting
the sync.Map or duplicating symbol→decimal parsing at every call site.
*/
func (instrument *Instrument) fee(table *sync.Map, symbol string) *decimal.Decimal {
	if table == nil || symbol == "" {
		return nil
	}

	val, ok := table.Load(symbol)

	if !ok || val == nil {
		return nil
	}

	return val.(*decimal.Decimal)
}
