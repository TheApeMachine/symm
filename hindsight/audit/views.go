package audit

import (
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

/*
priceSource is the Origin of a Price view; touchTag ends the label list of
the touch layer TickViews adds to a join. That layer is a plain vector, not
a signal layer: the grid pins signals only.
*/
const (
	priceSource = "price"
	touchTag    = "touch"
)

/*
SymbolView is one symbol of one stored tick as the stages consume it. Every
view is a Value carrying the tick's Epoch, Tick and local Timestamp:

  - Trades: one per trade (Origin "trade"), metrics price, qty and at (the
    venue time, ns); SeqIdx is Kraken's trade_id, so a stream's sequence is the
    venue's own.
  - Signals: one per signal layer (Origin = source), its labelled metrics.
  - Price: the price/<symbol> layer (Origin "price"): best_price:bid,
    best_price:ask and fee_rate, each only when the tick recorded it.
  - Join: Role "runtime:join", every signal layer of the symbol plus the touch
    as a plain vector tagged "touch" (read it with Touch); its signal layers
    read with data.Signals and Grid.Observe.
*/
type SymbolView struct {
	Symbol  string
	Trades  []*data.Value
	Signals []*data.Value
	Price   *data.Value
	Join    *data.Value
}

/*
TickViews reads one stored tick (broker.ReadTick) into a view per symbol,
limited to only when it is set. A tick that does not read is an error.
*/
func TickViews(value *data.Value, only string) ([]*SymbolView, error) {
	ticks, err := broker.ReadTick(value)

	if err != nil {
		return nil, err
	}

	views := make([]*SymbolView, 0, len(ticks))

	for _, tick := range ticks {
		if only != "" && tick.Symbol != only {
			continue
		}

		view := &SymbolView{Symbol: tick.Symbol}
		header := func(origin, role string, seq uint64) *data.Value {
			out := &data.Value{
				Origin:    origin,
				Role:      role,
				Scope:     tick.Symbol,
				Epoch:     value.Epoch,
				Tick:      value.Tick,
				SeqIdx:    seq,
				Timestamp: value.Timestamp,
			}
			out.FormatPrefix()
			return out
		}

		for _, trade := range tick.Trades {
			if trade.TradeID <= 0 {
				return nil, errnie.Err(errnie.Validation, "audit: "+value.Prefix+" carries a "+tick.Symbol+" trade without trade_id", nil)
			}

			out := header("trade", "trade", uint64(trade.TradeID))
			data.AppendVector(out, []float64{
				trade.Price.Float64(), trade.Qty, float64(trade.Timestamp.UnixNano()),
			}, "price", "qty", "at")
			view.Trades = append(view.Trades, out)
		}

		labels, values := touch(tick.Price)

		view.Price = header(priceSource, "price", value.SeqIdx)
		data.AppendVector(view.Price, values, labels...)

		view.Join = header(data.TickOrigin, "runtime:join", value.SeqIdx)

		for _, signal := range tick.Signals {
			out := header(signal.Source, "signal", value.SeqIdx)
			data.AppendVector(out, signal.Values, signal.Labels...)
			view.Signals = append(view.Signals, out)

			data.AppendSignalVector(view.Join, signal.Source, tick.Symbol, signal.Values, append(signal.Labels, signal.Tags...)...)
		}

		data.AppendVector(view.Join, values, append(labels, touchTag)...)
		views = append(views, view)
	}

	return views, nil
}

/*
touch is the price layer as labelled values; what the tick did not record
is absent, never zero.
*/
func touch(price broker.PriceLayer) ([]string, []float64) {
	labels := make([]string, 0, 3)
	values := make([]float64, 0, 3)

	if price.Bid != nil {
		labels, values = append(labels, "best_price:bid"), append(values, price.Bid.Float64())
	}

	if price.Ask != nil {
		labels, values = append(labels, "best_price:ask"), append(values, price.Ask.Float64())
	}

	if price.FeeRate != nil {
		labels, values = append(labels, "fee_rate"), append(values, *price.FeeRate)
	}

	return labels, values
}

/*
Touch returns the bid and ask a join recorded for its symbol; ok is false
when the tick recorded either side as absent.
*/
func Touch(join *data.Value) (bid, ask float64, ok bool) {
	head := join.Prefix + "/"

	for index, values := range data.Indexed[[]float64](join, head) {
		rest, isTouch := strings.CutPrefix(index, head)

		if !isTouch || !strings.HasSuffix(rest, "/"+touchTag) {
			continue
		}

		labels := strings.Split(rest, "/")
		bidIdx := slices.Index(labels, "best_price:bid")
		askIdx := slices.Index(labels, "best_price:ask")

		if bidIdx < 0 || askIdx < 0 || bidIdx >= len(values) || askIdx >= len(values) {
			return 0, 0, false
		}

		return values[bidIdx], values[askIdx], true
	}

	return 0, 0, false
}
