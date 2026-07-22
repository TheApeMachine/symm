package book

import (
	"embed"
	"encoding/json"
	"hash/crc32"
	"iter"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
FixtureType selects the Kraken Level2 envelope represented by a fixture.
*/
type FixtureType string

const (
	SNAPSHOT      FixtureType = "snapshot"
	UPDATE        FixtureType = "update"
	checksumDepth             = 10
)

/*
Fixture yields template-backed standalone or market-driven Level2 frames.
*/
type Fixture struct {
	horizon  int
	sequence [][]byte
	template []byte
	signal   *marketsignal.Signal
	previous map[string]map[string]any
	typ      FixtureType
}

/*
NewDecoderFixture repeats the embedded wire examples for parser tests. Dynamic
market tests use NewMarket, whose checksums are derived from authoritative book
state; this constructor does not claim its sequenced updates form a venue tape.
*/
func NewDecoderFixture(typ FixtureType, horizon int) *Fixture {
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
		previous: make(map[string]map[string]any, len(symbols)),
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

/*
Generate yields ready Kraken Level2 payloads in deterministic order.
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

	rows := make([]map[string]any, 0, len(samples))
	template := payload["data"].([]any)[0].(map[string]any)

	for _, sample := range samples {
		if fixture.typ == UPDATE && !sample.BookChanged {
			continue
		}

		row := clone(template)
		row["symbol"] = sample.Symbol
		fixture.inject(row, "bids", sample.Bids)
		fixture.inject(row, "asks", sample.Asks)
		row["timestamp"] = sample.At
		row["checksum"] = fixture.checksum(row)
		current := clone(row)

		if fixture.typ == UPDATE {
			row = fixture.delta(fixture.previous[sample.Symbol], current)
		}

		rows = append(rows, row)
		fixture.previous[sample.Symbol] = current
	}

	if len(rows) == 0 {
		return nil
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := json.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture encode failed", err))
	}

	fixture.typ = UPDATE

	return encoded
}

/*
inject aggregates the authoritative Level3 orders into Kraken L2 price levels.
*/
func (fixture *Fixture) inject(
	row map[string]any,
	side string,
	orders []marketsignal.Order,
) {
	aggregated := make(map[float64]float64, len(orders))

	for _, order := range orders {
		aggregated[order.Price] += order.Qty
	}

	prices := make([]float64, 0, len(aggregated))

	for price := range aggregated {
		prices = append(prices, price)
	}

	sort.Float64s(prices)

	if side == "bids" {
		sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
	}

	levels := make([]any, len(prices))

	for index, price := range prices {
		levels[index] = map[string]any{"price": price, "qty": round(aggregated[price])}
	}

	row[side] = levels
}

/*
delta emits only changed and removed L2 price levels from two coherent states.
*/
func (fixture *Fixture) delta(
	previous map[string]any,
	current map[string]any,
) map[string]any {
	update := map[string]any{
		"symbol":    current["symbol"],
		"timestamp": current["timestamp"],
		"checksum":  current["checksum"],
	}

	for _, side := range []string{"bids", "asks"} {
		prior := map[float64]float64{}

		for _, entry := range previous[side].([]any) {
			level := entry.(map[string]any)
			prior[number(level, "price")] = number(level, "qty")
		}

		changes := make([]any, 0)

		for _, entry := range current[side].([]any) {
			level := entry.(map[string]any)
			price := number(level, "price")
			quantity := number(level, "qty")

			if prior[price] != quantity {
				changes = append(changes, level)
			}

			delete(prior, price)
		}

		removed := make([]float64, 0, len(prior))

		for price := range prior {
			removed = append(removed, price)
		}

		sort.Float64s(removed)

		if side == "bids" {
			sort.Sort(sort.Reverse(sort.Float64Slice(removed)))
		}

		for _, price := range removed {
			changes = append(changes, map[string]any{"price": price, "qty": 0.0})
		}

		update[side] = changes
	}

	return update
}

/*
checksum derives Kraken's CRC over the ten best asks followed by the ten best
bids represented in the generated payload.
*/
func (fixture *Fixture) checksum(row map[string]any) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		for index, entry := range row[side].([]any) {
			if index == checksumDepth {
				break
			}

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
