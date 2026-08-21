package tests

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/backtest"
)

const (
	storedInstrumentSnapshot = `{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online","qty_precision":8,"qty_increment":0.00000001,"price_precision":1,"cost_min":0.5,"tick_size":0.1,"price_increment":0.1,"qty_min":0.00005},{"symbol":"ETH/USD","base":"ETH","quote":"USD","status":"online","qty_precision":5,"qty_increment":0.00001,"price_precision":2,"cost_min":1.25,"tick_size":0.05,"price_increment":0.05,"qty_min":0.002}]}}`
	storedTradeVolume        = `{"error":[],"result":{"fees":{"ETHUSD":{"fee":"0.40"},"XBTUSD":{"fee":"0.26"}},"fees_maker":{"ETHUSD":{"fee":"0.25"},"XBTUSD":{"fee":"0.16"}},"schedules":[{"pair":"ETHUSD","tiers":[{"maker_fee":"0.25","taker_fee":"0.40","active":true}]},{"pair":"XBTUSD","tiers":[{"maker_fee":"0.16","taker_fee":"0.26","active":true}]}]}}`
	storedETHSubscription    = `{"method":"subscribe","success":true,"result":{"channel":"trade","symbol":"ETH/USD"}}`
	storedBTCSubscription    = `{"method":"subscribe","success":true,"result":{"channel":"trade","symbol":"BTC/USD"}}`
	storedBTCTicker          = `{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/USD","bid":63000.0,"ask":63000.2,"last":62999.9}]}`
	storedETHTicker          = `{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","bid":3500.1,"ask":3500.3,"last":0}]}`
)

