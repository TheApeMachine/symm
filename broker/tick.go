package broker

import (
	"errors"
	"fmt"
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
	Signals    []data.Signal
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

	if err := data.CheckTickLayers(value); err != nil {
		return nil, errnie.Error(err)
	}

	layer, err := tradeLayer(value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	trades, err := data.AttributeOk[[]kraken.TradeData](layer, "data")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "broker: trade layer of "+value.Prefix+" does not decode", err,
		))
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

		raw := SymbolLayer(value, data.LayerPrice, symbol)

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

		if raw = SymbolLayer(value, data.LayerInstrument, symbol); raw != nil {
			if err := sonic.Unmarshal(raw, &tick.Instrument); err != nil {
				return nil, errnie.Error(errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
					"broker: instrument/%s of %s does not decode", symbol, value.Prefix,
				), err))
			}
		}

		ticks = append(ticks, tick)
		bySymbol[symbol] = tick
	}

	for signal, err := range data.Signals(value) {
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
	layer, err := tradeLayer(value)

	if err != nil {
		return nil, err
	}

	trades, err := data.AttributeOk[[]kraken.TradeData](layer, "data")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "broker: trade layer does not decode", err,
		))
	}

	return Symbols(trades), nil
}

/*
tradeLayer returns the tick's single "spot:trade/<type>" layer.
*/
func tradeLayer(value *data.Value) ([]byte, error) {
	var found []byte

	for index, layer := range data.Indexed[[]byte](value, data.LayerTrade+"/") {
		if !strings.HasPrefix(index, data.LayerTrade+"/") {
			continue
		}

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
