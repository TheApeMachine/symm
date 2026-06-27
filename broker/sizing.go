package broker

import (
	"math"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

/*
instrument holds the per-pair exchange constraints Kraken publishes on the
instrument feed and the discover path stores in the tree under instrument/
{symbol}. Quantities and notionals the desk sends must honour these or the
exchange rejects the order.
*/
type instrument struct {
	QtyMin       float64 `json:"qty_min"`
	QtyIncrement float64 `json:"qty_increment"`
	CostMin      float64 `json:"cost_min"`
}

/*
instrumentFor reads the latest instrument metadata for symbol from the tree. The
second return is false when the pair has not been catalogued yet, in which case
the caller must not invent constraints — it leaves alignment to a later tick once
the instrument frame has arrived.
*/
func (desk *Desk) instrumentFor(symbol string) (instrument, bool) {
	raw, ok := desk.tree.Get([]byte("instrument/" + symbol + "/"))

	if !ok {
		return instrument{}, false
	}

	artifact := datura.Acquire("broker", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(raw); err != nil {
		return instrument{}, false
	}

	var meta instrument

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &meta); err != nil {
		return instrument{}, false
	}

	return meta, true
}

/*
sizeBuy converts a risk fraction into an order quantity: the requested fraction
of free quote capital, priced at the live mark plus the configured adverse
slippage buffer, then aligned to the instrument's exchange constraints. It
returns 0 (which the caller rejects) when there is no mark, no free capital, or
the aligned size falls below the pair's minimums, so an unpriceable, unfunded, or
sub-minimum entry never dispatches a bogus size.
*/
func (desk *Desk) sizeBuy(symbol string, fraction float64) float64 {
	mark := desk.markFor(symbol)

	if mark <= 0 {
		return 0
	}

	entryMark := desk.entryMark(mark)
	free := desk.freeQuote()

	if free <= 0 {
		return 0
	}

	return desk.alignEntry(symbol, (fraction*free)/entryMark, entryMark)
}

/*
roundQuantity rounds a quantity down to the instrument's qty_increment so the
exchange does not reject it for sub-increment precision. Rounding down never
spends (or sells) more than intended. An uncatalogued pair passes through.
*/
func (desk *Desk) roundQuantity(symbol string, qty float64) float64 {
	meta, ok := desk.instrumentFor(symbol)

	if !ok || meta.QtyIncrement <= 0 {
		return qty
	}

	return math.Floor(qty/meta.QtyIncrement) * meta.QtyIncrement
}

/*
alignEntry rounds an entry quantity to qty_increment and rejects it (returns 0)
when the result falls below the pair's qty_min or its notional is below cost_min.
This guards only entries — exits must always be able to flatten a position even
if the residual is below the exchange minimum, so they use roundQuantity instead.
*/
func (desk *Desk) alignEntry(symbol string, qty, mark float64) float64 {
	meta, ok := desk.instrumentFor(symbol)

	if !ok {
		return qty
	}

	if meta.QtyIncrement > 0 {
		qty = math.Floor(qty/meta.QtyIncrement) * meta.QtyIncrement
	}

	if qty <= 0 || (meta.QtyMin > 0 && qty < meta.QtyMin) {
		return 0
	}

	if meta.CostMin > 0 && mark > 0 && qty*mark < meta.CostMin {
		return 0
	}

	return qty
}

/*
entryMark prices buy sizing against the configured adverse fill buffer, so a
market entry reserves quote capital for slippage instead of spending off the
static midpoint. The slippage source matches the paper fill simulator and is
zero in tests or deployments that do not set it.
*/
func (desk *Desk) entryMark(mark float64) float64 {
	if mark <= 0 {
		return mark
	}

	slippageBPS := viper.GetFloat64("trading.paper.slippage_bps")

	if slippageBPS <= 0 {
		return mark
	}

	return mark * (1 + slippageBPS/10000)
}

/*
freeQuote reads the free balance of the configured quote currency from the latest
balances frame in the tree — the same fill-mutated ledger paper and live both
publish through frame.Publish.
*/
func (desk *Desk) freeQuote() float64 {
	var (
		latest      *datura.Artifact
		latestStamp int64
	)

	for candidate := range desk.tree.Seek([]byte("balances/")) {
		if candidate.Timestamp() >= latestStamp {
			latest = candidate
			latestStamp = candidate.Timestamp()
		}
	}

	if latest == nil {
		return 0
	}

	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](latest, "asset", rowIndex, "asset")

		if asset == "" {
			return 0
		}

		if strings.ToUpper(asset) == desk.quote {
			return datura.Peek[float64](latest, "asset", rowIndex, "balance")
		}
	}
}
