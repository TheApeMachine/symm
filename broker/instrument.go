package broker

import (
	"slices"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Instrument owns validated immutable pair snapshots and the subscription universe.
Remember and On deep-copy decimals so callers cannot mutate cached precision.
*/
type Instrument struct {
	status  types.Status
	api     *websocket.API
	price   *Price
	cache   *sync.Map
	quote   string
	symbols []string
}

/*
NewInstrument creates the market-instrument registry
used by subscriptions and order validation.
*/
func NewInstrument(
	api *websocket.API,
	price *Price,
	channel chan []byte,
) *Instrument {
	instrument := &Instrument{
		status:  types.INITIALIZING,
		api:     api,
		price:   price,
		cache:   &sync.Map{},
		symbols: []string{},
		quote:   viper.GetViper().GetString("market.quote_currency"),
	}

	callback := types.NewSubscription[any]()
	api.SubInstrument(*callback)

	returned := <-callback.Channel

	for _, pair := range returned.(kraken.Instrument).Data.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}

	instrument.status = types.PENDING
	instrument.Subscribe()

	return instrument
}

/*
Status reports instrument readiness.
*/
func (instrument *Instrument) Status() types.Status {
	return instrument.status
}

/*
Pairs returns deep-copied instrument snapshots sorted by symbol.
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
Pair returns a deep-copied instrument snapshot for the symbol.
*/
func (instrument *Instrument) Pair(symbol string) (kraken.InstrumentPair, error) {
	value, ok := instrument.cache.Load(symbol)

	if !ok {
		return kraken.InstrumentPair{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader: instrument pair not found",
			nil,
		))
	}

	return value.(kraken.InstrumentPair), nil
}

/*
Subscribe issues paced market-data batches for the online quote universe.
*/
func (instrument *Instrument) Subscribe() error {
	if instrument.Status() == types.READY {
		return nil
	}

	errnie.Info("subscribing to instruments")

	subscribers := []func([]string){
		instrument.api.SubTrades,
		instrument.api.SubBook,
		instrument.api.SubTicker,
		instrument.api.SubL3,
	}

	for batch := range slices.Chunk(
		instrument.symbols, viper.GetViper().GetInt("market.subscribe.batch"),
	) {
		if err := instrument.price.GetFees(batch); err != nil {
			return errnie.Error(err)
		}

		for _, subscribe := range subscribers {
			subscribe(batch)
		}

		time.Sleep(viper.GetViper().GetDuration("market.subscribe.delay"))
	}

	instrument.status = types.READY
	return nil
}

/*
Symbols returns a copy of the subscribed market universe.
*/
func (instrument *Instrument) Symbols() []string {
	return instrument.symbols
}
