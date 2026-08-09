package signal

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestGeneratorGenerate(t *testing.T) {
	Convey("Given a generator and valid JSON template", t, func() {
		generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)
		template := []byte(`{"channel":"ticker","type":"snapshot","data":[{}]}`)
		count := 0

		for frame := range generator.Generate(template) {
			So(frame, ShouldNotBeEmpty)
			count++
		}

		Convey("Exactly one newly sampled frame should be yielded", func() {
			So(count, ShouldEqual, 1)
		})
	})
}

func TestGeneratorRender(t *testing.T) {
	Convey("Given one three-level coherent market sample", t, func() {
		symbol := testtypes.NewSymbol("SIM1/USD", 100, 42)
		symbol.BookDepthLevels = 3
		generator := NewGeneratorFromSymbol(symbol)
		So(generator.SetTime(time.Unix(10, 0)), ShouldBeNil)
		sample := generator.Step()
		stamp := sample.Timestamp.Format(time.RFC3339Nano)
		ticker := map[string]any{}
		book := map[string]any{}
		trade := map[string]any{}
		level3 := map[string]any{}

		So(json.Unmarshal(generator.Render(
			[]byte(`{"channel":"ticker","type":"update","data":[{}]}`), sample,
		), &ticker), ShouldBeNil)
		So(json.Unmarshal(generator.Render(
			[]byte(`{"channel":"book","type":"snapshot","data":[{}]}`), sample,
		), &book), ShouldBeNil)
		So(json.Unmarshal(generator.Render(
			[]byte(`{"channel":"trade","type":"update","data":[{}]}`), sample,
		), &trade), ShouldBeNil)
		So(json.Unmarshal(generator.Render(
			[]byte(`{"channel":"level3","type":"snapshot","data":[{}]}`), sample,
		), &level3), ShouldBeNil)
		tickerRow := firstRow(ticker)
		bookRow := firstRow(book)
		tradeRow := firstRow(trade)
		level3Row := firstRow(level3)
		bids, _ := bookRow["bids"].([]any)
		asks, _ := bookRow["asks"].([]any)
		l3Bids, _ := level3Row["bids"].([]any)

		Convey("Ticker, trade, book, and L3 should share price and time", func() {
			So(tickerRow["symbol"], ShouldEqual, sample.Symbol)
			So(tickerRow["timestamp"], ShouldEqual, stamp)
			So(tickerRow["last"], ShouldEqual, sample.Last)
			So(tradeRow["price"], ShouldEqual, sample.Last)
			So(tradeRow["qty"], ShouldEqual, sample.StepVolume)
			So(bookRow["timestamp"], ShouldEqual, stamp)
			So(level3Row["timestamp"], ShouldEqual, stamp)
			So(bids, ShouldHaveLength, 3)
			So(asks, ShouldHaveLength, 3)
			So(l3Bids, ShouldHaveLength, 3)
			So(firstMap(bids)["price"], ShouldEqual, sample.Bid)
			So(firstMap(asks)["price"], ShouldEqual, sample.Ask)
		})

		next := generator.Step()
		update := map[string]any{}
		So(json.Unmarshal(generator.Render(
			[]byte(`{"channel":"level3","type":"update","data":[{}]}`), next,
		), &update), ShouldBeNil)
		updateBids, _ := firstRow(update)["bids"].([]any)
		firstSnapshotOrder := firstMap(l3Bids)["order_id"]

		Convey("An update should delete and replace every stable depth identity", func() {
			So(updateBids, ShouldHaveLength, 6)
			So(firstMap(updateBids)["event"], ShouldEqual, "delete")
			So(firstMap(updateBids)["order_id"], ShouldEqual, firstSnapshotOrder)
			So(updateBids[3].(map[string]any)["event"], ShouldEqual, "add")
			So(updateBids[3].(map[string]any)["order_id"],
				ShouldEqual, firstSnapshotOrder)
		})

		Convey("A malformed fixture template should fail loudly", func() {
			So(func() {
				generator.Render([]byte("{"), sample)
			}, ShouldPanic)
		})
	})
}

func firstRow(frame map[string]any) map[string]any {
	data, _ := frame["data"].([]any)

	return firstMap(data)
}

func firstMap(rows []any) map[string]any {
	row, _ := rows[0].(map[string]any)

	return row
}

func BenchmarkGeneratorRender(b *testing.B) {
	generator := NewGenerator("RENDER/USD", 100, 0.01, 2, 72)
	sample := generator.Step()
	template := []byte(`{"channel":"ticker","type":"update","data":[{}]}`)

	for b.Loop() {
		_ = generator.Render(template, sample)
	}
}
