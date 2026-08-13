package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketPace(t *testing.T) {
	Convey("Given an undriven fixture", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100, 42),
		})
		defer market.Close()
		market.pace(time.Now().Add(time.Hour))

		Convey("Direct fixture use should not wait for wall-clock pacing", func() {
			So(market.clockSet, ShouldBeFalse)
		})
	})
}

func TestMarketPublishSample(t *testing.T) {
	Convey("Given two replay observations in one candle interval", t, func() {
		symbol := testtypes.NewSymbol("SIM1/USD", 100, 93)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		candles := make(chan *kraken.OHLC, 2)
		candleHandler := market.Public.Client().OnReceived.Recurring(
			func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
				candle := kraken.NewOHLC(event.Data.Bytes())

				if candle.Channel == "ohlc" {
					candles <- candle
				}
			},
		)
		defer market.Public.Client().OnReceived.Deregister(candleHandler)
		begin := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
		first := testtypes.Sample{
			Symbol: symbol.Pair, AggressorSide: "buy",
			Bid: 99, BidQty: 10, Ask: 101, AskQty: 12, Last: 100,
			Volume: 2, StepVolume: 2, VWAP: 100, Low: 100, High: 100,
			Timestamp: begin.Add(10 * time.Second),
		}
		second := testtypes.Sample{
			Symbol: symbol.Pair, AggressorSide: "buy",
			Bid: 100, BidQty: 11, Ask: 102, AskQty: 13, Last: 101,
			Volume: 5, StepVolume: 3, VWAP: 100.6, Low: 100, High: 101,
			Change: 1, ChangePct: 1, Timestamp: begin.Add(20 * time.Second),
		}

		So(market.PublishSample(first), ShouldBeNil)
		So(market.PublishSample(second), ShouldBeNil)
		<-candles
		candle := <-candles

		Convey("All channels should reflect the same replayed trades", func() {
			So(candle.Data[0].Open, ShouldEqual, 100.0)
			So(candle.Data[0].High, ShouldEqual, 101.0)
			So(candle.Data[0].Low, ShouldEqual, 100.0)
			So(candle.Data[0].Close, ShouldEqual, 101.0)
			So(candle.Data[0].Trades, ShouldEqual, int64(2))
			So(candle.Data[0].Volume, ShouldEqual, 5.0)
			So(candle.Data[0].Vwap, ShouldEqual, 100.6)
			So(market.tick, ShouldEqual, uint64(2))
		})

		Convey("Stale or incoherent replay data should be rejected", func() {
			So(market.PublishSample(second), ShouldNotBeNil)
			invalid := second
			invalid.Timestamp = invalid.Timestamp.Add(time.Second)
			invalid.Bids = []testtypes.DepthLevel{{
				Price: invalid.Bid + 1, Quantity: invalid.BidQty,
			}}
			So(market.PublishSample(invalid), ShouldNotBeNil)
		})
	})
}

func TestMarketPublish(t *testing.T) {
	Convey("Given two markets with the same complete replay identity", t, func() {
		first := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM/USD", 100, 17),
		})
		second := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM/USD", 100, 17),
		})
		defer first.Close()
		defer second.Close()
		first.Tick()
		second.Tick()
		firstSample, _ := first.LastSample("SIM/USD")
		secondSample, _ := second.LastSample("SIM/USD")

		Convey("Prices and timestamps should be identical", func() {
			So(firstSample, ShouldResemble, secondSample)
			So(first.Report().PublicTransport.Frames,
				ShouldResemble, second.Report().PublicTransport.Frames)
			So(firstSample.Timestamp,
				ShouldEqual, testtypes.DefaultScenarioStart.Add(100*time.Millisecond))
		})
	})
}

func TestMarketReplay(t *testing.T) {
	Convey("Given the opening frames of an exact Kraken capture", t, func() {
		symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
		symbol.PriceIncrement = 0.00001
		symbol.PricePrecision = 5
		symbol.QuantityPrecision = 5
		symbol.TakerFeePercent = 0.4
		symbol.MakerFeePercent = 0.23
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.Execution.DepthLevels = 10
		market, err := NewMarketWithScenario(t.Context(), config)
		So(err, ShouldBeNil)
		defer market.Close()
		previousDepth := viper.GetInt("market.l3_depth")
		viper.Set("market.l3_depth", 10)
		defer viper.Set("market.l3_depth", previousDepth)
		market.private.SubL3([]string{"IDOS/USD"})
		market.WithAutoFill()
		capture, err := os.Open(
			"/Users/theapemachine/.symm/data/backtests/kraken/" +
				"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
		)
		So(err, ShouldBeNil)
		defer capture.Close()

		decoder := json.NewDecoder(capture)
		var tape bytes.Buffer

		for range 4 {
			var frame json.RawMessage
			So(decoder.Decode(&frame), ShouldBeNil)
			tape.Write(frame)
			tape.WriteByte('\n')
		}

		err = market.Replay(&tape)
		sample, found := market.LastSample("IDOS/USD")

		Convey("It should route original L3 and ticker frames into one executable state", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(sample.Timestamp, ShouldEqual,
				time.Date(2026, time.August, 13, 11, 39, 0, 950432000, time.UTC))
			market.private.Book("IDOS/USD", func(book *spotbook.Book) {
				So(book, ShouldNotBeNil)
				So(book.BestBid().Price.Float64(), ShouldEqual, 0.00455)
				So(book.BestAsk().Price.Float64(), ShouldEqual, 0.00461)
				So(book.L3Checksum(fmt.Sprint(uint32(4289101961))).Match, ShouldBeTrue)
			})
		})
	})

	Convey("Given malformed or unordered captured data", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("IDOS/USD", 0.00455, 13),
		})
		defer market.Close()

		Convey("It should reject the tape instead of silently altering it", func() {
			So(market.Replay(bytes.NewBufferString("")), ShouldNotBeNil)
			So(market.Replay(bytes.NewBufferString(
				"{\"endpoint\":\"public\",\"payload\":{},\"received_at\":\"bad\"}\n",
			)), ShouldNotBeNil)
		})
	})
}

func BenchmarkMarketReplay(b *testing.B) {
	payload, err := os.ReadFile(
		"/Users/theapemachine/.symm/data/backtests/kraken/" +
			"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
	)

	if err != nil {
		b.Fatal(err)
	}

	newline := bytes.IndexByte(payload, '\n')
	frame := append([]byte(nil), payload[:newline+1]...)
	symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
	symbol.PriceIncrement = 0.00001
	symbol.PricePrecision = 5
	symbol.QuantityPrecision = 5
	config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
	config.Execution.DepthLevels = 10
	previousDepth := viper.GetInt("market.l3_depth")
	viper.Set("market.l3_depth", 10)
	defer viper.Set("market.l3_depth", previousDepth)
	b.ReportAllocs()

	for b.Loop() {
		market, marketErr := NewMarketWithScenario(b.Context(), config)

		if marketErr != nil {
			b.Fatal(marketErr)
		}
		market.private.SubL3([]string{"IDOS/USD"})

		if replayErr := market.Replay(bytes.NewReader(frame)); replayErr != nil {
			b.Fatal(replayErr)
		}

		market.Close()
	}
}
