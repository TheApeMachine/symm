package broker

import (
	"slices"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Instrument owns validated immutable pair snapshots and the subscription universe.
Remember and On deep-copy decimals so callers cannot mutate cached precision.
*/
type Instrument struct {
	status  atomic.Value
	api     *websocket.API
	price   *Price
	cache   map[string]kraken.InstrumentPair
	quote   string
	market  config.MarketConfig
	uiHub   chan []byte
	symbols []string
	active  bool
}

/*
NewInstrument creates the market-instrument registry used by subscriptions and
order validation.
*/
func NewInstrument(
	api *websocket.API,
	price *Price,
	channel chan []byte,
	market config.MarketConfig,
) *Instrument {
	instrument := &Instrument{
		api:    api,
		price:  price,
		cache:  make(map[string]kraken.InstrumentPair),
		quote:  market.QuoteCurrency,
		market: market,
		uiHub:  channel,
	}
	instrument.status.Store(types.INITIALIZING)

	if instrument.api != nil {
		instrument.api.SetPublicResubscribe(instrument.Resubscribe)
	}

	return instrument
}

/*
Initialize requests the instrument snapshot stream.
*/
func (instrument *Instrument) Initialize() error {
	errnie.Info("initializing instrument")
	instrument.status.Store(types.PENDING)

	if instrument.api == nil {
		instrument.status.Store(types.READY)
		return nil
	}

	if err := instrument.api.SubscribeInstruments(); err != nil {
		instrument.status.Store(types.ERROR)
		return errnie.Error(err)
	}

	return nil
}

/*
On ingests one instrument frame, storing deep-copied pair snapshots by value.
*/
func (instrument *Instrument) On(message any) any {
	frame := message.(*kraken.Instrument)

	if len(frame.Data.Pairs) == 0 {
		return nil
	}

	for index := range frame.Data.Pairs {
		pair := frame.Data.Pairs[index]
		instrument.cache[pair.Symbol] = pair
	}

	instrument.Publish()

	if slices.Contains([]types.Status{
		types.READY, types.ERROR,
	}, instrument.Status()) {
		return nil
	}

	if err := instrument.Subscribe(); err != nil {
		instrument.status.Store(types.ERROR)
		errnie.Error(err)
	}

	return nil
}

/*
Publish forwards the online quote-currency instrument universe to the terminal.
*/
func (instrument *Instrument) Publish() {
	if instrument.uiHub == nil {
		return
	}

	select {
	case instrument.uiHub <- datura.NewMap(
		"instruments", instrument.Pairs(),
	).MarshalAndFree():
	default:
	}
}

/*
Status reports instrument readiness.
*/
func (instrument *Instrument) Status() types.Status {
	status := instrument.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

/*
QuoteTotal returns the subscribed symbol count.
*/
func (instrument *Instrument) QuoteTotal() int {
	if instrument == nil {
		return 0
	}

	return len(instrument.symbols)
}

/*
Pairs returns deep-copied instrument snapshots sorted by symbol.
*/
func (instrument *Instrument) Pairs() []kraken.InstrumentPair {
	pairs := make([]kraken.InstrumentPair, 0, len(instrument.cache))

	for _, pair := range instrument.cache {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		pairs = append(pairs, copyPair(pair))
	}

	return pairs
}

/*
Remember stores one deep-copied pair for paper/fixture sizing.
*/
func (instrument *Instrument) Remember(pair kraken.InstrumentPair) {
	if pair.Symbol == "" {
		return
	}

	instrument.cache[pair.Symbol] = copyPair(pair)
}

/*
Pair returns a deep-copied instrument snapshot for the symbol.
*/
func (instrument *Instrument) Pair(symbol string) (kraken.InstrumentPair, error) {
	pair, ok := instrument.cache[symbol]

	if !ok {
		return kraken.InstrumentPair{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader: instrument pair not found",
			nil,
		))
	}

	return copyPair(pair), nil
}

/*
copyPair returns an independent instrument snapshot so cached venue precision
cannot be mutated through Pairs, Remember, or Pair callers.
*/
func copyPair(pair kraken.InstrumentPair) kraken.InstrumentPair {
	if pair.QtyIncrement != nil {
		pair.QtyIncrement = pair.QtyIncrement.Copy()
	}

	if pair.CostMin != nil {
		pair.CostMin = pair.CostMin.Copy()
	}

	if pair.QtyMin != nil {
		pair.QtyMin = pair.QtyMin.Copy()
	}

	return pair
}

/*
Subscribe issues paced market-data batches for the online quote universe.
*/
func (instrument *Instrument) Subscribe() error {
	if instrument.Status() == types.READY {
		return nil
	}

	errnie.Info("subscribing to instruments")

	symbols := make([]string, 0)
	seen := make(map[string]struct{})

	for _, pair := range instrument.cache {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		if _, duplicate := seen[pair.Symbol]; duplicate {
			continue
		}

		seen[pair.Symbol] = struct{}{}
		symbols = append(symbols, pair.Symbol)
	}

	if len(symbols) == 0 {
		instrument.status.Store(types.INITIALIZING)
		return nil
	}

	batchSize := instrument.market.SubscribeBatch

	if batchSize < 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: market.subscribe_batch must be at least 1",
			nil,
		))
	}

	pace := instrument.market.SubscribePace
	instrument.symbols = append([]string(nil), symbols...)

	subscribers := []func([]string) error{
		instrument.api.SubscribeTrade,
		instrument.api.SubscribeBook,
		instrument.api.SubscribeTicker,
	}

	if instrument.market.L3Enabled {
		subscribers = append(subscribers, instrument.api.SubscribeLevel3)
	}

	for batch := range slices.Chunk(symbols, batchSize) {
		if err := instrument.price.GetFees(batch); err != nil {
			return errnie.Error(err)
		}

		for _, subscribe := range subscribers {
			if err := subscribe(batch); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"trader: market subscription failed",
					err,
				))
			}
		}

		time.Sleep(pace)
	}

	instrument.status.Store(types.READY)

	return nil
}

/*
Resubscribe re-issues instrument and market channel subscriptions after reconnect.
*/
func (instrument *Instrument) Resubscribe() error {
	if instrument == nil || instrument.api == nil {
		return nil
	}

	if instrument.Status() != types.READY {
		return nil
	}

	return instrument.api.ResubscribeMarket(instrument.symbols)
}

/*
Symbols returns a copy of the subscribed market universe.
*/
func (instrument *Instrument) Symbols() []string {
	if instrument == nil {
		return nil
	}

	return append([]string(nil), instrument.symbols...)
}
