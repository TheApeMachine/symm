package book

import (
	"embed"
	"hash/crc32"
	"iter"
	"math"
	"strconv"
	"strings"
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

type Fixture struct {
	horizon  int
	sequence [][]byte
	template []byte
	signal   *marketsignal.Signal
	previous map[string]float64
	typ      FixtureType
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	return fixture.sequencer(raw)
}

/*
NewMarket creates a book fixture that renders both sides of every simulated
symbol from each shared market state.
*/
func NewMarket(symbols []string, signal *marketsignal.Signal) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture load failed", err))
	}

	return &Fixture{
		template: raw,
		signal:   signal,
		previous: make(map[string]float64, len(symbols)),
		typ:      SNAPSHOT,
	}
}

func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "book fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			advanceLevels(row, "bids", i, -1)
			advanceLevels(row, "asks", i, 1)
			row["checksum"] = uint64(number(row, "checksum")) + uint64(i+1)
			row["timestamp"] = advance(row["timestamp"].(string), time.Duration(i+1)*250*time.Millisecond)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "book fixture marshal failed", err))
		}

		fixture.sequence[i] = marshaled
	}

	return fixture
}

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
		for _, seq := range fixture.sequence {
			if !yield(seq) {
				return
			}
		}
	}
}

/*
render injects the current state into the Kraken book template.
*/
func (fixture *Fixture) render(samples []marketsignal.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture decode failed", err))
	}

	rows := make([]map[string]any, len(samples))
	template := payload["data"].([]any)[0].(map[string]any)

	for index, sample := range samples {
		row := clone(template)
		bid := sample.Price - marketsignal.PriceIncrement
		ask := sample.Price + marketsignal.PriceIncrement
		increment := marketsignal.PriceIncrement
		quantity := sample.Volume * 100

		row["symbol"] = sample.Symbol
		fixture.inject(row, "bids", bid, -increment, quantity)
		fixture.inject(row, "asks", ask, increment, quantity)
		row["timestamp"] = sample.At
		row["checksum"] = fixture.checksum(row)

		if fixture.typ == UPDATE {
			previous := clone(template)
			fixture.inject(
				previous,
				"bids",
				fixture.previous[sample.Symbol]-marketsignal.PriceIncrement,
				-marketsignal.PriceIncrement,
				quantity,
			)
			fixture.inject(
				previous,
				"asks",
				fixture.previous[sample.Symbol]+marketsignal.PriceIncrement,
				marketsignal.PriceIncrement,
				quantity,
			)

			for _, side := range []string{"bids", "asks"} {
				deletes := previous[side].([]any)

				for _, entry := range deletes {
					entry.(map[string]any)["qty"] = 0.0
				}

				row[side] = append(deletes, row[side].([]any)...)
			}
		}

		rows[index] = row
		fixture.previous[sample.Symbol] = sample.Price
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture encode failed", err))
	}

	fixture.typ = UPDATE

	return encoded
}

/*
inject moves one complete template side while retaining its depth and relative
liquidity distribution.
*/
func (fixture *Fixture) inject(
	row map[string]any,
	side string,
	price float64,
	increment float64,
	quantity float64,
) {
	levels := row[side].([]any)
	scale := quantity / number(levels[0].(map[string]any), "qty")

	for index, entry := range levels {
		level := entry.(map[string]any)
		level["price"] = round(price + increment*float64(index))
		level["qty"] = round(number(level, "qty") * scale)
	}
}

/*
checksum derives Kraken's CRC over the ten best asks followed by the ten best
bids represented in the generated payload.
*/
func (fixture *Fixture) checksum(row map[string]any) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		for _, entry := range row[side].([]any) {
			level := entry.(map[string]any)
			checksum = fixture.write(checksum, number(level, "price"))
			checksum = fixture.write(checksum, number(level, "qty"))
		}
	}

	return checksum
}

/*
write adds one emitted Kraken decimal to the rolling book CRC.
*/
func (fixture *Fixture) write(checksum uint32, value float64) uint32 {
	text := strings.TrimLeft(
		strings.ReplaceAll(strconv.FormatFloat(value, 'f', -1, 64), ".", ""),
		"0",
	)

	return crc32.Update(checksum, crc32.IEEETable, []byte(text))
}

func advanceLevels(row map[string]any, side string, step int, direction float64) {
	levels := row[side].([]any)

	for i, raw := range levels {
		level := raw.(map[string]any)
		level["price"] = round(number(level, "price") + direction*0.0001*float64(step+1+i))
		level["qty"] = round(math.Max(number(level, "qty")*(1+0.01*float64(step+1)), 0))
	}
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture clone decode failed", err))
	}

	return out
}

func rows(frame map[string]any) []map[string]any {
	out := []map[string]any{}

	for _, item := range frame["data"].([]any) {
		out = append(out, item.(map[string]any))
	}

	return out
}

func number(row map[string]any, key string) float64 {
	return row[key].(float64)
}
