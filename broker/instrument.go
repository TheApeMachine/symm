package broker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/data"

	"github.com/bytedance/sonic"
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
	public       network.Client
	Level3       *sync.Map
	cache        *sync.Map
	takerFees    *sync.Map
	normalizer   *spot.Normalizer
	quote        string
	symbols      []string
	auth         *kraken.Auth
	excluded     []string
	level3URL    string
	booksMu      sync.Mutex
	resubMu      sync.Mutex
	resubRunning bool
	resubPending []string
}

/*
NewInstrument blocks startup on the instrument snapshot because every later step
(subscribe, fee load, order mins) depends on that single channel payload.
*/
func NewInstrument(
	ctx context.Context, public network.Client, normalizer *spot.Normalizer,
) *Instrument {
	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	instrument := &Instrument{
		public:     public,
		Level3:     &sync.Map{},
		cache:      &sync.Map{},
		takerFees:  &sync.Map{},
		normalizer: normalizer,
		symbols:    []string{},
		quote:      system.Cfg.Market.QuoteCurrency,
		auth:       kraken.NewAuth(),
		excluded:   append([]string(nil), system.Cfg.Market.Instrument.Excluded...),
		level3URL:  system.Cfg.WebSocket.Endpoints.Level3,
	}

	instrument.System = runtime.NewSystem(
		ctx, "instrument", instrument,
	)

	instrument.Transition(runtime.BUSY)

	if ws, ok := public.(*network.WebsocketClient); ok && ws != nil {
		if err := ws.Open(); err != nil {
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

		errnie.Info("[instrument] received valid instrument snapshot")
		instrument.Cache(snapshot.Data.Pairs)
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

	symbol := instrument.normalizer.Name(pair.Symbol)

	if !slices.Contains(instrument.symbols, symbol) {
		instrument.symbols = append(instrument.symbols, symbol)
	}

	instrument.cache.Store(symbol, pair)
}

/*
Cache injects pairs without a live instrument channel so desk tests and
PaperBook can size orders without mocking the full ws snapshot loop.
*/
func (instrument *Instrument) Cache(pairs []kraken.InstrumentPair) {
	for _, pair := range pairs {
		instrument.cachePair(pair)
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
Has guards Allocate and tier enter — symbols outside the quote filter must fail
before a socket subscription exists.
*/
func (instrument *Instrument) Has(symbol string) bool {
	_, ok := instrument.cache.Load(instrument.normalizer.Name(symbol))
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

	raw, ok := instrument.cache.Load(instrument.normalizer.Name(symbol))

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
	symbol = instrument.normalizer.Name(symbol)
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
SubscribeBooks opens one Level3 socket per market.subscribe.batch symbols.
Subscribe writes stay within l3_rate_limit (cost 5/symbol at depth 10), paced
by market.subscribe.pace, so a large socket batch does not blow the venue
budget. read starts before the first write so snapshots are not rejected.
*/
func (instrument *Instrument) SubscribeBooks(read func(*network.WebsocketClient)) error {
	errnie.Info("[instrument] subscribing to level3 books")

	instrument.booksMu.Lock()
	defer instrument.booksMu.Unlock()

	if err := instrument.openBooks(read, instrument.symbols); err != nil {
		return err
	}

	instrument.Transition(runtime.WAITING)
	return nil
}

/*
RecoverBooks marks the failed batches and resubscribes them off the caller.
Checksum recovery runs from the level3 pump, so the dial stays on a worker
and later failures fold into the same worker instead of stacking goroutines.
*/
func (instrument *Instrument) RecoverBooks(
	read func(*network.WebsocketClient),
	symbols []string,
	stale func([]string),
) {
	instrument.resubMu.Lock()
	instrument.resubPending = append(instrument.resubPending, symbols...)
	start := !instrument.resubRunning
	instrument.resubRunning = true
	instrument.resubMu.Unlock()

	if !start {
		return
	}

	go instrument.resubLoop(read, stale)
}

func (instrument *Instrument) resubLoop(
	read func(*network.WebsocketClient),
	stale func([]string),
) {
	for {
		instrument.resubMu.Lock()
		pending := uniqueSymbols(instrument.resubPending)
		instrument.resubPending = nil

		if len(pending) == 0 {
			instrument.resubRunning = false
			instrument.resubMu.Unlock()
			return
		}

		instrument.resubMu.Unlock()

		if err := instrument.resubscribe(read, pending, stale); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 resubscribe failed",
				err,
			))
		}
	}
}

func (instrument *Instrument) resubscribe(
	read func(*network.WebsocketClient),
	symbols []string,
	stale func([]string),
) error {
	instrument.booksMu.Lock()
	defer instrument.booksMu.Unlock()

	failed := uniqueSymbols(symbols)
	if len(failed) == 0 {
		return nil
	}

	// Only the checksum-failed symbols go blind — not every peer on the batch
	// socket. Peel them off existing connections, then openBooks just those.
	if stale != nil {
		stale(failed)
	}

	want := instrument.symbolSet(failed)
	depth := system.Cfg.Market.Level3.Depth
	token, tokenErr := instrument.auth.Token(instrument.Context())

	for _, victim := range instrument.level3Victims(failed) {
		keep := make([]string, 0, len(victim.parts))
		drop := make([]string, 0, len(victim.parts))

		for _, part := range victim.parts {
			if instrument.batchWanted([]string{part}, want) {
				drop = append(drop, part)
			} else {
				keep = append(keep, part)
			}
		}

		if len(drop) == 0 {
			continue
		}

		instrument.Level3.Delete(victim.key)

		if victim.client != nil && tokenErr == nil {
			_ = instrument.writeLevel3Unsub(victim.client, drop, token, depth)
		}

		if len(keep) == 0 {
			if victim.client != nil {
				_ = victim.client.Close()
			}
			continue
		}

		newKey := strings.Join(keep, "|")
		instrument.Level3.Store(newKey, victim.client)

		if victim.client != nil {
			kept := slices.Clone(keep)
			// Replace — Callback appends and would re-subscribe the pre-peel set.
			victim.client.SetCallback(func() error {
				fresh, err := instrument.auth.Token(instrument.Context())
				if err != nil {
					return errnie.Err(
						errnie.Forbidden,
						"[instrument] level3 reconnect token unavailable",
						err,
					)
				}
				frame := system.Cfg.Market.Level3.RateLimit / 5
				batch := system.Cfg.Market.Subscribe.Batch
				if frame <= 0 {
					frame = batch
				}
				if frame > batch {
					frame = batch
				}
				return instrument.writeLevel3(victim.client, kept, fresh, depth, frame)
			})
		}
	}

	errnie.Warn("[instrument] level3 resubscribe " + strings.Join(failed, ","))

	// Prefer spare capacity on live sockets over opening a dedicated WS per
	// peel — repeated single-symbol checksum recoveries otherwise accumulate
	// orphan connections for the rest of the session.
	attachToken := ""
	if tokenErr == nil {
		attachToken = token
	}
	remaining, err := instrument.attachBooks(failed, attachToken, depth, frameFromCfg())
	if err != nil {
		return err
	}

	if err := instrument.openBooks(read, remaining); err != nil {
		syms := append([]string(nil), failed...)
		go func() {
			time.Sleep(time.Second)
			instrument.RecoverBooks(read, syms, stale)
		}()

		return err
	}

	return nil
}

func frameFromCfg() int {
	batch := system.Cfg.Market.Subscribe.Batch
	frame := system.Cfg.Market.Level3.RateLimit / 5
	if frame <= 0 {
		frame = batch
	}
	if frame > batch {
		frame = batch
	}
	return frame
}

/*
attachBooks packs as many symbols as fit onto existing Level3 sockets under
Subscribe.Batch. Returns symbols that still need a new openBooks socket.
Caller holds booksMu. token may be empty when auth failed — then nothing
attaches and the full set is returned for openBooks to reject.
*/
func (instrument *Instrument) attachBooks(
	symbols []string,
	token string,
	depth, frame int,
) ([]string, error) {
	if token == "" || len(symbols) == 0 {
		return symbols, nil
	}

	batch := system.Cfg.Market.Subscribe.Batch
	if batch <= 0 {
		return symbols, nil
	}

	remaining := slices.Clone(symbols)
	var hosts []level3Batch

	instrument.Level3.Range(func(key, value any) bool {
		batchKey, _ := key.(string)
		client, _ := value.(*network.WebsocketClient)
		if client == nil || batchKey == "" {
			return true
		}
		hosts = append(hosts, level3Batch{
			key:    batchKey,
			parts:  strings.Split(batchKey, "|"),
			client: client,
		})
		return true
	})

	for _, host := range hosts {
		if len(remaining) == 0 {
			break
		}

		room := batch - len(host.parts)
		if room <= 0 {
			continue
		}

		take := remaining
		if len(take) > room {
			take = remaining[:room]
		}

		if err := instrument.writeLevel3(host.client, take, token, depth, frame); err != nil {
			return remaining, err
		}

		newParts := append(slices.Clone(host.parts), take...)
		newKey := strings.Join(newParts, "|")
		instrument.Level3.Delete(host.key)
		instrument.Level3.Store(newKey, host.client)

		kept := slices.Clone(newParts)
		host.client.SetCallback(func() error {
			fresh, err := instrument.auth.Token(instrument.Context())
			if err != nil {
				return errnie.Err(
					errnie.Forbidden,
					"[instrument] level3 reconnect token unavailable",
					err,
				)
			}
			return instrument.writeLevel3(host.client, kept, fresh, depth, frameFromCfg())
		})

		remaining = remaining[len(take):]
	}

	return remaining, nil
}

func (instrument *Instrument) writeLevel3Unsub(
	client *network.WebsocketClient,
	symbols []string,
	token string,
	depth int,
) error {
	if client == nil || len(symbols) == 0 {
		return nil
	}

	msg, err := sonic.Marshal(kraken.NewLevel3Unsubscription(symbols, token, depth))
	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[instrument] level3 unsubscribe marshal failed",
			err,
		)
	}

	return client.Write(msg)
}

