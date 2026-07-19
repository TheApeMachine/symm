package conditions

import (
	"encoding/json"
	"hash/crc32"
	"strconv"
	"strings"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
)

/*
Level3Path emits checksum-valid Kraken L3 population epochs from explicit
midpoints and bid/ask quantities. Stable order identities let the production
manifold measure causal coordinate motion when the book reprices.
*/
func Level3Path(
	midPrices []float64,
	bidQuantities [][]float64,
	askQuantities [][]float64,
	stamps []time.Time,
) *tests.Market {
	return Level3PathFor(
		subjectSymbol, 0.0001, midPrices, bidQuantities, askQuantities, stamps,
	)
}

/*
Level3PathFor emits a checksum-valid L3 path for an explicit instrument using
its venue price increment.
*/
func Level3PathFor(
	symbol string,
	priceIncrement float64,
	midPrices []float64,
	bidQuantities [][]float64,
	askQuantities [][]float64,
	stamps []time.Time,
) *tests.Market {
	frameCount := len(midPrices)

	if symbol == "" || priceIncrement <= 0 || frameCount == 0 ||
		len(bidQuantities) != frameCount ||
		len(askQuantities) != frameCount || len(stamps) != frameCount {
		panic(errnie.Err(errnie.Validation, "conditions: aligned L3 path required", nil))
	}

	payloads := make([][]byte, frameCount)
	var prior level3Epoch

	for index := range frameCount {
		epoch := newLevel3Epoch(
			symbol, priceIncrement, midPrices[index], bidQuantities[index],
			askQuantities[index], stamps[index],
		)
		payloads[index] = epoch.payload(prior)
		prior = epoch
	}

	return tests.NewMarket().
		Prefix(instrumentfixture.NewFixture(instrumentfixture.SNAPSHOT, 1)).
		Feed(tests.NewStaticSequence(payloads...))
}

/*
level3Epoch is one complete synthetic visible-order population.
*/
type level3Epoch struct {
	symbol string
	bids   []level3Order
	asks   []level3Order
	at     time.Time
}

/*
level3Order retains the exact decimal strings used by both Kraken payloads and
the venue checksum so the producer cannot pass a differently rounded book.
*/
type level3Order struct {
	id       string
	price    *krakendecimal.Decimal
	quantity *krakendecimal.Decimal
}

/*
newLevel3Epoch builds one two-sided population around an explicit midpoint.
*/
func newLevel3Epoch(
	symbol string,
	priceIncrement float64,
	midPrice float64,
	bidQuantities []float64,
	askQuantities []float64,
	at time.Time,
) level3Epoch {
	if symbol == "" || priceIncrement <= 0 || midPrice <= 0 || len(bidQuantities) < 2 ||
		len(askQuantities) < 2 || at.IsZero() {
		panic(errnie.Err(errnie.Validation, "conditions: valid two-level L3 epoch required", nil))
	}

	epoch := level3Epoch{symbol: symbol, at: at}

	for index, quantity := range bidQuantities {
		epoch.bids = append(epoch.bids, newLevel3Order(
			"bid-"+strconv.Itoa(index),
			midPrice-priceIncrement*float64(index+1), quantity,
		))
	}

	for index, quantity := range askQuantities {
		epoch.asks = append(epoch.asks, newLevel3Order(
			"ask-"+strconv.Itoa(index),
			midPrice+priceIncrement*float64(index+1), quantity,
		))
	}

	return epoch
}

/*
newLevel3Order validates and decimalizes one synthetic resting order.
*/
func newLevel3Order(id string, price float64, quantity float64) level3Order {
	if quantity <= 0 {
		panic(errnie.Err(errnie.Validation, "conditions: positive L3 quantity required", nil))
	}
	priceDecimal, err := krakendecimal.NewFromString(
		strconv.FormatFloat(price, 'f', 8, 64),
	)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "conditions: decimal L3 price required", err))
	}

	quantityDecimal, err := krakendecimal.NewFromString(
		strconv.FormatFloat(quantity, 'f', 8, 64),
	)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "conditions: decimal L3 quantity required", err))
	}

	return level3Order{
		id:       id,
		price:    priceDecimal,
		quantity: quantityDecimal,
	}
}

/*
payload replaces the prior population through ordinary deletes and adds, then
publishes the checksum of the resulting book.
*/
func (epoch level3Epoch) payload(prior level3Epoch) []byte {
	bids := epoch.events(prior.bids, epoch.bids)
	asks := epoch.events(prior.asks, epoch.asks)

	return tests.MarshalFrame(map[string]any{
		"channel": "level3",
		"type":    "update",
		"data": []any{map[string]any{
			"symbol":   epoch.symbol,
			"bids":     bids,
			"asks":     asks,
			"checksum": epoch.checksum(),
		}},
	})
}

/*
events preserves order identity while moving each order between price epochs.
*/
func (epoch level3Epoch) events(
	prior []level3Order,
	current []level3Order,
) []map[string]any {
	events := make([]map[string]any, 0, len(prior)+len(current))

	for _, order := range prior {
		events = append(events, order.event("delete", epoch.at))
	}

	for _, order := range current {
		events = append(events, order.event("add", epoch.at))
	}

	return events
}

/*
event encodes one Kraken L3 lifecycle operation.
*/
func (order level3Order) event(event string, at time.Time) map[string]any {
	row := map[string]any{
		"event":       event,
		"order_id":    order.id,
		"limit_price": json.Number(order.price.String()),
		"timestamp":   at.UTC().Format(time.RFC3339Nano),
	}

	if event != "delete" {
		row["order_qty"] = json.Number(order.quantity.String())
	}

	return row
}

/*
checksum calculates Kraken's ask-then-bid L3 CRC over the resulting population.
*/
func (epoch level3Epoch) checksum() uint32 {
	var value strings.Builder

	for _, order := range epoch.asks {
		value.WriteString(level3ChecksumPart(order))
	}

	for _, order := range epoch.bids {
		value.WriteString(level3ChecksumPart(order))
	}

	return crc32.Checksum([]byte(value.String()), crc32.IEEETable)
}

/*
level3ChecksumPart removes decimal punctuation and leading zeroes per Kraken's
L3 checksum contract.
*/
func level3ChecksumPart(order level3Order) string {
	price := strings.TrimLeft(strings.ReplaceAll(order.price.String(), ".", ""), "0")
	quantity := strings.TrimLeft(strings.ReplaceAll(order.quantity.String(), ".", ""), "0")

	return price + quantity
}
