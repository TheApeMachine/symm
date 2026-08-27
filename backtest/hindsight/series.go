package hindsight

import (
	"encoding/json"
	"fmt"
	"time"
)

/*
Point is one executable price observation on a symbol's tape, reduced from the
venue's trade stream with concurrent market economics (bid, ask, spread, friction).
*/
type Point struct {
	At       time.Time `json:"at"`
	Price    float64   `json:"price"`
	Qty      float64   `json:"qty,omitempty"`
	Bid      float64   `json:"bid,omitempty"`
	Ask      float64   `json:"ask,omitempty"`
	Spread   float64   `json:"spread,omitempty"`
	Friction float64   `json:"friction,omitempty"`
}

/*
Series is the reduced tape for one symbol.
*/
type Series struct {
	Symbol string  `json:"symbol"`
	Points []Point `json:"points"`
}

/*
PriceAt returns the price at or nearest before `at`, or 0 if empty.
*/
func (series *Series) PriceAt(at time.Time) float64 {
	if series == nil || len(series.Points) == 0 {
		return 0
	}

	for index := len(series.Points) - 1; index >= 0; index-- {
		if !series.Points[index].At.After(at) {
			return series.Points[index].Price
		}
	}

	return series.Points[0].Price
}

/*
PointAt returns the point at or nearest before `at`, or empty Point.
*/
func (series *Series) PointAt(at time.Time) Point {
	if series == nil || len(series.Points) == 0 {
		return Point{}
	}

	for index := len(series.Points) - 1; index >= 0; index-- {
		if !series.Points[index].At.After(at) {
			return series.Points[index]
		}
	}

	return series.Points[0]
}

/*
TapeFrame is the wire shape of one captured trade update.
*/
type TapeFrame struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    []Row  `json:"data"`
}

/*
Row is one trade print on the wire.
*/
type Row struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
	Timestamp string  `json:"timestamp"`
}

/*
TickerPayload is the wire shape of one captured ticker update.
*/
type TickerPayload struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []TickerRow `json:"data"`
}

/*
TickerRow carries top of book quotes.
*/
type TickerRow struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Last   float64 `json:"last"`
}

type marketEconomics struct {
	bid      float64
	ask      float64
	spread   float64
	friction float64
}

/*
Reducer collects trade prints and market quotes into per-symbol price series.
*/
type Reducer struct {
	series map[string]*Series
	market map[string]marketEconomics
}

/*
NewReducer creates an empty tape reducer.
*/
func NewReducer() *Reducer {
	return &Reducer{
		series: map[string]*Series{},
		market: map[string]marketEconomics{},
	}
}

/*
Ingest absorbs one captured payload and records trade prints with market economics.
*/
func (reducer *Reducer) Ingest(payload []byte, frameAt time.Time) error {
	var probe struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
	}

	if err := json.Unmarshal(payload, &probe); err != nil {
		return nil
	}

	if probe.Channel == "ticker" {
		return reducer.ingestTicker(payload)
	}

	if probe.Channel == "trade" && (probe.Type == "update" || probe.Type == "snapshot") {
		return reducer.ingestTrade(payload, frameAt)
	}

	return nil
}

func (reducer *Reducer) ingestTicker(payload []byte) error {
	var frame TickerPayload

	if err := json.Unmarshal(payload, &frame); err != nil {
		return fmt.Errorf("hindsight: decode ticker frame: %w", err)
	}

	for _, row := range frame.Data {
		if row.Symbol == "" || row.Bid <= 0 || row.Ask <= 0 {
			continue
		}

		mid := (row.Bid + row.Ask) / 2
		spread := 0.0

		if mid > 0 {
			spread = (row.Ask - row.Bid) / mid
		}

		reducer.market[row.Symbol] = marketEconomics{
			bid:      row.Bid,
			ask:      row.Ask,
			spread:   spread,
			friction: spread,
		}
	}

	return nil
}

func (reducer *Reducer) ingestTrade(payload []byte, frameAt time.Time) error {
	var frame TapeFrame

	if err := json.Unmarshal(payload, &frame); err != nil {
		return fmt.Errorf("hindsight: decode trade frame: %w", err)
	}

	for _, row := range frame.Data {
		if row.Symbol == "" || row.Price <= 0 || row.Qty <= 0 {
			continue
		}

		entry := reducer.series[row.Symbol]

		if entry == nil {
			entry = &Series{Symbol: row.Symbol}
			reducer.series[row.Symbol] = entry
		}

		market := reducer.market[row.Symbol]
		bid := market.bid
		ask := market.ask
		spread := market.spread
		friction := market.friction

		if bid == 0 {
			bid = row.Price
		}

		if ask == 0 {
			ask = row.Price
		}

		entry.Points = append(entry.Points, Point{
			At:       timeOf(row, frameAt),
			Price:    row.Price,
			Qty:      row.Qty,
			Bid:      bid,
			Ask:      ask,
			Spread:   spread,
			Friction: friction,
		})
	}

	return nil
}

func timeOf(row Row, frameAt time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, row.Timestamp); err == nil {
		return parsed
	}

	return frameAt
}

/*
Symbols returns every series the reducer has seen so far.
*/
func (reducer *Reducer) Symbols() []*Series {
	out := make([]*Series, 0, len(reducer.series))

	for _, s := range reducer.series {
		if len(s.Points) > 0 {
			out = append(out, s)
		}
	}

	return out
}

/*
SeriesFor returns the reduced series for one symbol, or nil.
*/
func (reducer *Reducer) SeriesFor(symbol string) *Series {
	return reducer.series[symbol]
}
