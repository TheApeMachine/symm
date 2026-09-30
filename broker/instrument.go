package broker

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
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
	futures *network.WebsocketClient
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
}

/*
NewInstrument creates the market-instrument registry
used by subscriptions and order validation.
*/
func NewInstrument(
	public *network.WebsocketClient,
	futures *network.WebsocketClient,
) *Instrument {
	instrument := &Instrument{
		public:           public,
		futures:          futures,
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
Subscribe issues paced market-data batches for the online quote universe. It
subscribes only streams that enter a declared Workspace workload; capturing a
feed with no consumer would create an exact raw tape that can never influence
the system.
*/
func (instrument *Instrument) Subscribe() error {
	errnie.Info("[instrument] subscribing to symbol pairs")

	restClient := spot.NewREST()
	restClient.PublicKey = os.Getenv("KRAKEN_API_KEY")
	restClient.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if nonce, err := kraken.ProcessAuthNonce(); err == nil && nonce != nil {
		restClient.Nonce = nonce.Next
	}

	var wsToken string

	if tokenRes, err := restClient.GetWebSocketsToken(); err == nil && tokenRes != nil {
		wsToken = tokenRes.Result.Token
	}

	for batch := range slices.Chunk(
		instrument.symbols, system.Cfg.Market.Subscribe.Batch,
	) {
		subs := []json.Marshaler{
			kraken.NewTradeSubscription(batch),
			kraken.NewTickerSubscription(batch),
			kraken.NewFuturesSubscription("ticker", batch),
			kraken.NewFuturesSubscription("trade", batch),
		}

		for _, sub := range subs {
			msg, _ := sonic.Marshal(sub)

			if err := instrument.public.Write(msg); err != nil {
				return instrument.Error(errnie.Err(
					errnie.IO,
					"[instrument] required spot subscription failed",
					err,
				))
			}
		}

		l3Client := network.NewWebsocketClient(instrument.System.Context())

		if err := l3Client.Open(system.Cfg.WebSocket.Endpoints.Level3); err == nil {
			if l3Msg, err := sonic.Marshal(
				kraken.NewLevel3Subscription(batch, wsToken),
			); err == nil {
				errnie.Info(
					"[instrument] subscribed to level3 for " + strings.Join(batch, "|"),
				)

				l3Client.Write(l3Msg)

				instrument.Level3.Store(
					strings.Join(batch, "|"), l3Client,
				)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	instrument.Transition(runtime.READY)
	return nil
}

/*
Unsubscribe withdraws the market-data streams for the online quote universe. It
is the mirror of Subscribe and walks the same universe through the same batched
seam, so a deliberate teardown leaves the venue with no streams pointed at
sockets that are about to close.
*/
func (instrument *Instrument) Unsubscribe() error {
	errnie.Info("[instrument] unsubscribing from symbol pairs")

	for batch := range slices.Chunk(
		instrument.symbols, system.Cfg.Market.Subscribe.Batch,
	) {
		subs := []json.Marshaler{
			kraken.NewTradeUnsubscription(batch),
			kraken.NewTickerUnsubscription(batch),
			kraken.NewLevel3Unsubscription(batch),
			kraken.NewFuturesUnsubscription("ticker", batch),
			kraken.NewFuturesUnsubscription("trade", batch),
		}

		for _, sub := range subs {
			msg, _ := sonic.Marshal(sub)

			if err := instrument.public.Write(msg); err != nil {
				return instrument.Error(errnie.Err(
					errnie.IO,
					"[instrument] required spot unsubscription failed",
					err,
				))
			}
		}
	}

	instrument.Transition(runtime.WAITING)
	return nil
}

/*
Symbols returns a copy of the subscribed market universe.
*/
func (instrument *Instrument) Symbols() []string {
	return slices.Clone(instrument.symbols)
}