type level3Batch struct {
	key    string
	parts  []string
	client *network.WebsocketClient
}

func (instrument *Instrument) level3Victims(symbols []string) []level3Batch {
	want := instrument.symbolSet(symbols)
	matched := make([]level3Batch, 0)
	all := make([]level3Batch, 0)

	instrument.Level3.Range(func(key, value any) bool {
		batchKey, _ := key.(string)
		batch := level3Batch{
			key:   batchKey,
			parts: strings.Split(batchKey, "|"),
		}

		if client, ok := value.(*network.WebsocketClient); ok {
			batch.client = client
		}

		all = append(all, batch)

		if len(want) == 0 || instrument.batchWanted(batch.parts, want) {
			matched = append(matched, batch)
		}

		return true
	})

	if len(matched) == 0 {
		return all
	}

	return matched
}

func (instrument *Instrument) symbolSet(symbols []string) map[string]struct{} {
	want := make(map[string]struct{}, len(symbols)*2)

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		want[symbol] = struct{}{}

		if instrument.normalizer != nil {
			want[instrument.normalizer.Name(symbol)] = struct{}{}
		}
	}

	return want
}

func (instrument *Instrument) batchWanted(parts []string, want map[string]struct{}) bool {
	for _, part := range parts {
		if _, ok := want[part]; ok {
			return true
		}

		if instrument.normalizer == nil {
			continue
		}

		if _, ok := want[instrument.normalizer.Name(part)]; ok {
			return true
		}
	}

	return false
}

