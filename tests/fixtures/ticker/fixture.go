package ticker

import (
	"embed"
	"iter"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

/*
Fixture replays an ordered ticker payload sequence for market and broker tests.
The sequence is materialized once so Generate remains deterministic.
*/
type Fixture struct {
	sequence [][]byte
	template []byte
	signal   *marketsignal.Signal
	typ      FixtureType
}

/*
NewFixture loads a Kraken ticker payload and expands updates over the requested
horizon while snapshots remain a single exchange frame.
*/
func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
	}

	if typ == SNAPSHOT {
		return &Fixture{sequence: [][]byte{raw}}
	}

	return (&Fixture{}).sequenceUpdates(raw, horizon)
}

/*
NewMarket creates a ticker fixture that injects shared market states into the
Kraken update template for every simulated symbol.
*/
func NewMarket(_ []string, signal *marketsignal.Signal) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
	}

	return &Fixture{
		template: raw,
		signal:   signal,
		typ:      SNAPSHOT,
	}
}

/*
Generate yields the materialized Kraken ticker payloads in order.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	if fixture.signal != nil {
		return func(yield func([]byte) bool) {
			for samples := range fixture.signal.Generate() {
				if !yield(fixture.render(samples)) {
					return
				}
			}
		}
	}

	return func(yield func([]byte) bool) {
		for _, payload := range fixture.sequence {
			if !yield(payload) {
				return
			}
		}
	}
}

/*
render injects the current state into the Kraken ticker template.
*/
func (fixture *Fixture) render(samples []marketsignal.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture decode failed", err))
	}

	rows := make([]map[string]any, len(samples))
	for index, sample := range samples {
		row := map[string]any{}

		for key, value := range payload["data"].([]any)[0].(map[string]any) {
			row[key] = value
		}

		opening := sample.Statistics.Open
		change := sample.TradePrice - opening
		bid, bidQty := touch(sample.Bids, "buy")
		ask, askQty := touch(sample.Asks, "sell")
		row["symbol"] = sample.Symbol
		row["bid"] = bid
		row["bid_qty"] = bidQty
		row["ask"] = ask
		row["ask_qty"] = askQty
		row["last"] = sample.TradePrice
		row["volume"] = sample.Statistics.Volume
		row["vwap"] = sample.Statistics.VWAP
		row["low"] = sample.Statistics.Low
		row["high"] = sample.Statistics.High
		row["change"] = change
		row["change_pct"] = change / opening * 100
		row["timestamp"] = sample.At
		rows[index] = row
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture encode failed", err))
	}

	fixture.typ = UPDATE

	return encoded
}

/*
touch aggregates every authoritative order resting at the best price.
*/
func touch(orders []marketsignal.Order, side string) (float64, float64) {
	price := orders[0].Price

	for _, order := range orders[1:] {
		if side == "buy" && order.Price > price || side == "sell" && order.Price < price {
			price = order.Price
		}
	}

	quantity := 0.0

	for _, order := range orders {
		if order.Price == price {
			quantity += order.Qty
		}
	}

	return price, quantity
}

/*
sequenceUpdates materializes the standalone fixture sequence.
*/
func (fixture *Fixture) sequenceUpdates(raw []byte, horizon int) *Fixture {
	if horizon < 1 {
		panic(errnie.Err(errnie.Validation, "ticker fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, horizon)

	for index := range horizon {
		fixture.sequence[index] = fixture.advance(base, index)
	}

	return fixture
}

/*
advance renders one standalone ticker update from its fixture template.
*/
func (fixture *Fixture) advance(base map[string]any, index int) []byte {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture clone failed", err))
	}

	var frame map[string]any

	if err := sonic.Unmarshal(raw, &frame); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture clone decode failed", err))
	}

	for _, entry := range frame["data"].([]any) {
		row := entry.(map[string]any)
		multiplier := 1 + 0.001*float64(index+1)

		for _, field := range []string{"bid", "ask", "last", "vwap", "low", "high"} {
			row[field] = row[field].(float64) * multiplier
		}

		row["volume"] = row["volume"].(float64) + 10*float64(index+1)
		observed, parseErr := time.Parse(time.RFC3339Nano, row["timestamp"].(string))

		if parseErr != nil {
			panic(errnie.Err(errnie.Validation, "ticker fixture timestamp parse failed", parseErr))
		}

		row["timestamp"] = observed.Add(time.Duration(index+1) * 5 * time.Second).Format(time.RFC3339Nano)
	}

	encoded, err := sonic.Marshal(frame)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture encode failed", err))
	}

	return encoded
}
