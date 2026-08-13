package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketReplayCapture(t *testing.T) {
	Convey("Given exact frames split across independent capture readers", t, func() {
		capture, err := os.Open(
			"/Users/theapemachine/.symm/data/backtests/kraken/" +
				"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
		)
		So(err, ShouldBeNil)
		defer capture.Close()
		decoder := json.NewDecoder(capture)
		readers := [2]bytes.Buffer{}

		for index := range 4 {
			var frame json.RawMessage
			So(decoder.Decode(&frame), ShouldBeNil)
			readers[index%len(readers)].Write(frame)
			readers[index%len(readers)].WriteByte('\n')
		}

		symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
		symbol.PriceIncrement = 0.00001
		symbol.PricePrecision = 5
		symbol.QuantityPrecision = 5
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.Execution.DepthLevels = 10
		market, err := NewMarketWithScenario(t.Context(), config)
		So(err, ShouldBeNil)
		defer market.Close()
		previousDepth, depthWasSet := viper.GetInt("market.l3_depth"),
			viper.IsSet("market.l3_depth")
		viper.Set("market.l3_depth", 10)
		defer func() {
			if depthWasSet {
				viper.Set("market.l3_depth", previousDepth)
				return
			}

			viper.Set("market.l3_depth", nil)
		}()
		market.private.SubL3([]string{"IDOS/USD"})

		err = market.ReplayCapture([]string{"IDOS/USD"}, &readers[0], &readers[1])
		sample, found := market.LastSample("IDOS/USD")

		Convey("It should restore arrival order without modifying payloads", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(sample.Bid, ShouldEqual, 0.00455)
			So(sample.Ask, ShouldEqual, 0.00461)
			market.private.Book("IDOS/USD", func(book *spotbook.Book) {
				So(book.L3Checksum("4289101961").Match, ShouldBeTrue)
			})
		})
	})

	Convey("Given capture readers without the requested symbol", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100, 1),
		})
		defer market.Close()
		capture := bytes.NewBufferString(
			`{"endpoint":"public","payload":{"channel":"ticker","data":[{"symbol":"ETH/USD"}]},"received_at":"2026-08-13T00:00:00Z"}`,
		)

		Convey("It should reject the empty selection", func() {
			So(market.ReplayCapture([]string{"BTC/USD"}, capture), ShouldNotBeNil)
		})
	})
}

func BenchmarkMarketReplayCapture(b *testing.B) {
	payload, err := os.ReadFile(
		"/Users/theapemachine/.symm/data/backtests/kraken/" +
			"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
	)

	if err != nil {
		b.Fatal(err)
	}

	firstNewline := bytes.IndexByte(payload, '\n')
	frame := append([]byte(nil), payload[:firstNewline+1]...)
	symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
	symbol.PriceIncrement = 0.00001
	symbol.PricePrecision = 5
	symbol.QuantityPrecision = 5
	config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
	config.Execution.DepthLevels = 10
	b.ReportAllocs()

	for b.Loop() {
		market, marketErr := NewMarketWithScenario(b.Context(), config)

		if marketErr != nil {
			b.Fatal(marketErr)
		}

		market.private.SubL3([]string{"IDOS/USD"})

		if replayErr := market.ReplayCapture(
			[]string{"IDOS/USD"},
			bytes.NewReader(frame),
		); replayErr != nil {
			b.Fatal(replayErr)
		}

		market.Close()
	}
}
