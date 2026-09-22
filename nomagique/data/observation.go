package data

import (
	"encoding/json"
	"github.com/theapemachine/symm/nomagique/temporal"
	"strconv"
)

/* Interval carries one metric's actual observation interval, never a filled gap. */
type Interval struct {
	ID      string              `json:"id"`
	From    temporal.TapeCursor `json:"from"`
	To      temporal.TapeCursor `json:"to"`
	Delta   float64             `json:"delta"`
	RMS     float64             `json:"rms"`
	Signal  float64             `json:"signal"`
	Noise   float64             `json:"noise"`
	Support uint64              `json:"support"`
}

/* Observation is the causal information available at one instrument record. */
type Observation struct {
	Session  string                     `json:"session"`
	Endpoint string                     `json:"endpoint"`
	Symbol   string                     `json:"symbol"`
	Cursor   temporal.TapeCursor        `json:"cursor"`
	Metrics  []Interval                 `json:"metrics"`
	Quote    map[string]json.RawMessage `json:"quote"`
}

func (observation Observation) Stream() string {
	return "[" + strconv.Quote(observation.Session) + "," + strconv.Quote(observation.Endpoint) + "," + strconv.Quote(observation.Symbol) + "]"
}
func (observation Observation) Instrument() string {
	return "[" + strconv.Quote(observation.Endpoint) + "," + strconv.Quote(observation.Symbol) + "]"
}
