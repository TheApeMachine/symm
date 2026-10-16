package broker

import (
	"context"
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
	public       Transport
	snapshot     *kraken.Instrument
	Level3       *sync.Map
	cache        *sync.Map
	quote        string
	symbols      []string
	auth         *kraken.Auth
	level3Stale  func([]string)
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
	ctx context.Context, public Transport, storeTees ...runtime.Tee,
) *Instrument {
	instrument := &Instrument{
		public:  public,
		Level3:  &sync.Map{},
		cache:   &sync.Map{},
		symbols: []string{},
		quote:   system.Cfg.Market.QuoteCurrency,
		auth:    kraken.NewAuth(),
	}

	instrument.System = runtime.NewSystem(
		ctx, "instrument", instrument,
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
		instrument.snapshot = snapshot
		errnie.Info("[instrument] received valid instrument snapshot")

		for _, pair := range snapshot.Data.Pairs {
			instrument.cachePair(pair)
		}

		break
	}

	instrument.Transition(runtime.READY)
	return instrument
}

func (instrument *Instrument) cachePair(pair kraken.InstrumentPair) {
	if pair.Quote != instrument.quote || pair.Status != "online" || slices.Contains(
		system.Cfg.Market.Instrument.Excluded, pair.Base,
	) {
		return
	}

	if !slices.Contains(instrument.symbols, pair.Symbol) {
		instrument.symbols = append(instrument.symbols, pair.Symbol)
	}

	instrument.cache.Store(pair.Symbol, pair)
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
subscribeLevel3 writes the Level3 subscription for one batch with a currently
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

	for frame := range slices.Chunk(batch, 40) {
		msg, err := sonic.Marshal(kraken.NewLevel3Subscription(
			frame, token, level3Depth(),
		))

		if err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe marshal failed",
				err,
			)
		}

		time.Sleep(viper.GetDuration("market.subscribe.pace"))
		write(msg)
	}

	return nil
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

	for chunk := range slices.Chunk(instrument.symbols, 200) {
		connSymbols := slices.Clone(chunk)
		batchKey := strings.Join(connSymbols, "|")

		l3Client := network.NewWebsocketClient(instrument.System.Context())

		if err := l3Client.Open(system.Cfg.WebSocket.Endpoints.Level3); err != nil {
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
		instrument.level3Reader(l3Client)

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
			errnie.Info("[instrument] level3 resubscribe after reconnect for " + batchKey)

			if err := instrument.SubscribeLevel3(l3Client.Write, connSymbols); err != nil {
				// The books of this batch were marked stale on disconnect
				// and stay so until a resubscribe snapshot lands.
				stale(connSymbols)

				return errnie.Error(errnie.Err(
					errnie.IO,
					"[instrument] level3 resubscribe failed, books stale, redialing for "+batchKey,
					err,
				))
			}

			return nil
		})

		errnie.Info("[instrument] subscribed to level3 for " + batchKey)
		time.Sleep(100 * time.Millisecond)
	}

	if reconnector, ok := instrument.public.(interface{ OnReconnect(func() error) }); ok {
		reconnector.OnReconnect(func() error {
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

func (instrument *Instrument) Symbols() []string {
	return slices.Clone(instrument.symbols)
}
