package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
	"github.com/theapemachine/symm/kraken/core"
)

// TickerRow is one element from a ticker channel snapshot/update.
type TickerRow struct {
	Symbol    string  `json:"symbol"`
	Last      float64 `json:"last"`
	Bid       float64 `json:"bid"`
	BidQty    float64 `json:"bid_qty"`
	Ask       float64 `json:"ask"`
	AskQty    float64 `json:"ask_qty"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`
	Volume    float64 `json:"volume"`
	VWAP      float64 `json:"vwap"`
	Timestamp string  `json:"timestamp"`
}

// TickerMessage is a Kraken WebSocket v2 ticker channel message.
type TickerMessage struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []TickerRow `json:"data"`
}

/*
ParseTickerRows extracts ticker rows from a Kraken v2 ticker frame.
*/
func ParseTickerRows(payload []byte) ([]TickerRow, error) {
	channel, err := ChannelName(payload)
	if err != nil {
		return nil, err
	}

	if channel != core.ChannelTicker {
		return nil, fmt.Errorf("not a ticker event: channel=%q", channel)
	}

	rows := make([]TickerRow, 0, 4)

	_, err = jsonparser.ArrayEach(payload, func(value []byte, _ jsonparser.ValueType, _ int, err error) {
		if err != nil {
			return
		}

		row, ok := parseTickerEntry(value)
		if !ok {
			return
		}

		rows = append(rows, row)
	}, "data")
	if err != nil {
		return nil, fmt.Errorf("parse ticker data: %w", err)
	}

	return rows, nil
}

func parseTickerEntry(value []byte) (TickerRow, bool) {
	symbol, err := jsonparser.GetUnsafeString(value, "symbol")
	if err != nil {
		return TickerRow{}, false
	}

	last, err := jsonparser.GetFloat(value, "last")
	if err != nil {
		return TickerRow{}, false
	}

	bid, _ := jsonparser.GetFloat(value, "bid")
	ask, _ := jsonparser.GetFloat(value, "ask")
	changePctPoints, _ := jsonparser.GetFloat(value, "change_pct")
	volume, _ := jsonparser.GetFloat(value, "volume")
	timestamp, _ := jsonparser.GetUnsafeString(value, "timestamp")

	return TickerRow{
		Symbol:    string(symbol),
		Last:      last,
		Bid:       bid,
		Ask:       ask,
		ChangePct: changePctPoints / 100,
		Volume:    volume,
		Timestamp: string(timestamp),
	}, true
}
