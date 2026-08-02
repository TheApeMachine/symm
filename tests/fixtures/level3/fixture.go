package level3

import (
	"cmp"
	"embed"
	"encoding/json"
	"hash/crc32"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
FixtureType selects the Kraken Level3 envelope represented by a fixture.
*/
type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

/*
Fixture yields template-backed standalone or market-driven Level3 frames.
*/
type Fixture struct {
	horizon   int
	sequence  [][]byte
	template  []byte
	generator *tests.Generator
	typ       FixtureType
	previous  map[string]map[string]any
}

/*
NewDecoderFixture repeats embedded Level3 wire examples for parser tests. Full
market tests use NewMarket, which owns a coherent order ledger and valid CRC;
this constructor does not present its repeated updates as an exchange replay.
*/
func NewDecoderFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon, template: raw}

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
func NewMarket(symbols []string, generator *tests.Generator) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture load failed", err))
	}

	return &Fixture{
		template:  raw,
		generator: generator,
		previous:  make(map[string]map[string]any, len(symbols)),
		typ:       SNAPSHOT,
	}
}

/*
Generate yields ready Kraken Level3 payloads in deterministic order.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	if fixture.generator != nil {
		return fixture.generator.Generate(fixture.template)
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
func (fixture *Fixture) render(samples []tests.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture decode failed", err))
	}

	template := payload["data"].([]any)[0].(map[string]any)
	rows := make([]map[string]any, 0, len(samples))

	for _, sample := range samples {
		row := clone(template)

		row["symbol"] = sample.Symbol
		row["timestamp"] = sample.Timestamp.Format(time.RFC3339Nano)
		row["checksum"] = fixture.checksum(row)
		resting := row

		if fixture.typ == UPDATE {
			row = fixture.update(fixture.previous[sample.Symbol], row)
		}

		fixture.previous[sample.Symbol] = resting
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := json.Marshal(payload)

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
	orders []tests.Order,
) {
	orders = append([]tests.Order(nil), orders...)
	slices.SortFunc(orders, func(left, right tests.Order) int {
		priceOrder := cmp.Compare(left.Price, right.Price)

		if side == "bids" {
			priceOrder = -priceOrder
		}

		if priceOrder != 0 {
			return priceOrder
		}

		if left.Priority < right.Priority {
			return -1
		}

		if left.Priority > right.Priority {
			return 1
		}

		return strings.Compare(left.ID, right.ID)
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

		removedIDs := slices.Sorted(maps.Keys(prior))

		for _, orderID := range removedIDs {
			order := prior[orderID]
			removed := maps.Clone(order)
			removed["event"] = "delete"
			removed["timestamp"] = current["timestamp"]
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
		levels := 0
		lastPrice := ""

		for _, entry := range row[side].([]any) {
			order := entry.(map[string]any)
			price := order["limit_price"].(json.Number).String()

			if price != lastPrice {
				levels++
				lastPrice = price
			}

			if levels > 10 {
				break
			}

			checksum = fixture.write(checksum, price)
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
