package hindsight

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
)

/*
tradesTick is a stored tick where symbol traded once per price, each trade a
millisecond after the last, and other traded once in between.
*/
func tradesTick(epoch, tick uint64, symbol string, prices []float64) *data.Value {
	trades := make([]kraken.TradeData, 0, len(prices)+1)

	for idx, price := range prices {
		trades = append(trades, kraken.TradeData{
			Symbol: symbol, Side: "buy", Price: *decimal.NewFromFloat64(price), Qty: 0.1, OrderType: "market",
			TradeID: int64(tick*100) + int64(idx), Timestamp: tradeAt(tick, idx),
		})

		if idx == 0 {
			trades = append(trades, kraken.TradeData{
				Symbol: ethUSD, Side: "sell", Price: *decimal.NewFromFloat64(2000), Qty: 1, OrderType: "market",
				TradeID: int64(tick*100) + 99, Timestamp: tradeAt(tick, 0),
			})
		}
	}

	frame, err := sonic.Marshal(map[string]any{"channel": "trade", "type": "update", "data": trades})
	So(err, ShouldBeNil)

	value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	value.Epoch, value.Tick, value.Timestamp = epoch, tick, tick*uint64(time.Second)
	value.FormatPrefix()
	value.AppendJSON(frame, data.TradeIndex("update"))

	for _, traded := range []string{symbol, ethUSD} {
		price, err := sonic.Marshal(broker.PriceLayer{Symbol: traded, Bid: decimal.NewFromFloat64(99), Ask: decimal.NewFromFloat64(101)})
		So(err, ShouldBeNil)
		value.AppendJSON(price, data.SymbolIndex(data.LayerPrice, traded))
	}

	return value
}

func tradeAt(tick uint64, idx int) time.Time {
	return time.Unix(int64(tick), int64(idx)*int64(time.Millisecond)).UTC()
}

