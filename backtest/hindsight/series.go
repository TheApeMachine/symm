package hindsight

import (
	"encoding/json"
	"fmt"
	"time"
)

/*
Point is one executable price observation on a symbol's tape, reduced from the
venue's trade stream. Trades are the honest microstructure: each print is a
real executed price at a real moment, so a hindsight series built from them
contains no reconstruction fiction about what was tradable.
*/
type Point struct {
	At    time.Time `json:"at"`
	Price float64   `json:"price"`
	Qty   float64   `json:"qty,omitempty"`
}

/*
Series is the reduced tape for one symbol.
*/
type Series struct {
	Symbol string
	Points []Point
}

/*
TapeFrame is the wire shape of one captured trade update: a batch of print
rows carrying their venue timestamp.
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
isTradeUpdate reports whether a payload is an actual trade data update (as
opposed to a subscribe acknowledgement or a heartbeat), the only frames that
carry priced prints.
*/
func isTradeUpdate(payload []byte) bool {
	var frame TapeFrame

	if err := json.Unmarshal(payload, &frame); err != nil {
		return false
	}

	return frame.Channel == "trade" && frame.Type == "update" && len(frame.Data) > 0
}

/*
timeOf returns the print's venue time when the row carries one, and falls back
to the frame delivery time so ordering never relies on the local clock.
*/
func timeOf(row Row, frameAt time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, row.Timestamp); err == nil {
		return parsed
	}

	return frameAt
}

/*
Reducer collects trade prints into per-symbol price series. It is append-only
and the caller feeds frames in capture order; each print keeps an honest time
from the venue timestamp or the frame delivery time.
*/
type Reducer struct {
	series map[string]*Series
}

/*
NewReducer creates an empty tape reducer.
*/
func NewReducer() *Reducer {
	return &Reducer{series: map[string]*Series{}}
}

/*
Ingest absorbs one captured payload and records any trade prints it carries
with a reliable arrival time. Non-trade payloads are skipped so the capture
feed can be piped through wholesale.
*/
func (reducer *Reducer) Ingest(payload []byte, frameAt time.Time) error {
	if !isTradeUpdate(payload) {
		return nil
	}

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

		entry.Points = append(entry.Points, Point{
			At:    timeOf(row, frameAt),
			Price: row.Price,
			Qty:   row.Qty,
		})
	}

	return nil
}

/*
Symbols returns every series the reducer has seen so far, in whatever order
the frames arrived.
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
