package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
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

		detectorCfg := &system.Detector{MinPrecursorTicks: 10, ExecutionLeadFraction: 0.1}

		// detectPath runs a fresh detector over one trade per tick (tick
		// idx+1 trades at path[idx], touch p*0.9999 / p*1.0001) and returns
		// every excursion read back from storage.
		detectPath := func(path []float64) ([]*data.Value, *strategy.Detector) {
			price, err := offlinePrice(ctx, "0.26", []string{symbol})
			So(err, ShouldBeNil)

			detector, err := strategy.NewDetector(ctx, price, epoch, symbol, detectorCfg)
			So(err, ShouldBeNil)

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

			read := make([]*data.Value, 0)
			for _, excursion := range append(excursions, results...) {
				read = append(read, stored(excursion))
			}

			return read, detector
		}

		// upDown is flat ticks, then a 100-tick rise and a 100-tick fall.
		upDown := func(flat int) []float64 {
			path := make([]float64, 0, flat+300)
			for idx := range flat {
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
			return path
		}

		upOf := func(excursions []*data.Value) map[string]float64 {
			for _, excursion := range excursions {
				if excursion.Role == "up" {
					fields, err := data.ExcursionFields(excursion)
					So(err, ShouldBeNil)
					return fields
				}
			}
			return nil
		}

		Convey("detect -> train/audit: an excursion reads back under its prefix with every field", func() {
			path := upDown(300)
			excursions, _ := detectPath(path)
			So(excursions, ShouldNotBeEmpty)

			for _, read := range excursions {
				So(read.Prefix, ShouldStartWith, data.Prefix(data.ExcursionOrigin, read.Role, symbol, epoch))
				So(data.ExcursionPrefixes(symbol, epoch), ShouldContain, data.Prefix(data.ExcursionOrigin, read.Role, symbol, epoch))

				fields, err := data.ExcursionFields(read)
				So(err, ShouldBeNil)
				So(fields["b_tick"], ShouldBeLessThan, fields["c_tick"])
				So(fields["a_tick"], ShouldBeGreaterThanOrEqualTo, 1)
				So(fields["entry_tick"], ShouldBeLessThanOrEqualTo, fields["b_tick"])
				So(fields["exit_tick"], ShouldBeLessThanOrEqualTo, fields["c_tick"])
			}

			up := upOf(excursions)
			So(up, ShouldNotBeNil)

			// A is half the B..C move before B (300 flat ticks: no clamp).
			b, c := int64(up["b_tick"]), int64(up["c_tick"])
			So(int64(up["a_tick"]), ShouldEqual, b-(c-b)/2)
			So(int64(up["d_tick"]), ShouldEqual, c+(c-b)/2)

			// Entry and exit lead B and C by 10% of the precursor, and carry
			// the touch their ticks recorded.
			lead := int64(math.Round(0.1 * float64(b-int64(up["a_tick"]))))
			So(lead, ShouldBeGreaterThan, 0)
			So(int64(up["entry_tick"]), ShouldEqual, b-lead)
			So(int64(up["exit_tick"]), ShouldEqual, c-lead)
			So(up["entry_ask"], ShouldAlmostEqual, path[int(b-lead)-1]*1.0001, 1e-9)
			So(up["exit_bid"], ShouldAlmostEqual, path[int(c-lead)-1]*0.9999, 1e-9)
			So(up["entry_at"], ShouldBeLessThan, up["b_at"])
			So(up["exit_at"], ShouldBeLessThan, up["c_at"])
			So(up["entry_idx"], ShouldEqual, up["entry_tick"])

			// The audit's precursor stage accepts the stored excursion.
			var stored *data.Value
			for _, excursion := range excursions {
				if excursion.Role == "up" {
					stored = excursion
				}
			}

			stage := audit.NewPrecursorStage(ctx)
			go func() {
				stage.Channel() <- stored
				close(stage.Channel())
			}()
			So(stage.Run(), ShouldBeNil)
		})

		Convey("detect -> train: the epoch's watermark reads back under its prefix", func() {
			watermark := data.DetectorWatermark{Epoch: epoch, FirstTick: 1, LastTick: 23138, Ticks: 23138, Symbols: 465}
			read := stored(watermark.Value())

			So(read.Prefix, ShouldEqual, data.DetectorWatermarkPrefix(epoch)+fmt.Sprintf("%019d.bin", 23138))
			back, err := data.ReadDetectorWatermark(read)
			So(err, ShouldBeNil)
			So(back, ShouldResemble, watermark)

			// The watermark is no excursion class: train never tallies it.
			for _, prefix := range data.ExcursionPrefixes(data.PolicyScope, epoch) {
				So(read.Prefix, ShouldNotStartWith, prefix)
			}
		})

		Convey("A watermark that is foreign, short a field, or disagrees with its key is refused", func() {
			_, err := data.ReadDetectorWatermark(stored(collectTick(epoch, 1, kraken.TradeData{
				Symbol: symbol, Side: "buy", Price: *decimal.NewFromFloat64(1), Qty: 1, OrderType: "market", TradeID: 1, Timestamp: time.Unix(1, 0),
			}, 0.9, 1.1)))
			So(err, ShouldNotBeNil)

			short := &data.Value{Origin: data.ExcursionOrigin, Role: data.DetectorWatermarkRole, Scope: data.PolicyScope, Epoch: epoch, Tick: 9}
			short.FormatPrefix()
			data.AppendVector(short, []float64{1, 9, 9}, data.DetectorWatermarkKeys[:3]...)
			_, err = data.ReadDetectorWatermark(stored(short))
			So(err, ShouldNotBeNil)

			moved := data.DetectorWatermark{Epoch: epoch, FirstTick: 1, LastTick: 9, Ticks: 9, Symbols: 1}.Value()
			moved.Tick = 10
			moved.FormatPrefix()
			_, err = data.ReadDetectorWatermark(stored(moved))
			So(err, ShouldNotBeNil)
		})

		Convey("detect clamps A to the symbol's first trade when enough precursor remains", func() {
			excursions, detector := detectPath(upDown(20))
			up := upOf(excursions)
			So(up, ShouldNotBeNil)

			b, c := int64(up["b_tick"]), int64(up["c_tick"])
			So(b-(c-b)/2, ShouldBeLessThan, 1)
			So(up["a_tick"], ShouldEqual, 1)
			So(b-1, ShouldBeGreaterThanOrEqualTo, detectorCfg.MinPrecursorTicks)
			So(int64(up["entry_tick"]), ShouldEqual, b-int64(math.Round(0.1*float64(b-1))))

			// Only episodes starting within min_precursor_ticks of the tape's
			// start (the opening flat stretch) go unemitted.
			So(detector.Insufficient(), ShouldBeLessThanOrEqualTo, 1)
			for _, excursion := range excursions {
				fields, err := data.ExcursionFields(excursion)
				So(err, ShouldBeNil)
				So(fields["b_tick"]-fields["a_tick"], ShouldBeGreaterThanOrEqualTo, detectorCfg.MinPrecursorTicks)
			}
		})

		Convey("detect does not emit an excursion left with too short a precursor", func() {
			// The rise starts at tick 1: nothing precedes B.
			excursions, detector := detectPath(upDown(0))

			for _, excursion := range excursions {
				fields, err := data.ExcursionFields(excursion)
				So(err, ShouldBeNil)
				So(fields["b_tick"]-fields["a_tick"], ShouldBeGreaterThanOrEqualTo, detectorCfg.MinPrecursorTicks)
				So(excursion.Role, ShouldNotEqual, "up")
			}

			So(detector.Insufficient(), ShouldBeGreaterThan, 0)
		})

		Convey("detect refuses a missing or out-of-range detector configuration", func() {
			price, err := offlinePrice(ctx, "0.26", []string{symbol})
			So(err, ShouldBeNil)

			for _, cfg := range []*system.Detector{
				nil,
				{MinPrecursorTicks: 0, ExecutionLeadFraction: 0.1},
				{MinPrecursorTicks: 10, ExecutionLeadFraction: -0.1},
				{MinPrecursorTicks: 10, ExecutionLeadFraction: 1},
			} {
				_, err := strategy.NewDetector(ctx, price, epoch, symbol, cfg)
				So(err, ShouldNotBeNil)
			}
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

		paperRecord := func(action, path string, seq uint64) *data.Value {
			record := data.NewValue(data.PaperOrigin, action, symbol)
			record.Epoch, record.Tick = epoch+1, seq
			record.FormatPrefix()
			vector := make([]float64, len(data.PaperKeys))

			for idx := range vector {
				vector[idx] = float64(idx) + 0.5
			}

			So(data.AppendPaper(record, path, epoch, vector), ShouldBeNil)
			return record
		}

		Convey("run -> paper -> audit: a paper record reads back from the local file and the store with every field", func() {
			var file bytes.Buffer

			// run appends each record to its local file with io.Copy.
			for idx, action := range []string{"enter", "exit"} {
				_, err := io.Copy(&file, paperRecord(action, "R01/R04", uint64(idx+1)))
				So(err, ShouldBeNil)
			}

			records, err := readPaperRecords(bytes.NewReader(file.Bytes()))
			So(err, ShouldBeNil)
			So(records, ShouldHaveLength, 2)

			for idx, action := range []string{"enter", "exit"} {
				// paper uploads each one under its own key; audit reads it.
				read := stored(records[idx])
				So(data.PaperPrefixes(symbol, epoch+1), ShouldContain, data.Prefix(data.PaperOrigin, action, symbol, epoch+1))
				So(read.Prefix, ShouldEqual, data.Prefix(data.PaperOrigin, action, symbol, epoch+1)+fmt.Sprintf("%019d.bin", idx+1))
				So(data.PaperActionPrefixes(), ShouldContain, data.PaperOrigin+"/"+action+"/")

				record, err := data.PaperFields(read)
				So(err, ShouldBeNil)
				So(record.Action, ShouldEqual, action)
				So(record.Symbol, ShouldEqual, symbol)
				So(record.Path, ShouldEqual, "R01/R04")
				So(record.PolicyEpoch, ShouldEqual, epoch)

				for key, field := range data.PaperKeys {
					So(record.Fields[field], ShouldEqual, float64(key)+0.5)
				}
			}
		})

		Convey("A truncated or foreign record in the paper file is a loud error", func() {
			var file bytes.Buffer
			_, err := io.Copy(&file, paperRecord("enter", "R01/R04", 1))
			So(err, ShouldBeNil)

			_, err = readPaperRecords(bytes.NewReader(file.Bytes()[:file.Len()-3]))
			So(err, ShouldNotBeNil)

			foreign := &data.Value{Origin: data.PolicyOrigin, Role: "enter", Scope: data.PolicyScope, Epoch: epoch, Tick: 1}
			foreign.FormatPrefix()
			_, err = io.Copy(&file, foreign)
			So(err, ShouldBeNil)

			_, err = readPaperRecords(bytes.NewReader(file.Bytes()))
			So(err, ShouldNotBeNil)

			empty, err := readPaperRecords(bytes.NewReader(nil))
			So(err, ShouldBeNil)
			So(empty, ShouldBeEmpty)
		})

		Convey("A paper record without its full vector or path is refused", func() {
			record := data.NewValue(data.PaperOrigin, "enter", symbol)
			So(data.AppendPaper(record, "R01/R04", epoch, []float64{1}), ShouldNotBeNil)
			So(data.AppendPaper(record, "", epoch, make([]float64, len(data.PaperKeys))), ShouldNotBeNil)

			_, err := data.PaperFields(stored(record))
			So(err, ShouldNotBeNil)
		})
	})
}
