package level3

import (
	"embed"
	"encoding/json"
	"hash/crc32"
	"iter"
	"maps"
	"math"
	"slices"
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
	typ      FixtureType
	previous map[string]map[string]any
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	return fixture.sequencer(raw)
}

/*
NewMarket creates a checksum-valid Level3 snapshot fixture for every simulated
symbol and shared market state.
*/
func NewMarket(symbols []string, signal *marketsignal.Signal) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture load failed", err))
	}

	return &Fixture{
		template: raw,
		signal:   signal,
		typ:      SNAPSHOT,
		previous: make(map[string]map[string]any, len(symbols)),
	}
}

func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "level3 fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			advanceLevels(row, "bids", i, -1)
			advanceLevels(row, "asks", i, 1)
			row["checksum"] = uint64(number(row, "checksum")) + uint64(i+1)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "level3 fixture marshal failed", err))
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
Depth returns the number of price levels represented by the Level3 template.
*/
func (fixture *Fixture) Depth() int {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture decode failed", err))
	}

	row := payload["data"].([]any)[0].(map[string]any)

	return max(len(row["bids"].([]any)), len(row["asks"].([]any)))
}

/*
render injects the current state into checksum-valid Kraken Level3 snapshots.
*/
func (fixture *Fixture) render(samples []marketsignal.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture decode failed", err))
	}

	template := payload["data"].([]any)[0].(map[string]any)
	rows := make([]map[string]any, len(samples))

	for index, sample := range samples {
		row := clone(template)

		fixture.inject(row, "bids", sample.Bids)
		fixture.inject(row, "asks", sample.Asks)
		row["symbol"] = sample.Symbol
		row["timestamp"] = sample.At
		row["checksum"] = fixture.checksum(row)
		resting := row

		if fixture.typ == UPDATE {
			row = fixture.update(fixture.previous[sample.Symbol], row)
		}

		fixture.previous[sample.Symbol] = resting
		rows[index] = row
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture encode failed", err))
	}

	fixture.typ = UPDATE

	return encoded
}

/*
inject writes one authoritative resting side into the Kraken JSON template.
*/
func (fixture *Fixture) inject(
	row map[string]any,
	side string,
	orders []marketsignal.Order,
) {
	orders = append([]marketsignal.Order(nil), orders...)
	slices.SortFunc(orders, func(left, right marketsignal.Order) int {
		if side == "bids" {
			return -cmp(left.Price, right.Price)
		}

		return cmp(left.Price, right.Price)
	})
	encoded := make([]any, len(orders))

	for index, source := range orders {
		order := map[string]any{}
		order["order_id"] = source.ID
		order["limit_price"] = json.Number(strconv.FormatFloat(
			source.Price,
			'f',
			8,
			64,
		))
		order["order_qty"] = json.Number(strconv.FormatFloat(
			source.Qty,
			'f',
			8,
			64,
		))
		order["timestamp"] = source.At
		encoded[index] = order
	}

	row[side] = encoded
}

func cmp(left, right float64) int {
	if left < right {
		return -1
	}

	if left > right {
		return 1
	}

	return 0
}

/*
update converts two complete resting states into Kraken delete and add events.
*/
func (fixture *Fixture) update(
	previous map[string]any,
	current map[string]any,
) map[string]any {
	row := map[string]any{
		"symbol":    current["symbol"],
		"timestamp": current["timestamp"],
		"checksum":  current["checksum"],
	}

	for _, side := range []string{"bids", "asks"} {
		prior := make(map[string]map[string]any)

		for _, entry := range previous[side].([]any) {
			order := entry.(map[string]any)
			prior[order["order_id"].(string)] = order
		}

		events := make([]any, 0)

		for _, entry := range current[side].([]any) {
			order := entry.(map[string]any)
			orderID := order["order_id"].(string)
			before, exists := prior[orderID]
			event := "add"

			if exists {
				event = "modify"
			}

			if !exists || before["limit_price"] != order["limit_price"] ||
				before["order_qty"] != order["order_qty"] {
				changed := maps.Clone(order)
				changed["event"] = event
				events = append(events, changed)
			}

			delete(prior, orderID)
		}

		for _, order := range prior {
			removed := maps.Clone(order)
			removed["event"] = "delete"
			events = append(events, removed)
		}

		row[side] = events
	}

	return row
}

/*
checksum derives Kraken's CRC over best asks followed by best bids.
*/
func (fixture *Fixture) checksum(row map[string]any) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		for _, entry := range row[side].([]any) {
			order := entry.(map[string]any)
			checksum = fixture.write(checksum, order["limit_price"].(json.Number).String())
			checksum = fixture.write(checksum, order["order_qty"].(json.Number).String())
		}
	}

	return checksum
}

/*
write adds one Kraken-normalized decimal to the rolling Level3 CRC.
*/
func (fixture *Fixture) write(checksum uint32, value string) uint32 {
	text := strings.TrimLeft(
		strings.ReplaceAll(value, ".", ""),
		"0",
	)

	return crc32.Update(checksum, crc32.IEEETable, []byte(text))
}

func advanceLevels(row map[string]any, side string, step int, direction float64) {
	levels := row[side].([]any)

	for i, raw := range levels {
		level := raw.(map[string]any)
		level["limit_price"] = round(number(level, "limit_price") + direction*0.0001*float64(step+1+i))
		level["order_qty"] = round(math.Max(number(level, "order_qty")*(1+0.01*float64(step+1)), 0))
		level["timestamp"] = advance(level["timestamp"].(string), time.Duration(step+1)*250*time.Millisecond)
	}
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture clone decode failed", err))
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
