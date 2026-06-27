package broker

import (
	"math"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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
desk treats missing or invalid metadata as a system invariant failure: Kraken
AssetPairs is loaded before trading starts, so entries and exits should never
have to invent exchange constraints.
*/
func (desk *Desk) instrumentFor(symbol string) (instrument, error) {
	raw, ok := desk.tree.Get([]byte("instrument/" + symbol + "/"))

	if !ok {
		return instrument{}, errnie.Err(
			errnie.Validation,
			"desk: missing instrument metadata for "+symbol,
			nil,
		)
	}

	artifact := datura.Acquire("broker", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(raw); err != nil {
		return instrument{}, errnie.Err(
			errnie.Validation,
			"desk: unpack instrument metadata for "+symbol,
			err,
		)
	}

	var meta instrument

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &meta); err != nil {
		return instrument{}, errnie.Err(
			errnie.Validation,
			"desk: decode instrument metadata for "+symbol,
			err,
		)
	}

	return meta, nil
}

/*
sizeBuy converts a risk fraction into an order quantity: the requested fraction
of free quote capital, priced at the live mark plus the configured adverse
slippage buffer, then aligned to the instrument's exchange constraints. It
returns an error when there is no mark, no free capital, or the aligned size
falls below the pair's minimums, so an unpriceable, unfunded, or sub-minimum
entry halts at the source instead of surfacing as an anonymous zero quantity.
*/
func (desk *Desk) sizeBuy(symbol string, fraction float64) (float64, error) {
	if fraction <= 0 {
		return 0, errnie.Err(errnie.Validation, "desk: non-positive entry fraction for "+symbol, nil)
	}

	mark, markErr := desk.markFor(symbol)
	if markErr != nil {
		return 0, markErr
	}

	entryMark := desk.entryMark(mark)
	free, freeErr := desk.freeQuote()
	if freeErr != nil {
		return 0, freeErr
	}

	return desk.alignEntry(symbol, (fraction*free)/entryMark, entryMark)
}

/*
roundQuantity rounds a quantity down to the instrument's qty_increment so the
exchange does not reject it for sub-increment precision. Rounding down never
spends (or sells) more than intended.
*/
func (desk *Desk) roundQuantity(symbol string, qty float64) (float64, error) {
	if qty <= 0 {
		return 0, errnie.Err(errnie.Validation, "desk: non-positive exit quantity for "+symbol, nil)
	}

	meta, metaErr := desk.instrumentFor(symbol)
	if metaErr != nil {
		return 0, metaErr
	}

	if meta.QtyIncrement <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"desk: invalid quantity increment for "+symbol,
			nil,
		)
	}

	rounded := alignQuantity(qty, meta.QtyIncrement)
	if rounded <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"desk: rounded exit quantity is non-positive for "+symbol,
			nil,
		)
	}

	return rounded, nil
}

/*
alignEntry rounds an entry quantity to qty_increment and returns an error when
the result falls below the pair's qty_min or its notional is below cost_min.
*/
func (desk *Desk) alignEntry(symbol string, qty, mark float64) (float64, error) {
	meta, metaErr := desk.instrumentFor(symbol)
	if metaErr != nil {
		return 0, metaErr
	}

	if meta.QtyIncrement > 0 {
		qty = alignQuantity(qty, meta.QtyIncrement)
	}

	if qty <= 0 || (meta.QtyMin > 0 && qty < meta.QtyMin) {
		return 0, errnie.Err(
			errnie.Validation,
			"desk: entry quantity below instrument minimum for "+symbol,
			nil,
		)
	}

	if meta.CostMin > 0 && mark > 0 && qty*mark < meta.CostMin {
		return 0, errnie.Err(
			errnie.Validation,
			"desk: entry notional below instrument minimum for "+symbol,
			nil,
		)
	}

	return qty, nil
}

func alignQuantity(qty, increment float64) float64 {
	rounded := math.Floor(qty/increment) * increment
	precision := incrementPrecision(increment)
	scale := math.Pow10(precision)

	return math.Floor(rounded*scale+1e-9) / scale
}

func incrementPrecision(increment float64) int {
	text := strconv.FormatFloat(increment, 'f', -1, 64)
	dot := strings.IndexByte(text, '.')

	if dot < 0 {
		return 0
	}

	return len(strings.TrimRight(text[dot+1:], "0"))
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
func (desk *Desk) freeQuote() (float64, error) {
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
		return 0, errnie.Err(errnie.Validation, "desk: no balances frame for quote sizing", nil)
	}

	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](latest, "asset", rowIndex, "asset")

		if asset == "" {
			return 0, errnie.Err(
				errnie.Validation,
				"desk: balances frame missing quote asset "+desk.quote,
				nil,
			)
		}

		if strings.ToUpper(asset) == desk.quote {
			balance := datura.Peek[float64](latest, "asset", rowIndex, "balance")
			if balance <= 0 {
				return 0, errnie.Err(
					errnie.Validation,
					"desk: non-positive free quote balance "+desk.quote,
					nil,
				)
			}

			return balance, nil
		}
	}
}