func TestDetections(t *testing.T) {
	Convey("Given an epoch detect labelled: ticks, its watermark and one up excursion of BTC/USD", t, func() {
		ctx := context.Background()
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()
		backend.Buckets["symm"] = true

		// BTC/USD trades: ticks 1..6, three trades each: seq 1..18.
		for tick := uint64(1); tick <= 6; tick++ {
			base := 100 + float64(tick)
			put(backend, tradesTick(run, tick, btcUSD, []float64{base, base + 0.25, base + 0.5}))
		}

		watermark := data.DetectorWatermark{Epoch: run, FirstTick: 1, LastTick: 6, Ticks: 6, Symbols: 2}
		put(backend, watermark.Value())

		// A at seq 5 (tick 2, 2nd trade), B seq 7 (tick 3, 1st), C seq 13
		// (tick 5, 1st), D seq 16 (tick 6, 1st).
		at := func(tick uint64, idx int) float64 { return float64(tradeAt(tick, idx).UnixNano()) }
		fields := map[string]float64{
			"a_idx": 5, "a_tick": 2, "b_idx": 7, "b_tick": 3, "c_idx": 13, "c_tick": 5, "d_idx": 16, "d_tick": 6,
			"b_price": 103, "c_price": 105, "b_ask": 101, "c_bid": 99, "b_at": at(3, 0), "c_at": at(5, 0),
			"entry_idx": 6, "entry_tick": 2, "exit_idx": 12, "exit_tick": 4,
			"entry_ask": 101, "exit_bid": 99, "entry_at": at(2, 2), "exit_at": at(4, 2),
		}

		excursion := func(fields map[string]float64) {
			value := data.NewValue(data.ExcursionOrigin, "up", btcUSD)
			value.Epoch, value.Tick = run, uint64(fields["c_tick"])
			value.FormatPrefix()
			vector := make([]float64, 0, len(data.ExcursionKeys))

			for _, key := range data.ExcursionKeys {
				vector = append(vector, fields[key])
			}

			data.AppendVector(value, vector, data.ExcursionKeys...)
			put(backend, value)
		}

		excursion(fields)

		reader, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)
		service, err := NewService(ctx, reader)
		So(err, ShouldBeNil)
		stored := len(backend.Objects)

		detect := func(detection Detection) ([]*data.Value, error) {
			out := &bytes.Buffer{}
			err := service.Detect(ctx, out, detection)
			return frames(out.Bytes()), err
		}

		Convey("Run 0 lists each labelled epoch with its watermark and class counts", func() {
			values, err := detect(Detection{})
			So(err, ShouldBeNil)
			So(roles(values), ShouldResemble, []string{data.HindsightDetectionRun, data.HindsightDone})
			So(values[0].Epoch, ShouldEqual, run)

			metrics := data.Metrics(values[0])
			So(metrics["first_tick"], ShouldEqual, 1)
			So(metrics["last_tick"], ShouldEqual, 6)
			So(metrics["ticks"], ShouldEqual, 6)
			So(metrics["symbols"], ShouldEqual, 2)
			So(metrics["up"], ShouldEqual, 1)
			So(metrics["down"], ShouldEqual, 0)
		})

		Convey("A run streams its excursions verbatim", func() {
			values, err := detect(Detection{Run: run})
			So(err, ShouldBeNil)
			So(roles(values), ShouldResemble, []string{data.ExcursionOrigin, data.HindsightDone})
			So(values[0].Origin, ShouldEqual, data.DetectionsOrigin)
			So(values[0].Scope, ShouldEqual, btcUSD)
			So(values[0].Tick, ShouldEqual, 5)
			metrics := data.Metrics(values[0])

			for key, want := range fields {
				So(metrics[key], ShouldEqual, want)
			}
		})

		Convey("An excursion streams its trades A..D with detect's trade index, anchored at B", func() {
			values, err := detect(Detection{Run: run, Symbol: btcUSD, Class: "up", CTick: 5})
			So(err, ShouldBeNil)
			So(roles(values[len(values)-1:]), ShouldResemble, []string{data.HindsightDone})

			seqs, prices := make([]float64, 0), make([]float64, 0)

			for _, value := range values[:len(values)-1] {
				So(value.Role, ShouldEqual, data.HindsightTrades)
				So(value.Scope, ShouldEqual, btcUSD)
				So(value.Index[0], ShouldEndWith, "/seq")
				So(value.Index[1], ShouldEndWith, "/price")
				So(value.Index[2], ShouldEndWith, "/at")
				seqs = append(seqs, floats(value, 0)...)
				prices = append(prices, floats(value, 1)...)
			}

			So(seqs, ShouldResemble, []float64{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			// seq 5 is tick 2's second trade; B (seq 7) tick 3's first.
			So(prices[0], ShouldEqual, 102.25)
			So(prices[slices.Index(seqs, 7)], ShouldEqual, 103)
			So(prices[slices.Index(seqs, 13)], ShouldEqual, 105)
			So(values[0].Tick, ShouldEqual, 2)
		})

		Convey("A C that does not land on c_idx from B is an error, written as an error frame", func() {
			broken := maps(fields)
			broken["c_idx"] = 14
			excursion(broken)

			values, err := detect(Detection{Run: run, Symbol: btcUSD, Class: "up", CTick: 5})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "C counts as trade 13")
			So(values[len(values)-1].Role, ShouldEqual, data.HindsightError)
		})

		Convey("A B trade that is not stored is an error", func() {
			broken := maps(fields)
			broken["b_at"]++
			excursion(broken)

			_, err := detect(Detection{Run: run, Symbol: btcUSD, Class: "up", CTick: 5})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "holds no trade at b_at")
		})

		Convey("An excursion that is not stored, or an unknown class, is an error", func() {
			_, err := detect(Detection{Run: run, Symbol: btcUSD, Class: "up", CTick: 4})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no BTC/USD up excursion")

			_, err = detect(Detection{Run: run, Symbol: btcUSD, Class: "sideways", CTick: 5})
			So(err, ShouldNotBeNil)
		})

		Convey("Nothing is ever written to the bucket", func() {
			for _, detection := range []Detection{{}, {Run: run}, {Run: run, Symbol: btcUSD, Class: "up", CTick: 5}} {
				_, err := detect(detection)
				So(err, ShouldBeNil)
			}

			So(backend.Objects, ShouldHaveLength, stored)
		})
	})
}

/*
floats decodes vector layer idx of value (little-endian float64s).
*/
func floats(value *data.Value, idx int) []float64 {
	raw := *data.Layer[[]byte](value, idx)
	out := make([]float64, len(raw)/8)

	for at := range out {
		out[at] = math.Float64frombits(binary.LittleEndian.Uint64(raw[at*8:]))
	}

	return out
}

func maps(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))

	for key, value := range in {
		out[key] = value
	}

	return out
}
