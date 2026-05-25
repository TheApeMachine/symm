package ui

import (
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
)

/*
CandleQueue mirrors Kraken trade ticks into OHLCV candle_bar frames on ui.
*/
type CandleQueue struct {
	stream *CandleStream
	watch  *ChartWatch
	ui     *qpool.BroadcastGroup
}

/*
NewCandleQueue builds a source-side candle mirror for watched chart symbols.
*/
func NewCandleQueue(
	uiGroup *qpool.BroadcastGroup,
	chartWatch *ChartWatch,
	interval time.Duration,
) (*CandleQueue, error) {
	if uiGroup == nil {
		return nil, fmt.Errorf("candle queue requires ui broadcast group")
	}

	if chartWatch == nil {
		return nil, fmt.Errorf("candle queue requires chart watch")
	}

	stream, err := NewCandleStream(interval)

	if err != nil {
		return nil, err
	}

	return &CandleQueue{
		stream: stream,
		watch:  chartWatch,
		ui:     uiGroup,
	}, nil
}

/*
Close releases candle mirror resources.
*/
func (candleQueue *CandleQueue) Close() {}

/*
PublishTradeUpdate emits candle_bar frames for watched symbols from one trade batch.
It is called synchronously from the Kraken websocket trade handler.
*/
func (candleQueue *CandleQueue) PublishTradeUpdate(
	update engine.TradeUpdate,
) error {
	for _, tick := range update.Ticks {
		if err := candleQueue.handleTradeTick(update.Symbol, tick); err != nil {
			return err
		}
	}

	return nil
}

/*
PublishTickerUpdate emits candle_bar frames for watched symbols from one ticker row.
It is called synchronously from the Kraken websocket ticker handler.
*/
func (candleQueue *CandleQueue) PublishTickerUpdate(
	symbol string,
	last float64,
	timestamp string,
) error {
	if !candleQueue.watch.Has(symbol) {
		return nil
	}

	candleBar, err := candleQueue.stream.ObserveTicker(symbol, last, timestamp)

	if err != nil {
		return err
	}

	Publish(candleQueue.ui, "candle_bar", candleBar.Payload())

	return nil
}

func (candleQueue *CandleQueue) handleTradeTick(
	updateSymbol string,
	tick market.TradeTick,
) error {
	if tick.Symbol == "" {
		return fmt.Errorf("candle queue trade tick missing symbol")
	}

	if updateSymbol != "" && updateSymbol != tick.Symbol {
		return fmt.Errorf(
			"candle queue trade symbol mismatch: batch=%s tick=%s",
			updateSymbol,
			tick.Symbol,
		)
	}

	if !candleQueue.watch.Has(tick.Symbol) {
		return nil
	}

	candleBar, err := candleQueue.stream.ObserveTrade(
		tick.Symbol,
		tick.Price,
		tick.Volume,
		tick.Timestamp,
	)

	if err != nil {
		return err
	}

	Publish(candleQueue.ui, "candle_bar", candleBar.Payload())

	return nil
}
