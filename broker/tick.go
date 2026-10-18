package broker

import (
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

/*
SymbolTick is one symbol's state as a stored tick recorded it: the trades of
the frame for the symbol, its price layer, its book (or the tick's record of
why the book was not valid), and every signal layer written for it.
*/
type SymbolTick struct {
	Symbol     string
	Trades     []kraken.TradeData
	Price      PriceLayer
	Book       *Snapshot
	BookErr    error
	Instrument kraken.InstrumentPair
	Signals    []Sample
}

/*
Sample is one signal vector on a tick: who wrote it, for which symbol, and
the labelled floats. The address is signal/<source>/<labels>/symbol:<symbol>.
*/
type Sample struct {
	Source string
	Symbol string
	Labels []string
	Values []float64
	Tags   []string
}

/*
Metric returns the value labelled label.
*/
func (sample Sample) Metric(label string) (float64, error) {
	for idx, name := range sample.Labels {
		if name == label {
			return sample.Values[idx], nil
		}
	}

	return 0, errnie.Err(errnie.NotFound, fmt.Sprintf(
		"broker: %s signal for %s has no %s", sample.Source, sample.Symbol, label,
	), data.ErrMissing)
}

/*
ReadTick decodes a stored tick (data.TickPrefix) into its symbols, in the
order they first trade in the frame. A tick without a decodable trade layer,
a traded symbol without a decodable price layer, a book layer that does not
decode, or a signal layer for a symbol the frame did not trade is an error.
A book the tick records as not valid is kept in BookErr (wrapping
ErrBookNotValid), so a reader skips it on the tick's explicit record.
*/
func ReadTick(value *data.Value) ([]*SymbolTick, error) {
	if value == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "broker: no tick to read", nil,
		))
	}

	trades, err := Trades(value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	ticks := make([]*SymbolTick, 0, 1)
	bySymbol := make(map[string]*SymbolTick)

	for _, symbol := range Symbols(trades) {
		tick := &SymbolTick{Symbol: symbol}

		for _, trade := range trades {
			if trade.Symbol == symbol {
				tick.Trades = append(tick.Trades, trade)
			}
		}

		raw := SymbolLayer(value, "price", symbol)

		if raw == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.NotFound, fmt.Sprintf("broker: %s has no price/%s layer", value.Prefix, symbol), nil,
			))
		}

		if err := sonic.Unmarshal(raw, &tick.Price); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, fmt.Sprintf("broker: price/%s of %s does not decode", symbol, value.Prefix), err,
			))
		}

		tick.Book, err = ReadSnapshot(value, symbol)

		if errors.Is(err, ErrBookNotValid) {
			tick.BookErr, err = err, nil
		}

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "broker: book of "+value.Prefix, err,
			))
		}

		if raw = SymbolLayer(value, "instrument", symbol); raw != nil {
			if err := sonic.Unmarshal(raw, &tick.Instrument); err != nil {
				return nil, errnie.Error(errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
					"broker: instrument/%s of %s does not decode", symbol, value.Prefix,
				), err))
			}
		}

		ticks = append(ticks, tick)
		bySymbol[symbol] = tick
	}

	for signal, err := range Samples(value) {
		if err != nil {
			return nil, errnie.Error(err)
		}

		tick, ok := bySymbol[signal.Symbol]

		if !ok {
			return nil, errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf(
				"broker: %s carries a %s signal for %s, which the frame did not trade",
				value.Prefix, signal.Source, signal.Symbol,
			), nil,
			))
		}

		tick.Signals = append(tick.Signals, signal)
	}

	return ticks, nil
}

/*
TradedSymbols returns the symbols that appear in the tick's trade frame, in
first-trade order. Aggregate ticks carry no single Scope symbol; callers that
need an instrument for paper or policy use this (and dashboard focus) instead.
*/
func TradedSymbols(value *data.Value) ([]string, error) {
	trades, err := Trades(value)

	if err != nil {
		return nil, err
	}

	return Symbols(trades), nil
}

/*
Trades returns the frame's trades. Append stores the Kraken JSON as a gob
[]byte under spot:trade, so the JSON is the decoded layer, not Layers[0].
*/
func Trades(value *data.Value) ([]kraken.TradeData, error) {
	layer, err := tradeLayer(value)

	if err != nil {
		return nil, err
	}

	trades, err := data.AttributeOk[[]kraken.TradeData](layer, "data")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "broker: trade layer of "+value.Prefix+" does not decode", err,
		))
	}

	return trades, nil
}

/*
tradeLayer returns the tick's single "spot:trade/<type>" layer.
*/
func tradeLayer(value *data.Value) ([]byte, error) {
	var found []byte

	for _, layer := range data.Layer[[]byte](value, "spot:trade") {
		if found != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "broker: "+value.Prefix+" carries more than one trade layer", nil,
			))
		}

		found = layer
	}

	if found == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "broker: "+value.Prefix+" has no spot:trade layer", nil,
		))
	}

	return found, nil
}

/*
samples yields every signal layer. The address is
signal/<source>/<labels...>/<tags...>/symbol:<symbol>.
*/
func Samples(value *data.Value) iter.Seq2[Sample, error] {
	return func(yield func(Sample, error) bool) {
		if value == nil {
			return
		}

		for index, values := range data.Layer[[]float64](value, "signal") {
			sample, err := parseSample(index, values)

			if !yield(sample, err) {
				return
			}
		}
	}
}

func parseSample(index string, values []float64) (Sample, error) {
	var sample Sample

	at := strings.Index(index, "/signal/")

	if at < 0 && strings.HasPrefix(index, "signal/") {
		at = 0
	}

	if at < 0 {
		return sample, errnie.Err(errnie.Validation, "broker: "+index+" is not a signal layer", nil)
	}

	rest := index[at:]
	rest = strings.TrimPrefix(rest, "/")
	rest = strings.TrimPrefix(rest, "signal/")

	tagAt := strings.LastIndex(rest, "/symbol:")

	if tagAt < 0 || tagAt+len("/symbol:") == len(rest) {
		return sample, errnie.Err(errnie.Validation, "broker: "+index+" has no symbol", nil)
	}

	symbol := rest[tagAt+len("/symbol:"):]
	parts := strings.Split(rest[:tagAt], "/")

	if len(parts) == 0 || parts[0] == "" {
		return sample, errnie.Err(errnie.Validation, "broker: "+index+" has no source", nil)
	}

	sample.Source = parts[0]
	sample.Symbol = symbol
	parts = parts[1:]

	if len(values) > len(parts) {
		return sample, errnie.Err(errnie.Validation, fmt.Sprintf(
			"broker: %s has %d values for %d labels", index, len(values), len(parts),
		), nil)
	}

	sample.Values = values
	sample.Labels = parts[:len(values)]
	sample.Tags = parts[len(values):]

	return sample, nil
}
