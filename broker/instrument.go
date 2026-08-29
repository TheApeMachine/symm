package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"

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
	ctx context.Context,
	bus *runtime.Workspace,
) *Instrument {
	if bus == nil {
		panic("broker: workspace bus required")
	}

	var api *websocket.API
	if shared, _ := bus.Shared("api", ""); shared != nil {
		api, _ = shared.(*websocket.API)
	}
	if api == nil {
		panic("broker: api not found in workspace")
	}

	var price *Price
	if shared, _ := bus.Shared("price", ""); shared != nil {
		price, _ = shared.(*Price)
	}
	if price == nil {
		panic("broker: price not found in workspace")
	}
	instrument := &Instrument{
		status:  types.INITIALIZING,
		api:     api,
		price:   price,
		cache:   &sync.Map{},
		symbols: []string{},
		quote:   viper.GetViper().GetString("market.quote_currency"),
	}

	callback := make(chan any, 1)
	api.SubInstrument(callback)

	returned := <-callback

	for _, pair := range returned.(*kraken.Instrument).Data.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" || isExcludedBase(pair.Base) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}

	instrument.status = types.PENDING

	bus.On(types.ChannelDisconnect, func() {
		errnie.Info("instrument: channel disconnect received")
	})

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
Subscribe issues paced market-data batches for the online quote universe.
*/
func (instrument *Instrument) Subscribe() error {
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
		errnie.Info(fmt.Sprintf("subscribing to %d symbols", len(batch)))

		if err := instrument.price.GetFees(batch); err != nil {
			errnie.Warn(fmt.Sprintf("instrument: failed to load fee tier batch: %v", err))
		}

		for _, subscribe := range subscribers {
			subscribe(batch)
		}

		futuresBatch := make([]string, 0, len(batch))

		for _, spotSymbol := range batch {
			if pid := kraken.SpotToFuturesProductID(spotSymbol); pid != "" {
				futuresBatch = append(futuresBatch, pid)
			}
		}

		if len(futuresBatch) > 0 && instrument.api.Futures() != nil {
			_ = instrument.api.SubFuturesTicker(futuresBatch)
			_ = instrument.api.SubFuturesTrades(futuresBatch)
			_ = instrument.api.SubFuturesBook(futuresBatch)
		}

		time.Sleep(viper.GetViper().GetDuration("market.subscribe.pace"))
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

func isExcludedBase(base string) bool {
	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD",
		"USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG",
		"USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD",
		"CUSD", "USD0", "USDS", "RLUSD", "UST":
		return true
	default:
		return false
	}
}