func uniqueSymbols(symbols []string) []string {
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}

	return out
}

func (instrument *Instrument) openBooks(
	read func(*network.WebsocketClient),
	symbols []string,
) error {
	if read == nil {
		return instrument.Error(errnie.Err(
			errnie.Validation,
			"[instrument] level3 reader is required before subscribe",
			nil,
		))
	}

	if len(symbols) == 0 {
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

	depth := system.Cfg.Market.Level3.Depth
	batch := system.Cfg.Market.Subscribe.Batch
	frame := system.Cfg.Market.Level3.RateLimit / 5

	if frame <= 0 {
		frame = batch
	}

	if frame > batch {
		frame = batch
	}

	for chunk := range slices.Chunk(symbols, batch) {
		connSymbols := slices.Clone(chunk)
		batchKey := strings.Join(connSymbols, "|")

		l3Client := network.NewWebsocketClient(instrument.System.Context(), instrument.level3URL)

		if err := l3Client.Open(); err != nil {
			return instrument.Error(errnie.Err(
				errnie.IO,
				"[instrument] level3 open failed for "+batchKey,
				err,
			))
		}

		instrument.Level3.Store(batchKey, l3Client)
		read(l3Client)

		if err := instrument.writeLevel3(l3Client, connSymbols, token, depth, frame); err != nil {
			return instrument.Error(err)
		}

		// After the first subscribe: Open's reconnect path re-runs SetCallback.
		l3Client.SetCallback(func() error {
			fresh, err := instrument.auth.Token(instrument.Context())

			if err != nil {
				return errnie.Err(
					errnie.Forbidden,
					"[instrument] level3 reconnect token unavailable",
					err,
				)
			}

			return instrument.writeLevel3(l3Client, connSymbols, fresh, depth, frame)
		})

		errnie.Info("[instrument] subscribed to level3 for " + batchKey)
	}

	return nil
}

/*
writeLevel3 sends paced, rate-limit-sized subscribe frames for one socket.
*/
func (instrument *Instrument) writeLevel3(
	client *network.WebsocketClient,
	symbols []string,
	token string,
	depth, frame int,
) error {
	for chunk := range slices.Chunk(symbols, frame) {
		msg, err := sonic.Marshal(kraken.NewLevel3Subscription(chunk, token, depth))

		if err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe marshal failed",
				err,
			)
		}

		time.Sleep(system.Cfg.Market.Subscribe.Pace)

		if err := client.Write(msg); err != nil {
			return errnie.Err(
				errnie.IO,
				"[instrument] level3 subscribe failed",
				err,
			)
		}
	}

	return nil
}

/*
SubscribeTrades opens the trade firehose on the public socket, the sole frame
type the workspace pipeline consumes. It is the last startup phase: the
workspace and every book it reads are already READY.
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

	if ws, ok := instrument.public.(*network.WebsocketClient); ok {
		ws.Callback(func() error {
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
CloseLevel3 closes every open book websocket so readLevel3 goroutines unblock
on shutdown instead of sitting in ReadMessage until process exit.
*/
func (instrument *Instrument) CloseLevel3() {
	if instrument == nil || instrument.Level3 == nil {
		return
	}

	instrument.Level3.Range(func(_, value any) bool {
		if client, ok := value.(*network.WebsocketClient); ok && client != nil {
			_ = client.Close()
		}

		return true
	})
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
TakerFee is the account rate from TradeVolume, not the venue default — FeeNet
and Allocate budgets would lie if this table were empty.
*/
func (instrument *Instrument) TakerFee(symbol string) *decimal.Decimal {
	val, ok := instrument.takerFees.Load(instrument.normalizer.Name(symbol))

	if !ok || val == nil {
		return nil
	}

	return val.(*decimal.Decimal)
}
