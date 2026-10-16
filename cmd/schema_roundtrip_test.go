package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/hindsight/audit"
	"github.com/theapemachine/symm/kraken"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

/*
stored passes value through the store's encoding (io.Copy out, Write back):
what one command writes is exactly what the next reads.
*/
func stored(value *data.Value) *data.Value {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, value)
	So(err, ShouldBeNil)

	out := data.New()
	_, err = out.Write(buf.Bytes())
	So(err, ShouldBeNil)
	return out
}

/*
collectTick builds a tick the way cmd/booter.go does for one trade frame:
the frame, the symbol's price layer, balance and positions, then the
signals' Step on reserved slots.
*/
func collectTick(epoch, tick uint64, trade kraken.TradeData, bid, ask float64, signals ...nmruntime.Node) *data.Value {
	frame, err := sonic.Marshal(map[string]any{"channel": "trade", "type": "update", "data": []kraken.TradeData{trade}})
	So(err, ShouldBeNil)

	value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	value.Epoch, value.Tick, value.Timestamp = epoch, tick, uint64(trade.Timestamp.Add(40*time.Millisecond).UnixNano())
	value.FormatPrefix()
	value.AppendJSON(frame, data.TradeIndex("update"))

	bidD, askD := decimal.NewFromFloat64(bid), decimal.NewFromFloat64(ask)
	priceJSON, err := sonic.Marshal(broker.PriceLayer{Symbol: trade.Symbol, Bid: bidD, Ask: askD})
	So(err, ShouldBeNil)

	value.AppendJSON(priceJSON, data.SymbolIndex(data.LayerPrice, trade.Symbol)).
		AppendJSON([]byte(`{}`), data.LayerBalance).
		AppendJSON([]byte(`[]`), data.LayerPositions)

	value.Reserve(len(signals))

	for _, signal := range signals {
		signal.Step(value)
	}

	return value
}

