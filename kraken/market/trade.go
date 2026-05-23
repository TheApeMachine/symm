package market

import (
	"errors"
	"fmt"
	"time"

	"github.com/qntfy/jsonparser"
)

var ErrNotTrade = errors.New("not a trade event")

const tradeBatchCapacity = 10

/*
TradeTick is one Kraken v2 trade execution extracted from a websocket frame.
*/
type TradeTick struct {
	Symbol    string
	Price     float64
	Volume    float64
	Side      string
	Timestamp time.Time
}

/*
ParseTrades extracts trade executions from a Kraken v2 trade channel frame.
It scans only the data array and avoids full struct unmarshaling.
*/
func ParseTrades(payload []byte) ([]TradeTick, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return nil, err
	}

	if !isTradeChannel(channel) {
		return nil, fmt.Errorf("%w: channel=%q", ErrNotTrade, channel)
	}

	ticks := make([]TradeTick, 0, tradeBatchCapacity)

	_, err = jsonparser.ArrayEach(payload, func(value []byte, _ jsonparser.ValueType, _ int, err error) {
		if err != nil {
			return
		}

		tick, ok := parseTradeEntry(value)
		if !ok {
			return
		}

		ticks = append(ticks, tick)
	}, "data")
	if err != nil {
		return nil, fmt.Errorf("parse trade data: %w", err)
	}

	return ticks, nil
}

func parseTradeEntry(value []byte) (TradeTick, bool) {
	symbol, err := jsonparser.GetUnsafeString(value, "symbol")
	if err != nil {
		return TradeTick{}, false
	}

	price, err := jsonparser.GetFloat(value, "price")
	if err != nil {
		return TradeTick{}, false
	}

	volume, err := jsonparser.GetFloat(value, "qty")
	if err != nil {
		return TradeTick{}, false
	}

	side, err := jsonparser.GetUnsafeString(value, "side")
	if err != nil {
		return TradeTick{}, false
	}

	timeText, err := jsonparser.GetUnsafeString(value, "timestamp")
	if err != nil {
		return TradeTick{}, false
	}

	timestamp, err := time.Parse(time.RFC3339Nano, timeText)
	if err != nil {
		return TradeTick{}, false
	}

	return TradeTick{
		Symbol:    string(symbol),
		Price:     price,
		Volume:    volume,
		Side:      string(side),
		Timestamp: timestamp,
	}, true
}
