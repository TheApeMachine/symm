package broker

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/derivatives"
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
func NewInstrument(api *websocket.API, price *Price) *Instrument {
	if api == nil {
		panic("broker: api required")
	}

	if price == nil {
		panic("broker: price required")
	}

	instrument := &Instrument{
		status:           types.INITIALIZING,
		api:              api,
		price:            price,
		cache:            &sync.Map{},
		symbols:          []string{},
		quote:            viper.GetViper().GetString("market.quote_currency"),
		products:         make(map[string]string),
		symbolsByProduct: make(map[string]string),
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

	instrument.loadFuturesProducts()

	// The transport attributes inbound frames with the same mapping the
	// subscription path uses, so both directions agree by construction.
	if api.Futures() != nil {
		api.Futures().SetResolver(instrument.FuturesSymbol)
	}

	instrument.status = types.PENDING

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

		instrument.futuresLegs("subscribe", batch, []func([]string) error{
			instrument.api.SubFuturesTicker,
			instrument.api.SubFuturesTrades,
			instrument.api.SubFuturesBook,
		})

		time.Sleep(viper.GetViper().GetDuration("market.subscribe.pace"))
	}

	instrument.status = types.READY
	return nil
}

/*
Unsubscribe withdraws the market-data streams for the online quote universe. It
is the mirror of Subscribe and walks the same universe through the same batched
seam, so a deliberate teardown leaves the venue with no streams pointed at
sockets that are about to close.
*/
func (instrument *Instrument) Unsubscribe() {
	errnie.Info("unsubscribing from instruments")

	unsubscribers := []func([]string){
		instrument.api.UnsubTrades,
		instrument.api.UnsubTicker,
		instrument.api.UnsubL3,
	}

	for batch := range slices.Chunk(
		instrument.symbols, viper.GetViper().GetInt("market.subscribe.batch"),
	) {
		for _, unsubscribe := range unsubscribers {
			unsubscribe(batch)
		}

		instrument.futuresLegs("unsubscribe", batch, []func([]string) error{
			instrument.api.UnsubFuturesTicker,
			instrument.api.UnsubFuturesTrades,
			instrument.api.UnsubFuturesBook,
		})
	}

	instrument.status = types.PENDING
}

/*
futuresLegs applies one batch of spot symbols to the futures feeds, translating
them to the venue's product identifiers first. Symbols with no perpetual
contract carry no futures leg and drop out of the batch, so the venue is never
asked about a product it does not list.
*/
func (instrument *Instrument) futuresLegs(
	action string,
	batch []string,
	feeds []func([]string) error,
) {
	if instrument.api.Futures() == nil {
		return
	}

	products := instrument.productIDs(batch)

	if len(products) == 0 {
		return
	}

	for _, feed := range feeds {
		if err := feed(products); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("instrument: failed to %s futures feed", action),
				err,
			))
		}
	}
}

/*
loadFuturesProducts indexes the venue's futures instrument specifications into
both directions of the spot/futures mapping. The endpoint is public, so it needs
no credentials, and it is read once here alongside the spot instrument snapshot
so one construction settles the whole universe.
*/
func (instrument *Instrument) loadFuturesProducts() {
	response, err := derivatives.NewREST().Instruments()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"instrument: failed to load futures instrument specifications",
			err,
		))

		return
	}

	for _, listed := range response.Result.Instruments {
		// Pair is the venue's own spot-form name for the contract's underlying,
		// which is the join to the spot universe and needs no alias table of
		// ours. Futures reports it colon-separated ("BTC:USD") where the spot
		// universe is slash-separated, so only the separator is reconciled.
		symbol := strings.ToUpper(strings.ReplaceAll(listed.Pair, ":", "/"))
		product := strings.ToUpper(listed.Symbol)

		if symbol == "" || product == "" {
			continue
		}

		instrument.symbolsByProduct[product] = symbol

		// Only a tradeable perpetual is a subscription target. Dated futures
		// and delisted contracts still resolve inbound frames above, but must
		// not be subscribed as a symbol's futures leg.
		if listed.Tradeable && strings.HasPrefix(product, "PF_") {
			instrument.products[symbol] = product
		}
	}
}

/*
productIDs returns the perpetual futures product identifiers for the spot
symbols in a batch, dropping the symbols the venue lists no contract for.

The two namespaces do not follow a derivable rule — the futures leg of "BTC/USD"
is "PF_XBTUSD" while that of "DOGE/USD" is "PF_DOGEUSD", so no alias table gets
both right — and most spot pairs have no futures contract at all. The venue's
own instrument list is the authority on both questions, so the mapping is built
from it and a lookup miss simply means the symbol carries no futures leg.
*/
func (instrument *Instrument) productIDs(batch []string) []string {
	products := make([]string, 0, len(batch))

	for _, symbol := range batch {
		if product, listed := instrument.products[symbol]; listed {
			products = append(products, product)
		}
	}

	return products
}

/*
FuturesSymbol returns the spot symbol carrying a futures product identifier, so
inbound futures frames attribute to the same symbol the spot streams use.
*/
func (instrument *Instrument) FuturesSymbol(productID string) (string, bool) {
	symbol, listed := instrument.symbolsByProduct[strings.ToUpper(productID)]

	return symbol, listed
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