func TestSchemaRoundTrip(t *testing.T) {
	Convey("Given ticks as collect writes them", t, func() {
		system.NewConfig()
		ctx := context.Background()
		const epoch, symbol = uint64(1792187257188073000), "BTC/USD"

		signal := cvd.NewSignal(ctx)
		signal.Transition(nmruntime.READY)

		start := time.Unix(1792187257, 0).UTC()
		trade := func(idx int, price float64) kraken.TradeData {
			return kraken.TradeData{
				Symbol: symbol, Side: []string{"buy", "sell"}[idx%2], Price: *decimal.NewFromFloat64(price),
				Qty: 0.1, OrderType: "market", TradeID: int64(1000 + idx), Timestamp: start.Add(time.Duration(idx) * time.Second),
			}
		}

		Convey("collect -> detect/train/audit: a tick reads back with its trade, touch and signal", func() {
			tick := stored(collectTick(epoch, 1, trade(0, 100), 99.9, 100.1, signal))

			So(tick.Prefix, ShouldEqual, data.TickPrefix(epoch)+fmt.Sprintf("%019d.bin", 1))
			So(data.CheckTickLayers(tick), ShouldBeNil)

			symbolTicks, err := broker.ReadTick(tick)
			So(err, ShouldBeNil)
			So(symbolTicks, ShouldHaveLength, 1)
			So(symbolTicks[0].Symbol, ShouldEqual, symbol)
			So(symbolTicks[0].Trades, ShouldHaveLength, 1)
			So(symbolTicks[0].Price.Ask.Float64(), ShouldEqual, 100.1)
			So(symbolTicks[0].Signals, ShouldHaveLength, 1)
			So(symbolTicks[0].Signals[0].Source, ShouldEqual, "cvd")
			So(symbolTicks[0].Signals[0].Labels, ShouldContain, "signed_net_fraction")

			views, err := audit.TickViews(tick, "")
			So(err, ShouldBeNil)
			So(views, ShouldHaveLength, 1)

			bid, ask, ok := audit.Touch(views[0].Join)
			So(ok, ShouldBeTrue)
			So(bid, ShouldEqual, 99.9)
			So(ask, ShouldEqual, 100.1)

			// Every label the signal wrote is declared in the grid's pins.
			_, err = nmstore.NewGrid(ctx).Observe(views[0].Join, symbol)
			So(err, ShouldBeNil)
		})

		Convey("A signal layer in a retired index format fails loudly", func() {
			tick := collectTick(epoch, 1, trade(0, 100), 99.9, 100.1)
			data.AppendVector(tick, []float64{1}, "signed_net_fraction", "source:cvd")
			tick.Index[len(tick.Index)-1] = symbol + "/" + tick.Index[len(tick.Index)-1]

			_, err := broker.ReadTick(stored(tick))
			So(err, ShouldNotBeNil)
		})

		Convey("detect -> train/audit: an excursion reads back under its prefix with every field", func() {
			price, err := offlinePrice(ctx, "0.26", []string{symbol})
			So(err, ShouldBeNil)

			detector, err := strategy.NewDetector(ctx, price, epoch, symbol)
			So(err, ShouldBeNil)

			path := make([]float64, 0, 400)
			for idx := range 100 {
				path = append(path, 100+0.001*float64(idx%2))
			}
			for idx := range 100 {
				path = append(path, 100+float64(idx)*0.2)
			}
			for idx := range 100 {
				path = append(path, 120-float64(idx)*0.2)
			}
			for idx := range 100 {
				path = append(path, 100+0.001*float64(idx%2))
			}

			excursions := make([]*data.Value, 0)

			for idx, p := range path {
				tick := stored(collectTick(epoch, uint64(idx+1), trade(idx, p), p*0.9999, p*1.0001))
				symbolTicks, err := broker.ReadTick(tick)
				So(err, ShouldBeNil)

				results, err := detector.Step(tick.Tick, symbolTicks[0].Trades[0], symbolTicks[0].Price)
				So(err, ShouldBeNil)
				excursions = append(excursions, results...)
			}

			results, err := detector.Close()
			So(err, ShouldBeNil)
			excursions = append(excursions, results...)
			So(excursions, ShouldNotBeEmpty)

			var up *data.Value
			for _, excursion := range excursions {
				read := stored(excursion)
				So(read.Prefix, ShouldStartWith, data.Prefix(data.ExcursionOrigin, read.Role, symbol, epoch))
				So(data.ExcursionPrefixes(symbol, epoch), ShouldContain, data.Prefix(data.ExcursionOrigin, read.Role, symbol, epoch))

				fields, err := data.ExcursionFields(read)
				So(err, ShouldBeNil)
				So(fields["b_tick"], ShouldBeLessThan, fields["c_tick"])

				if read.Role == "up" {
					up = read
				}
			}

			So(up, ShouldNotBeNil)

			// The audit's precursor stage accepts the stored excursion.
			stage := audit.NewPrecursorStage(ctx)
			go func() {
				stage.Channel() <- up
				close(stage.Channel())
			}()
			So(stage.Run(), ShouldBeNil)
		})

		Convey("train -> live training/audit: a policy reads back with its path", func() {
			policy := &data.Value{Origin: data.PolicyOrigin, Role: "enter", Scope: data.PolicyScope, Epoch: epoch, Tick: 1}
			policy.FormatPrefix()
			So(data.AppendPolicy(policy, "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66}), ShouldBeNil)

			read := stored(policy)
			So(data.PolicyPrefixes(epoch), ShouldContain, data.Prefix(data.PolicyOrigin, "enter", data.PolicyScope, epoch))
			So(read.Prefix, ShouldStartWith, data.Prefix(data.PolicyOrigin, "enter", data.PolicyScope, epoch))

			path, err := data.PolicyPath(read)
			So(err, ShouldBeNil)
			So(path, ShouldEqual, "R01/R04")
			So(data.Metrics(read)["confidence"], ShouldEqual, 0.66)

			So(data.AppendPolicy(policy, "R01", []float64{1}), ShouldNotBeNil)
		})
	})
}