func TestCaptureSymbolsFromStoredFrames(t *testing.T) {
	Convey("Given stored venue profiles in capture order", t, func() {
		frames := []backtest.Frame{
			{Endpoint: "public", Payload: []byte(storedInstrumentSnapshot)},
			{Endpoint: "/0/private/TradeVolume", Payload: []byte(storedTradeVolume)},
			{Endpoint: "public", Payload: []byte(storedETHSubscription)},
			{Endpoint: "public", Payload: []byte(storedBTCSubscription)},
			{Endpoint: "public", Payload: []byte(storedBTCTicker)},
			{Endpoint: "public", Payload: []byte(storedETHTicker)},
		}

		symbols, err := CaptureSymbolsFromStoredFrames(
			storedFrameReader(frames),
			25,
		)

		Convey("It should preserve subscription order and exact execution rules", func() {
			So(err, ShouldBeNil)
			So(symbols, ShouldHaveLength, 2)
			ethBid, parseErr := decimal.NewFromString("3500.1")
			So(parseErr, ShouldBeNil)
			ethAsk, parseErr := decimal.NewFromString("3500.3")
			So(parseErr, ShouldBeNil)
			btcBid, parseErr := decimal.NewFromString("63000.0")
			So(parseErr, ShouldBeNil)
			btcAsk, parseErr := decimal.NewFromString("63000.2")
			So(parseErr, ShouldBeNil)

			So(symbols[0].Pair, ShouldEqual, "ETH/USD")
			So(symbols[0].StartPrice, ShouldEqual, (3500.1+3500.3)/2)
			So(symbols[0].PriceIncrement, ShouldEqual, 0.05)
			So(symbols[0].PricePrecision, ShouldEqual, 2)
			So(symbols[0].QuantityPrecision, ShouldEqual, 5)
			So(symbols[0].OrderMinimum, ShouldEqual, 0.002)
			So(symbols[0].CostMinimum, ShouldEqual, 1.25)
			So(symbols[0].TakerFeePercent, ShouldEqual, 0.40)
			So(symbols[0].MakerFeePercent, ShouldEqual, 0.25)
			So(symbols[0].BaseSpreadFraction, ShouldEqual,
				(ethAsk.Float64()-ethBid.Float64())/
					((ethBid.Float64()+ethAsk.Float64())/2))
			So(symbols[0].BookDepthLevels, ShouldEqual, 25)

			So(symbols[1].Pair, ShouldEqual, "BTC/USD")
			So(symbols[1].StartPrice, ShouldEqual, 62999.9)
			So(symbols[1].PriceIncrement, ShouldEqual, 0.1)
			So(symbols[1].PricePrecision, ShouldEqual, 1)
			So(symbols[1].QuantityPrecision, ShouldEqual, 8)
			So(symbols[1].OrderMinimum, ShouldEqual, 0.00005)
			So(symbols[1].CostMinimum, ShouldEqual, 0.5)
			So(symbols[1].TakerFeePercent, ShouldEqual, 0.26)
			So(symbols[1].MakerFeePercent, ShouldEqual, 0.16)
			So(symbols[1].BaseSpreadFraction, ShouldEqual,
				(btcAsk.Float64()-btcBid.Float64())/
					((btcBid.Float64()+btcAsk.Float64())/2))
			So(symbols[1].BookDepthLevels, ShouldEqual, 25)
		})
	})

	Convey("Given a maker fee that disagrees with its active schedule", t, func() {
		frames := []backtest.Frame{
			{Endpoint: "public", Payload: []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online","qty_precision":8,"qty_increment":0.00000001,"price_precision":1,"cost_min":0.5,"tick_size":0.1,"price_increment":0.1,"qty_min":0.00005}]}}`)},
			{Endpoint: "/0/private/TradeVolume", Payload: []byte(`{"error":[],"result":{"fees":{"BTCUSD":{"fee":"0.26"}},"fees_maker":{"BTCUSD":{"fee":"0.15"}},"schedules":[{"pair":"BTCUSD","tiers":[{"maker_fee":"0.16","taker_fee":"0.26","active":true}]}]}}`)},
			{Endpoint: "public", Payload: []byte(storedBTCSubscription)},
			{Endpoint: "public", Payload: []byte(storedBTCTicker)},
		}

		_, err := CaptureSymbolsFromStoredFrames(storedFrameReader(frames), 25)

		Convey("It should reject the contradictory fee evidence", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"market: fee batch at record 2: inconsistent active fee tier for BTCUSD")
		})
	})

	Convey("Given a quantity increment that disagrees with venue precision", t, func() {
		frames := []backtest.Frame{
			{Endpoint: "public", Payload: []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online","qty_precision":8,"qty_increment":0.000001,"price_precision":1,"cost_min":0.5,"tick_size":0.1,"price_increment":0.1,"qty_min":0.00005}]}}`)},
			{Endpoint: "/0/private/TradeVolume", Payload: []byte(`{"error":[],"result":{"fees":{"BTCUSD":{"fee":"0.26"}},"fees_maker":{"BTCUSD":{"fee":"0.16"}},"schedules":[{"pair":"BTCUSD","tiers":[{"maker_fee":"0.16","taker_fee":"0.26","active":true}]}]}}`)},
			{Endpoint: "public", Payload: []byte(storedBTCSubscription)},
			{Endpoint: "public", Payload: []byte(storedBTCTicker)},
		}

		_, err := CaptureSymbolsFromStoredFrames(storedFrameReader(frames), 25)

		Convey("It should reject the contradictory quantity rules", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"market: quantity increment disagrees with precision for BTC/USD")
		})
	})
}

func storedFrameReader(frames []backtest.Frame) func() (backtest.Frame, bool) {
	index := 0

	return func() (backtest.Frame, bool) {
		if index == len(frames) {
			return backtest.Frame{}, false
		}

		frame := frames[index]
		index++

		return frame, true
	}
}

func BenchmarkCaptureSymbolsFromStoredFrames(b *testing.B) {
	frames := []backtest.Frame{
		{Endpoint: "public", Payload: []byte(storedInstrumentSnapshot)},
		{Endpoint: "/0/private/TradeVolume", Payload: []byte(storedTradeVolume)},
		{Endpoint: "public", Payload: []byte(storedETHSubscription)},
		{Endpoint: "public", Payload: []byte(storedBTCSubscription)},
		{Endpoint: "public", Payload: []byte(storedBTCTicker)},
		{Endpoint: "public", Payload: []byte(storedETHTicker)},
	}
	b.ReportAllocs()

	for b.Loop() {
		symbols, err := CaptureSymbolsFromStoredFrames(storedFrameReader(frames), 25)

		if err != nil {
			b.Fatal(err)
		}

		if len(symbols) != 2 {
			b.Fatalf("expected two symbols, got %d", len(symbols))
		}
	}
}
