package kraken

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

/*
TickerRow is one pair from GET /0/public/Ticker. Arrays are [today, last 24h]
except ask/bid/last (touch) and o (today's open).
*/
type TickerRow struct {
	Ask    []string `json:"a"`
	Bid    []string `json:"b"`
	Last   []string `json:"c"`
	Volume []string `json:"v"`
	VWAP   []string `json:"p"`
	Trades []int    `json:"t"`
	Low    []string `json:"l"`
	High   []string `json:"h"`
	Open   string   `json:"o"`
}

type Ticker struct {
	Error    []any                `json:"error"`
	Result   map[string]TickerRow `json:"result"`
	onChange func(market.MarketType)
}

func NewTicker() *Ticker {
	return &Ticker{Result: make(map[string]TickerRow)}
}

func (ticker *Ticker) OnChange(fn func(market.MarketType)) {
	ticker.onChange = fn
}

func (ticker *Ticker) Request() map[string]any {
	return map[string]any{}
}

func (ticker *Ticker) MarshalJSON() ([]byte, error) {
	type alias Ticker
	return sonic.Marshal((*alias)(ticker))
}

func (ticker *Ticker) UnmarshalJSON(data []byte) error {
	type alias Ticker
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] invalid ticker frame",
			err,
		)
	}

	if len(decoded.Error) > 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] rejected",
			fmt.Errorf("%v", decoded.Error),
		)
	}

	if len(decoded.Result) == 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] empty result",
			nil,
		)
	}

	onChange := ticker.onChange
	*ticker = Ticker(decoded)
	ticker.onChange = onChange

	if ticker.Result == nil {
		ticker.Result = make(map[string]TickerRow)
	}

	if ticker.onChange != nil {
		ticker.onChange(ticker)
	}

	return nil
}

func (ticker *Ticker) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Ticker)

	if !ok || book == nil || ticker == nil {
		return other
	}

	rows := make(map[string]*market.TickerData, len(ticker.Result))

	for pair, row := range ticker.Result {
		rows[pair] = &market.TickerData{
			Symbol:    pair,
			VolumeUSD: row.QuoteVolume24h(),
			ChangePct: row.ChangePct(),
			High24h:   row.High24h(),
			Low24h:    row.Low24h(),
			Last:      row.LastTrade(),
		}
	}

	return market.NewTicker(rows).ToMarket(book)
}

func (ticker *Ticker) FromMarket(other market.MarketType) market.MarketType {
	if _, ok := other.(*market.Ticker); !ok || ticker == nil {
		return ticker
	}

	return ticker
}

func ParseTicker(buf []byte) (map[string]TickerRow, error) {
	ticker := NewTicker()

	if err := ticker.UnmarshalJSON(buf); err != nil {
		return nil, err
	}

	return ticker.Result, nil
}

func (row TickerRow) QuoteVolume24h() float64 {
	base := atIndex(row.Volume, 1)
	vwap := atIndex(row.VWAP, 1)

	if base <= 0 || vwap <= 0 {
		return 0
	}

	return base * vwap
}

func (row TickerRow) ChangePct() float64 {
	open := parseFloat(row.Open)
	last := row.LastTrade()

	if open <= 0 || last <= 0 {
		return 0
	}

	return (last - open) / open * 100
}

func (row TickerRow) LastTrade() float64 { return atIndex(row.Last, 0) }
func (row TickerRow) High24h() float64   { return atIndex(row.High, 1) }
func (row TickerRow) Low24h() float64    { return atIndex(row.Low, 1) }

func atIndex(vals []string, i int) float64 {
	if i < 0 || i >= len(vals) {
		return 0
	}

	return parseFloat(vals[i])
}

func parseFloat(raw string) float64 {
	if raw == "" {
		return 0
	}

	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}

	return v
}
