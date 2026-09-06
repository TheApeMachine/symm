package broker

import (
	"context"
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
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
Instrument owns validated immutable pair snapshots and the subscription universe.
Remember and On deep-copy decimals so callers cannot mutate cached precision.
*/
type Instrument struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	status  *runtime.Status
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

	ctx, cancel := context.WithCancel(api.Context())

	instrument := &Instrument{
		ctx:              ctx,
		cancel:           cancel,
		status:           runtime.NewStatus(),
		api:              api,
		price:            price,
		cache:            &sync.Map{},
		symbols:          []string{},
		quote:            viper.GetViper().GetString("market.quote_currency"),
		products:         make(map[string]string),
		symbolsByProduct: make(map[string]string),
	}
	instrument.status.Transition(runtime.BUSY)

	callback := make(chan any, 1)
	api.SubInstrument(callback)

	if err := api.Error(); err != nil {
		instrument.fail(errnie.Err(
			errnie.IO,
			"instrument: snapshot subscription failed",
			err,
		))

		return instrument
	}

	var returned any

	select {
	case returned = <-callback:
	case <-instrument.ctx.Done():
		instrument.fail(errnie.Err(
			errnie.IO,
			"instrument: snapshot unavailable",
			instrument.operationalError(),
		))

		return instrument
	}

	snapshot, valid := returned.(*kraken.Instrument)

	if !valid || snapshot == nil {
		instrument.fail(errnie.Err(
			errnie.Validation,
			"instrument: invalid snapshot response",
			nil,
		))

		return instrument
	}

	for _, pair := range snapshot.Data.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" || isExcludedBase(pair.Base) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}

	if err := instrument.loadFuturesProducts(); err != nil {
		instrument.fail(err)

		return instrument
	}

	// The transport attributes inbound frames with the same mapping the
	// subscription path uses, so both directions agree by construction.
	if api.Futures() != nil {
		api.Futures().SetResolver(instrument.FuturesSymbol)
	}

	instrument.status.Transition(runtime.WAITING)

	return instrument
}

func (instrument *Instrument) Cache(pairs []kraken.InstrumentPair) {
	for _, pair := range pairs {
		if pair.Quote != instrument.quote ||
			pair.Status != "online" ||
			slices.Contains(system.Cfg.Market.Instrument.Excluded, 	pair.Base) {
			continue
		}

		instrument.symbols = append(instrument.symbols, pair.Symbol)
		instrument.cache.Store(pair.Symbol, pair)
	}
}

/*
Status reports instrument readiness.
*/
func (instrument *Instrument) Status() runtime.Stage {
	return instrument.status.Current()
}

/*
Error returns the first terminal instrument failure.
*/
func (instrument *Instrument) Error() error {
	return instrument.err
}

func (instrument *Instrument) fail(err error) {
	if instrument == nil || err == nil || instrument.err != nil {
		return
	}

	instrument.err = errnie.Error(err)
	instrument.status.Transition(runtime.ERROR)
	instrument.cancel()
}

func (instrument *Instrument) operationalError() error {
	if instrument.err != nil {
		return instrument.err
	}

	if err := instrument.api.Error(); err != nil {
		return err
	}

	select {
	case <-instrument.ctx.Done():
		return instrument.ctx.Err()
	default:
		return nil
	}
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
Subscribe issues paced market-data batches for the online quote universe. It
subscribes only streams that enter a declared Workspace workload; capturing a
feed with no consumer would create an exact raw tape that can never influence
the system.
*/
func (instrument *Instrument) Subscribe() error {
	if err := instrument.operationalError(); err != nil {
		instrument.fail(err)

		return instrument.err
	}

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
			instrument.fail(errnie.Err(
				errnie.IO,
				"instrument: failed to load fee tier batch",
				err,
			))

			return instrument.err
		}

		for _, subscribe := range subscribers {
			subscribe(batch)

			if err := instrument.api.Error(); err != nil {
				instrument.fail(errnie.Err(
					errnie.IO,
					"instrument: required spot subscription failed",
					err,
				))

				return instrument.err
			}
		}

		if err := instrument.futuresLegs("subscribe", batch, []func([]string) error{
			instrument.api.SubFuturesTicker,
			instrument.api.SubFuturesTrades,
		}); err != nil {
			instrument.fail(err)

			return instrument.err
		}

		pace := time.NewTimer(viper.GetViper().GetDuration("market.subscribe.pace"))

		select {
		case <-pace.C:
		case <-instrument.ctx.Done():
			if !pace.Stop() {
				select {
				case <-pace.C:
				default:
				}
			}

			instrument.fail(errnie.Err(
				errnie.IO,
				"instrument: subscription interrupted",
				instrument.operationalError(),
			))

			return instrument.err
		}
	}

	if err := instrument.operationalError(); err != nil {
		instrument.fail(err)

		return instrument.err
	}

	instrument.status.Transition(runtime.READY)
	return nil
}

/*
Unsubscribe withdraws the market-data streams for the online quote universe. It
is the mirror of Subscribe and walks the same universe through the same batched
seam, so a deliberate teardown leaves the venue with no streams pointed at
sockets that are about to close.
*/
func (instrument *Instrument) Unsubscribe() error {
	if err := instrument.operationalError(); err != nil {
		instrument.fail(err)

		return instrument.err
	}

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

			if err := instrument.api.Error(); err != nil {
				instrument.fail(errnie.Err(
					errnie.IO,
					"instrument: required spot unsubscription failed",
					err,
				))

				return instrument.err
			}
		}

		if err := instrument.futuresLegs("unsubscribe", batch, []func([]string) error{
			instrument.api.UnsubFuturesTicker,
			instrument.api.UnsubFuturesTrades,
		}); err != nil {
			instrument.fail(err)

			return instrument.err
		}
	}

	instrument.status.Transition(runtime.WAITING)

	return nil
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
) error {
	if instrument.api.Futures() == nil {
		return nil
	}

	products := instrument.productIDs(batch)

	if len(products) == 0 {
		return nil
	}

	for _, feed := range feeds {
		if err := feed(products); err != nil {
			return errnie.Err(
				errnie.IO,
				fmt.Sprintf("instrument: failed to %s futures feed", action),
				err,
			)
		}
	}

	return nil
}

/*
loadFuturesProducts indexes the venue's futures instrument specifications into
both directions of the spot/futures mapping. The endpoint is public, so it needs
no credentials, and it is read once here alongside the spot instrument snapshot
so one construction settles the whole universe.
*/
func (instrument *Instrument) loadFuturesProducts() error {
	response, err := derivatives.NewREST().Instruments()

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"instrument: failed to load futures instrument specifications",
			err,
		)
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

	return nil
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

/*
Close cancels the instrument registry lifecycle.
*/
func (instrument *Instrument) Close() {
	if instrument == nil {
		return
	}

	instrument.cancel()

	if instrument.err == nil {
		instrument.status.Transition(runtime.DONE)
	}
}
