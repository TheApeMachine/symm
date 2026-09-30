package broker

import (
	"encoding/json"
	"slices"
	"sync"

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
	futures *network.WebsocketClient
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
func NewInstrument(public *network.WebsocketClient, futures *network.WebsocketClient) *Instrument {
	if public == nil {
		panic("broker: public transport required")
	}

	instrument := &Instrument{
		public:           public,
		futures:          futures,
		cache:            &sync.Map{},
		symbols:          []string{},
		quote:            system.Cfg.Market.QuoteCurrency,
		products:         make(map[string]string),
		symbolsByProduct: make(map[string]string),
	}

	instrument.System = runtime.NewSystem(public.Context(), "instrument", instrument)
	instrument.Transition(runtime.BUSY)

	msg, _ := sonic.Marshal(kraken.NewInstrumentSubscription())
	if err := public.Write(msg); err != nil {
		instrument.Error(errnie.Err(
			errnie.IO,
			"instrument: snapshot subscription failed",
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
				"instrument: snapshot unavailable",
				err,
			))

			return instrument
		}

		var peek struct {
			Channel string `json:"channel"`
		}

		if err := sonic.UnmarshalString(string(buf), &peek); err == nil && peek.Channel == "instrument" {
			snapshot = kraken.NewInstrument(buf)
			break
		}
	}

	if snapshot == nil {
		instrument.Error(errnie.Err(
			errnie.Validation,
			"instrument: invalid snapshot response",
			nil,
		))

		return instrument
	}

	for _, pair := range snapshot.Data {
		if pair.Quote != instrument.quote || pair.Status != "online" || slices.Contains(system.Cfg.Market.Instrument.Excluded, pair.Base) {
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
	return nil
}

/*
Unsubscribe withdraws the market-data streams for the online quote universe. It
is the mirror of Subscribe and walks the same universe through the same batched
seam, so a deliberate teardown leaves the venue with no streams pointed at
sockets that are about to close.
*/
func (instrument *Instrument) Unsubscribe() error {
	errnie.Info("unsubscribing from instruments")

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
					"instrument: required spot unsubscription failed",
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
