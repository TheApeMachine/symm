package strategy

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

/*
trainingFixture records one past run, lets each tape add that run's signal,
trade, or detection rows, and returns a Training for a later run once its
trie loading has finished.
*/
func trainingFixture(
	t *testing.T, ctx context.Context, tapes ...func(*tables.Writer, int64),
) *Training {
	training := trainingSetup(t, ctx, tapes...)
	training.Train()

	deadline := time.After(2 * time.Second)

	for training.Passes() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the training pass to complete")
		case <-time.After(20 * time.Millisecond):
		}
	}

	return training
}

/*
trainingSetup is trainingFixture without the trie loading: it records the
past run and returns a WAITING Training that has not started Train.
*/
func trainingSetup(
	t *testing.T, ctx context.Context, tapes ...func(*tables.Writer, int64),
) *Training {
	catalog := tablestest.New(t)
	epoch := int64(100)

	So(catalog.RecordRun(ctx, tables.Run{
		Epoch:     epoch,
		StartedAt: time.Now().UTC(),
		Status:    "ACTIVE",
	}), ShouldBeNil)

	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				WSName:        "BTC/USD",
				Base:          "BTC",
				Quote:         "USD",
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.SetReferenceCash(decimal.NewFromFloat64(200))

	writer := tables.NewWriter(catalog, epoch)

	for _, tape := range tapes {
		tape(writer, epoch)
	}

	So(writer.CommitReady(ctx, true), ShouldBeNil)

	storeTee := hindsight.NewStoreTee(ctx, "storeTee")
	storeTee.Transition(runtime.READY)

	training := NewTraining(
		ctx, data.NewArenaOwner("training", 4096), price, broker.NewDesk(ctx, nil, price), catalog, storeTee, 1000,
	)
	training.Transition(runtime.WAITING)

	return training
}

/*
firstDetection returns the first stored detection of the training's catalog.
*/
func firstDetection(ctx context.Context, training *Training) *data.Measurement {
	for det, err := range training.catalog.Detections(ctx) {
		So(err, ShouldBeNil)
		return det
	}

	return nil
}

/*
haltedInternal reports whether training halted the way cmd/root stops the
process on: ERROR status, an Internal error, and a closed training context.
*/
func haltedInternal(training *Training) {
	So(training.Status(), ShouldEqual, runtime.ERROR)
	So(errnie.IsInternal(training.Error()), ShouldBeTrue)

	select {
	case <-training.Context().Done():
	case <-time.After(time.Second):
		So("training context still open", ShouldBeEmpty)
	}
}

/*
signal writes one signal measurement whose value rises with the tick.
*/
func signal(writer *tables.Writer, epoch int64, source string, tick int64, scale float64) {
	measurement := data.NewMeasurement(epoch, "BTC/USD", source, tick*10, tick)
	measurement.At = time.Now().UTC()
	measurement.From = measurement.At
	measurement.Write(data.NewMetric(source+"_value", scale*(1.5+float64(tick)*0.1), data.UnitCount, data.TimescaleTick))
	writer.Add("measurements", data.Publication{Measurement: measurement})
}

/*
uniformTape is an excursion tape (ignition B=10, peak C=15) whose precursor
and holding ticks each carry one signal from four sources the grid settles
into a single region, so the precursor and the holding run share one token.
*/
func uniformTape(writer *tables.Writer, epoch int64) {
	signal(writer, epoch, "cvd", 7, 1)
	signal(writer, epoch, "hawkes", 8, 1)
	signal(writer, epoch, "cvd", 10, 1)
	signal(writer, epoch, "depthflow", 11, 1)
	signal(writer, epoch, "liquidity", 12, 1)
	signal(writer, epoch, "cvd", 15, 1)
}

var (
	precursorSources = []string{"cvd", "hawkes", "sentiment", "toxicity"}
	holdingSources   = []string{"depthflow", "liquidity", "morphology", "pumpdump"}
)

/*
regimeTape is an excursion tape (ignition B=10, peak C=15) in which one group
of sources co-moves through the precursor and another group co-moves through
the holding run, so the settled grid gives the two phases distinct tokens.
*/
func regimeTape(writer *tables.Writer, epoch int64) {
	write := func(sources []string, ticks ...int64) {
		for _, tick := range ticks {
			for index, source := range sources {
				signal(writer, epoch, source, tick, float64(index+1))
			}
		}
	}

	write(precursorSources, 7, 8, 9, 10, 15)
	write(holdingSources, 10, 11, 12, 13, 14, 15)
}

/*
detection writes one stored detector row of the given class for the fixture
excursion (start 7, B=10, C=15).
*/
func detection(class string, bPrice, cPrice float64) func(*tables.Writer, int64) {
	return detectionAt(class, 7, 10, 15, bPrice, cPrice)
}

/*
detectionAt writes one stored detector row of the given class with explicit
start, B, and C ticks, together with the spot:trade rows the detector read it
from: a trade at the start and at B at the B price, and one at C at the C
price. A run with stored detections is never rescanned, so these trades only
feed the fragment's price tape.
*/
func detectionAt(
	class string, start, b, c int64, bPrice, cPrice float64,
) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		detectionRowAt(class, start, b, c, bPrice, cPrice)(writer, epoch)

		prices := map[int64]float64{start: bPrice, b: bPrice, c: cPrice}
		tape := make(map[int64]string, len(prices))

		for tick, price := range prices {
			tape[tick] = strconv.FormatFloat(price, 'f', -1, 64)
		}

		trades(tape)(writer, epoch)
	}
}

/*
trades writes one spot:trade row per tick at the given exact price, with the
sequence index equal to the tick.
*/
func trades(prices map[int64]string) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		for tick, price := range prices {
			exact, err := decimal.NewFromString(price)
			So(err, ShouldBeNil)

			trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", tick, tick)
			trade.At = time.Now().UTC()
			trade.From = trade.At
			trade.Write(data.NewExactMetric("price", exact, data.UnitPrice, data.TimescaleTick))
			writer.Add("measurements", data.Publication{Measurement: trade})
		}
	}
}

/*
detectionRowAt writes only the stored detector row, without the trade tape
it came from.
*/
func detectionRowAt(
	class string, start, b, c int64, bPrice, cPrice float64,
) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		measurement := data.NewMeasurement(
			epoch, "BTC/USD", "detector", start*10, c, &data.StringEntry{Key: "type", Value: class},
		)
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		measurement.Write(
			data.NewMetric("start_tick", float64(start), data.UnitCount, data.TimescaleEvent),
			data.NewMetric("b_tick", float64(b), data.UnitCount, data.TimescaleEvent),
			data.NewMetric("c_tick", float64(c), data.UnitCount, data.TimescaleEvent),
			data.NewExactMetric("b_price", decimal.NewFromFloat64(bPrice), data.UnitPrice, data.TimescaleEvent),
			data.NewExactMetric("c_price", decimal.NewFromFloat64(cPrice), data.UnitPrice, data.TimescaleEvent),
		)
		writer.Add("measurements", data.Publication{Measurement: measurement})
	}
}

func TestTraining_Train(t *testing.T) {
	Convey("Given a past run with a stored excursion whose phases the grid tells apart", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))

		Convey("It loads enter and exit associations, beats the constant-policy baseline, and opens trading", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(training.records(), ShouldBeGreaterThan, 0)
			So(training.classCount("enter"), ShouldBeGreaterThan, 0)
			So(training.classCount("exit"), ShouldBeGreaterThan, 0)
			So(training.baseline.Load(), ShouldEqual, 1)
			So(training.skill.Load(), ShouldBeGreaterThan, training.baseline.Load())
		})
	})

	Convey("Given a past run with a stored excursion whose precursor and holding run share one token", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, uniformTape, detection(excursionUp, 60000, 63000))

		Convey("It learns records but keeps the skill gate closed and stays in WAITING", func() {
			So(training.records(), ShouldBeGreaterThan, 0)
			So(training.classCount("enter"), ShouldBeGreaterThan, 0)
			So(training.classCount("exit"), ShouldBeGreaterThan, 0)
			So(training.baseline.Load(), ShouldEqual, 1)
			So(training.skill.Load(), ShouldBeLessThanOrEqualTo, training.baseline.Load())
			So(training.Status(), ShouldEqual, runtime.WAITING)
		})

		Convey("Step reports the closed gate and never reaches paper trading", func() {
			measurement := data.NewMeasurement(1000, "BTC/USD", "cvd", 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			measurement.Write(data.NewMetric("price", 60000, data.UnitPrice, data.TimescaleTick))

			out := training.Step(measurement)
			So(out, ShouldNotBeNil)
			So(out.Meta("stage"), ShouldEqual, StageHistoricalValidation.String())
			So(out.Meta("stage_blocker"), ShouldContainSubstring, "skill gate")
			So(training.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
		})
	})

	Convey("Given a past run with a trade tape that does not clear friction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, uniformTape, func(writer *tables.Writer, epoch int64) {
			for tick, price := range map[int64]string{
				10: "60000", 11: "60005", 15: "60010", 16: "60008",
			} {
				exact, err := decimal.NewFromString(price)
				So(err, ShouldBeNil)

				trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", tick, tick)
				trade.At = time.Now().UTC()
				trade.From = trade.At
				priceMetric := data.NewMetric("price", exact.Float64(), data.UnitPrice, data.TimescaleTick)
				priceMetric.Exact = exact
				trade.Write(priceMetric)
				writer.Add("measurements", data.Publication{Measurement: trade})
			}
		})

		Convey("It learns the deadband tape as wait, never as enter, and remains in WAITING", func() {
			So(training.Status(), ShouldEqual, runtime.WAITING)
			So(training.records(), ShouldBeGreaterThan, 0)
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.classCount(actionEnter), ShouldEqual, 0)
			So(training.baseline.Load(), ShouldEqual, 1)
			So(training.skill.Load(), ShouldBeLessThanOrEqualTo, training.baseline.Load())

			classes := map[string]bool{}
			for _, fragment := range training.Fragments() {
				classes[fragment.Class] = true
				So(fragment.EntryIdx, ShouldEqual, -1)
			}
			So(classes[excursionChop], ShouldBeTrue)
		})
	})

	Convey("Given a past run with only its trade tape stored", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, func(writer *tables.Writer, epoch int64) {
			// The high at tick 6 precedes the low; the draw-up is 10 -> 15.
			for tick, price := range map[int64]string{
				6: "64000", 7: "61000", 8: "60500", 10: "60000",
				11: "61000", 12: "62000", 15: "63000", 16: "62500",
			} {
				exact, err := decimal.NewFromString(price)
				So(err, ShouldBeNil)

				trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", tick, tick)
				trade.At = time.Now().UTC()
				trade.From = trade.At
				priceMetric := data.NewMetric("price", exact.Float64(), data.UnitPrice, data.TimescaleTick)
				priceMetric.Exact = exact
				trade.Write(priceMetric)
				writer.Add("measurements", data.Publication{Measurement: trade})
			}
		})

		Convey("It detects the excursion and learns from it in the same pass", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(training.classCount("enter"), ShouldBeGreaterThan, 0)
			So(training.classCount("exit"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegionToken(t *testing.T) {
	Convey("Given training component and measurements", t, func() {
		training := &Training{
			grid: store.NewGrid(),
		}

		tok := training.token()
		So(tok, ShouldBeNil)
	})
}

func TestTraining_Step(t *testing.T) {
	Convey("Given a Training instance with an off-ramp worker", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		normalizer := spot.NewNormalizer()
		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		desk := broker.NewDesk(ctx, nil, price)
		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, catalog, storeTee, 1000,
		)

		testUITee := hindsight.NewStoreTee(ctx, "uiTee")
		testUITee.Transition(runtime.READY)
		training.SetUITee(testUITee)

		Convey("Step processes measurement synchronously and returns report", func() {
			measurement := data.NewMeasurement(1000, "BTC/USD", "cvd", 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			priceMetric := data.NewMetric("price", 60000, data.UnitPrice, data.TimescaleTick)
			measurement.Write(priceMetric)

			result := training.Step(measurement)
			So(result, ShouldNotBeNil)
			So(result.Source, ShouldEqual, "training")
			So(result.Label, ShouldEqual, "BTC/USD")
		})
	})
}

func TestTraining_TriadGate(t *testing.T) {
	Convey("Given a Training component with triad gating", t, func() {
		training := &Training{}

		Convey("When resonance surprise is positive and manifold impedance is clear", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 1.5, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 0.1, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(training.authorized(resonanceM, manifoldM), ShouldBeTrue)
		})

		Convey("When resonance surprise is zero (equilibrium churn), entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 0.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous))

			So(training.authorized(resonanceM, manifoldM), ShouldBeFalse)
		})

		Convey("When manifold has complete locked synchronization and opposing pressure, entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 2.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 1.0, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 5.0, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(training.authorized(resonanceM, manifoldM), ShouldBeFalse)
		})
	})
}

func TestTraining_GridCheckpointRoundtrip(t *testing.T) {
	Convey("Given a settled training grid", t, func() {
		training := &Training{grid: store.NewGrid()}

		base := map[string]float64{
			store.CellKey("cvd_value"):       1.0,
			store.CellKey("hawkes_value"):    2.0,
			store.CellKey("depthflow_value"): 3.0,
			store.CellKey("liquidity_value"): 4.0,
		}

		for tick := int64(1); tick <= 8; tick++ {
			step := make(map[string]float64, len(base))
			for key, value := range base {
				step[key] = value + float64(tick)*0.1
			}
			training.grid.Update(tick, step)
		}

		training.grid.Settle()
		So(training.grid.IsSettled(), ShouldBeTrue)

		lit := map[string]float64{
			store.CellKey("cvd_value"):       2.5,
			store.CellKey("hawkes_value"):    3.5,
			store.CellKey("depthflow_value"): 4.5,
			store.CellKey("liquidity_value"): 5.5,
		}
		before := training.grid.LitRegions(lit)
		So(len(before), ShouldBeGreaterThan, 0)

		Convey("Snapshot and RestoreSnapshot preserve settled regions", func() {
			encoded, err := training.grid.Snapshot()
			So(err, ShouldBeNil)
			So(len(encoded), ShouldBeGreaterThan, 0)

			restored := &Training{grid: store.NewGrid()}
			So(restored.grid.IsSettled(), ShouldBeFalse)
			So(restored.grid.RestoreSnapshot(encoded), ShouldBeNil)
			So(restored.grid.IsSettled(), ShouldBeTrue)

			after := restored.grid.LitRegions(lit)
			So(len(after), ShouldEqual, len(before))
			So(string(after[0]), ShouldEqual, string(before[0]))
		})
	})
}

func TestTraining_FragmentMarkers(t *testing.T) {
	Convey("Given a past excursion learned into a fragment", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Trades are sparser than signal frames (no trade at ticks 8, 10,
		// 11), so the sweet-spot markers must map onto real trade ticks.
		training := trainingFixture(
			t, ctx, uniformTape, detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000),
			trades(map[int64]string{
				7: "60200", 9: "60100", 12: "61500", 14: "62500", 15: "63000",
			}),
		)

		Convey("It exposes A/B/C ticks and ENTER/EXIT sweet spots before B and C", func() {
			frags := training.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)

			frag := frags[0]
			So(frag.MarkA, ShouldBeGreaterThan, 0)
			So(frag.MarkB, ShouldEqual, 10)
			So(frag.MarkC, ShouldEqual, 15)
			So(frag.MarkA, ShouldBeLessThan, frag.MarkB)

			So(frag.Points, ShouldHaveLength, 5)
			So(frag.EntryIdx, ShouldBeBetweenOrEqual, 0, len(frag.Points)-1)
			So(frag.ExitIdx, ShouldBeBetweenOrEqual, 0, len(frag.Points)-1)
			So(frag.ExitIdx, ShouldBeGreaterThan, frag.EntryIdx)

			enterSeq := frag.Points[frag.EntryIdx].Tick
			exitSeq := frag.Points[frag.ExitIdx].Tick

			// ENTER fills strictly before ignition B, EXIT strictly between
			// B and exhaustion C, on ticks that actually traded.
			So(enterSeq, ShouldBeGreaterThanOrEqualTo, frag.MarkA)
			So(enterSeq, ShouldBeLessThan, frag.MarkB)
			So(exitSeq, ShouldBeGreaterThan, frag.MarkB)
			So(exitSeq, ShouldBeLessThan, frag.MarkC)
			So(enterSeq, ShouldEqual, 7)
			So(exitSeq, ShouldEqual, 12)

			So(frag.Direction, ShouldEqual, "up")
			So(frag.Magnitude, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDeduplicateTokens(t *testing.T) {
	Convey("Given a frame sequence with repeated region tokens", t, func() {
		frames := [][]byte{
			[]byte("R1"), []byte("R1"), []byte("R1"),
			[]byte("R2"),
			nil,
			[]byte("R2"), []byte(""), []byte("R2"),
			[]byte("R1"),
			[]byte("R3"), []byte("R3"),
		}

		Convey("Consecutive duplicates (also across empty frames) collapse to one transition", func() {
			So(deduplicateTokens(frames), ShouldResemble, [][]byte{
				[]byte("R1"), []byte("R2"), []byte("R1"), []byte("R3"),
			})
		})

		Convey("Holding a region longer does not change the learned context", func() {
			short := bytes.Join(deduplicateTokens([][]byte{
				[]byte("R1"), []byte("R2"), []byte("R3"),
			}), []byte("/"))
			long := bytes.Join(deduplicateTokens([][]byte{
				[]byte("R1"), []byte("R1"), []byte("R1"), []byte("R2"),
				[]byte("R2"), []byte("R3"), []byte("R3"), []byte("R3"),
			}), []byte("/"))

			So(string(long), ShouldEqual, string(short))
			So(string(short), ShouldEqual, "R1/R2/R3")
		})

		Convey("An empty or all-empty sequence yields no context", func() {
			So(deduplicateTokens(nil), ShouldBeEmpty)
			So(deduplicateTokens([][]byte{nil, []byte("")}), ShouldBeEmpty)
		})
	})
}

func TestTraining_LearnAtOffsetsA(t *testing.T) {
	Convey("Given the same excursion learned at two A offsets", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		epoch := int64(100)
		So(catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: time.Now().UTC(),
			Status:    "ACTIVE",
		}), ShouldBeNil)

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName:        "BTC/USD",
					Base:          "BTC",
					Quote:         "USD",
					LotDecimals:   8,
					LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price.SetReferenceCash(decimal.NewFromFloat64(200))

		writer := tables.NewWriter(catalog, epoch)
		detection(excursionUp, 60000, 63000)(writer, epoch)

		for _, tick := range []int64{7, 8, 9, 10, 11, 12, 13, 14, 15} {
			measurement := data.NewMeasurement(epoch, "BTC/USD", "cvd", tick, tick)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			valMetric := data.NewMetric("value", 1.5+float64(tick)*0.1, data.UnitCount, data.TimescaleTick)
			measurement.Write(valMetric)
			writer.Add("measurements", data.Publication{Measurement: measurement})
		}

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, broker.NewDesk(ctx, nil, price), catalog, storeTee, 1000,
		)
		training.Transition(runtime.WAITING)
		// Do not Settle() an empty grid: learn/frames settles from the signal tape.

		var detectionRow *data.Measurement
		for det, err := range catalog.Detections(ctx) {
			So(err, ShouldBeNil)
			detectionRow = det
			break
		}
		So(detectionRow, ShouldNotBeNil)

		asked, _, err := training.learnAt(detectionRow, 0)
		So(err, ShouldBeNil)
		So(asked, ShouldResemble, map[string]int{actionEnter: 1, actionExit: 1})

		asked, _, err = training.learnAt(detectionRow, 2)
		So(err, ShouldBeNil)
		So(asked, ShouldResemble, map[string]int{actionEnter: 1, actionExit: 1})

		frags := training.Fragments()
		So(len(frags), ShouldEqual, 2)
		So(frags[0].MarkA, ShouldNotEqual, frags[1].MarkA)
		So(frags[0].MarkA, ShouldBeLessThan, frags[0].MarkB)
		So(frags[1].MarkA, ShouldBeLessThan, frags[1].MarkB)
		So(training.classCount("enter"), ShouldBeGreaterThan, 0)
		So(training.classCount("exit"), ShouldBeGreaterThan, 0)
	})
}

/*
fillingTransport fills every market order at a fixed unit price with a fixed
fee, echoing the client order ID back through Desk.Apply like Paper does.
*/
type fillingTransport struct {
	desk  *broker.Desk
	unit  map[string]float64
	fee   float64
	write chan struct{}
}

func (transport *fillingTransport) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}
	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}
	quantity, err := decimal.NewFromString(message.Params.Volume)
	if err != nil {
		return err
	}
	unit := transport.unit[message.Params.Type]
	transport.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: message.Params.ClOrdId,
		Symbol:        message.Params.Pair,
		Side:          message.Params.Type,
		LastQty:       quantity,
		Cost:          decimal.NewFromFloat64(quantity.Float64() * unit),
		FeeUsdEquiv:   decimal.NewFromFloat64(transport.fee),
	}}})
	transport.write <- struct{}{}
	return nil
}

func TestTraining_PaperSettleRefines(t *testing.T) {
	Convey("Given a READY training session that paper trades a round trip", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 63000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, catalog, storeTee, 1000,
		)
		training.Transition(runtime.READY)

		held := training.episode("BTC/USD")
		held.setEntry([]byte("R0_R1/R1_R2"))
		held.setExit([]byte("R2_R3/R3_R0"))

		before := training.records()

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		So(training.resolved.Load(), ShouldEqual, 1)
		So(training.records(), ShouldBeGreaterThan, before)
		So(training.classCount("enter"), ShouldBeGreaterThan, 0)
		So(training.classCount("exit"), ShouldBeGreaterThan, 0)
		So(training.getReturn(), ShouldNotEqual, 0)
	})
}

func TestTraining_LosingClasses(t *testing.T) {
	Convey("Given a past up_friction excursion (an ignition that does not clear fees)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionUpShort, 60000, 60010))

		Convey("Its precursor teaches wait and its holding run teaches exit, never enter", func() {
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.classCount(actionExit), ShouldBeGreaterThan, 0)
			So(training.classCount(actionEnter), ShouldEqual, 0)
			So(training.baseline.Load(), ShouldEqual, 1)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionUpShort)
			So(frags[0].EntryIdx, ShouldEqual, -1)
			So(frags[0].ExitIdx, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})

	Convey("Given a past down excursion", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionDown, 63000, 60000))

		Convey("Its precursor before the top teaches wait and the fall teaches exit, never enter", func() {
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.classCount(actionExit), ShouldBeGreaterThan, 0)
			So(training.classCount(actionEnter), ShouldEqual, 0)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionDown)
			So(frags[0].Direction, ShouldEqual, "down")
			So(frags[0].EntryIdx, ShouldEqual, -1)
		})
	})

	Convey("Given a past flat stretch", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detectionAt(excursionFlat, 11, 11, 14, 60000, 60000))

		Convey("It teaches wait with the friction avoided, and opens no paper trading", func() {
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.classCount(actionEnter), ShouldEqual, 0)
			So(training.classCount(actionExit), ShouldEqual, 0)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionFlat)
			So(frags[0].MarkA, ShouldBeGreaterThanOrEqualTo, frags[0].MarkB)
			So(frags[0].EntryIdx, ShouldEqual, -1)
			So(frags[0].ExitIdx, ShouldEqual, -1)
		})
	})

	Convey("Given a winning and a losing excursion stored over the same tape", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(
			t, ctx, regimeTape,
			detection(excursionUp, 60000, 63000),
			detection(excursionUpShort, 60000, 60010),
		)

		Convey("Both are learned and the baseline is the largest single-action share (exit, twice)", func() {
			So(training.classCount(actionEnter), ShouldBeGreaterThan, 0)
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.baseline.Load(), ShouldEqual, 2)
		})
	})

	Convey("Given a stored detection without an excursion class", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection("", 60000, 63000))

		Convey("It is rejected instead of being guessed into a class, and training halts", func() {
			So(training.records(), ShouldEqual, 0)
			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "unknown excursion class")
		})
	})
}

func TestTraining_WaitNeverEnters(t *testing.T) {
	Convey("Given a READY training session whose trie answers a live context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 60000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, tablestest.New(t), storeTee, 1000,
		)

		for tick := int64(1); tick <= 16; tick++ {
			training.grid.Update(tick, map[string]float64{
				store.CellKey("cvd_value"):    1.5 + float64(tick)*0.1,
				store.CellKey("hawkes_value"): 3.0 + float64(tick)*0.2,
			})
		}

		training.grid.Settle()
		training.Transition(runtime.READY)

		live := func() *data.Measurement {
			measurement := data.NewMeasurement(1000, "BTC/USD", "cvd", 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			return measurement.Write(
				data.NewMetric("cvd_value", 2.0, data.UnitCount, data.TimescaleTick),
			)
		}

		tok := training.token(live())
		So(len(tok), ShouldBeGreaterThan, 0)

		teach := func(action string) {
			_, err := training.ask(training.trainer, map[string]string{
				"context": string(tok),
				"class":   action,
			}, map[string]float64{
				"feedback": 0.01,
				"graded":   1,
			})
			So(err, ShouldBeNil)
		}

		Convey("When the trie answers wait", func() {
			teach(actionWait)
			out := training.Step(live())

			Convey("No order is submitted and the desk stays flat", func() {
				So(out, ShouldNotBeNil)

				select {
				case <-transport.write:
					t.Fatal("wait submitted an order")
				case <-time.After(200 * time.Millisecond):
				}

				So(desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
				So(metricRaw(out, "action"), ShouldEqual, 0)
			})
		})

		Convey("When the trie answers enter", func() {
			teach(actionEnter)
			out := training.Step(live())

			Convey("The entry is submitted to the desk", func() {
				select {
				case <-transport.write:
				case <-time.After(time.Second):
					t.Fatal("enter submitted no order")
				}

				So(metricRaw(out, "action"), ShouldEqual, 1)
			})
		})
	})
}

func TestTraining_PaperLossTeachesWait(t *testing.T) {
	Convey("Given a READY training session that paper trades a losing round trip", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 59000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, tablestest.New(t), storeTee, 1000,
		)
		training.Transition(runtime.READY)

		held := training.episode("BTC/USD")
		held.setEntry([]byte("R0_R1/R1_R2"))
		held.setExit([]byte("R2_R3/R3_R0"))

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		Convey("The losing entry context teaches wait, never enter", func() {
			So(training.getReturn(), ShouldBeLessThan, 0)
			So(training.classCount(actionWait), ShouldBeGreaterThan, 0)
			So(training.classCount(actionEnter), ShouldEqual, 0)

			reading, err := training.ask(training.recall, map[string]string{"context": "R0_R1/R1_R2"}, nil)
			So(err, ShouldBeNil)
			winner, _, err := readText(reading, "winner")
			So(err, ShouldBeNil)
			So(winner, ShouldEqual, actionWait)
		})
	})
}

func TestTraining_PaperLossKeepsExit(t *testing.T) {
	Convey("Given a READY training session whose exit context competes with wait", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 59000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, tablestest.New(t), storeTee, 1000,
		)
		training.Transition(runtime.READY)

		held := training.episode("BTC/USD")
		held.setEntry([]byte("R0_R1/R1_R2"))
		held.setExit([]byte("R2_R3/R3_R0"))

		/*
			A barely-positive wait competes on the exit context, so the losing
			settle decides the winner: exit must be graded by the loss it closed,
			not inhibited by it.
		*/
		_, err := training.ask(training.trainer, map[string]string{
			"context": "R2_R3/R3_R0",
			"class":   actionWait,
		}, map[string]float64{
			"feedback": 1e-4,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		Convey("The exit context that closed the loss still prefers exit", func() {
			So(training.getReturn(), ShouldBeLessThan, -1e-4)

			reading, err := training.ask(training.recall, map[string]string{"context": "R2_R3/R3_R0"}, nil)
			So(err, ShouldBeNil)
			winner, _, err := readText(reading, "winner")
			So(err, ShouldBeNil)
			So(winner, ShouldEqual, actionExit)
		})
	})
}

func TestTraining_MissingTapeHalts(t *testing.T) {
	Convey("Given a stored detection whose run has no signal/logic tape", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("learnAt errors instead of reporting nothing to learn", func() {
			training := trainingSetup(t, ctx, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			asked, _, err := training.learnAt(detectionRow, 0)
			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "no signal/logic tape")
			So(asked, ShouldBeNil)
			So(training.Fragments(), ShouldBeEmpty)
		})

		Convey("The training pass fails Internal and closes the context cmd watches", func() {
			training := trainingFixture(t, ctx, detection(excursionUp, 60000, 63000))

			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "failed during training pass")
			So(training.Error().Error(), ShouldContainSubstring, "no signal/logic tape")
			So(training.records(), ShouldEqual, 0)
		})
	})

	Convey("Given a stored detection whose signal/logic rows light no grid region", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Rows exist for every frame, but carry no numeric channel the grid
		// could place, so no frame produces a region token.
		blank := func(writer *tables.Writer, epoch int64) {
			for tick := int64(7); tick <= 15; tick++ {
				measurement := data.NewMeasurement(
					epoch, "BTC/USD", "cvd", tick*10, tick,
					&data.StringEntry{Key: "note", Value: "blank"},
				)
				measurement.At = time.Now().UTC()
				measurement.From = measurement.At
				// Write finalizes the row (WORM); it carries no metric.
				writer.Add("measurements", data.Publication{Measurement: measurement.Write()})
			}
		}

		training := trainingSetup(t, ctx, blank, detection(excursionUp, 60000, 63000))
		detectionRow := firstDetection(ctx, training)
		So(detectionRow, ShouldNotBeNil)

		_, _, err := training.learnAt(detectionRow, 0)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "lights no grid region")
	})
}

func TestTraining_ShortPhasesStaySoft(t *testing.T) {
	cases := []struct {
		name string
		tape func(*tables.Writer, int64)
	}{
		// B=10, C=11: after the fill pullback the holding run keeps no frame.
		{"a holding run too short to leave a frame", detectionAt(excursionUp, 7, 10, 11, 60000, 63000)},
		// Ignites on the first tick of its tape: no precursor at all.
		{"an excursion igniting on its first tick", detectionAt(excursionUp, 10, 10, 15, 60000, 63000)},
		// Tape exists from tick 7, but no frame falls before ignition B=7.
		{"no token frame before ignition", detectionAt(excursionUp, 5, 7, 15, 60000, 63000)},
	}

	for _, tc := range cases {
		Convey("Given a stored detection with "+tc.name+" over an existing tape", t, func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			Convey("learnAt reports nothing to learn without an error", func() {
				training := trainingSetup(t, ctx, regimeTape, tc.tape)
				detectionRow := firstDetection(ctx, training)
				So(detectionRow, ShouldNotBeNil)

				asked, _, err := training.learnAt(detectionRow, 0)
				So(err, ShouldBeNil)
				So(asked, ShouldBeEmpty)
				So(training.Fragments(), ShouldBeEmpty)
			})

			Convey("The training pass completes, stays WAITING, and keeps its context open", func() {
				training := trainingFixture(t, ctx, regimeTape, tc.tape)

				So(training.Status(), ShouldEqual, runtime.WAITING)
				So(training.Error(), ShouldBeNil)
				So(training.Context().Err(), ShouldBeNil)
				So(training.records(), ShouldEqual, 0)
			})
		})
	}
}

func TestTraining_CatalogReadFailureHalts(t *testing.T) {
	Convey("Given a stored excursion over its signal tape whose data files then fail to read", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("learnAt reports the read failure, not a missing tape", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			asked, _, err := training.learnAt(detectionRow, 0)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read signal/logic tape")
			So(err.Error(), ShouldContainSubstring, "[iceberg]")
			So(err.Error(), ShouldNotContainSubstring, "no signal/logic tape")
			So(asked, ShouldBeNil)
			So(training.Fragments(), ShouldBeEmpty)
		})

		Convey("priceTape returns the read failure instead of an empty or fabricated tape", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			startTick, _, cTick, err := tables.DetectionTicks(detectionRow)
			So(err, ShouldBeNil)

			// Healthy storage reads the stored trades, so the failure below
			// is the read itself, not the shape of the window.
			points, err := training.priceTape(detectionRow, startTick, cTick)
			So(err, ShouldBeNil)
			So(points, ShouldNotBeEmpty)

			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			points, err = training.priceTape(detectionRow, startTick, cTick)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read price tape")
			So(err.Error(), ShouldContainSubstring, "[iceberg]")
			So(points, ShouldBeNil)
		})

		Convey("A training pass fails instead of grading the trie on no excursions", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			trained, _, seen, _, _, err := training.trainPass()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read stored detections")
			So(trained, ShouldEqual, 0)
			So(seen, ShouldEqual, 0)
		})

		Convey("Train halts Internal in the detector scan, before any pass, and records nothing", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			training.Train()

			// detectorDone closes after runDetectorScan's Error returns.
			// The context closes inside that call, before runtime.Close
			// finishes joining closer errors, so waiting on it would race.
			select {
			case <-training.detectorDone:
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the detector scan to halt")
			}

			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "unable to read detections for detector scan")
			So(training.Error().Error(), ShouldContainSubstring, "[iceberg]")
			So(training.Passes(), ShouldEqual, 0)
			So(training.records(), ShouldEqual, 0)
		})
	})
}

func TestTraining_RestoreGridUnconfiguredStorage(t *testing.T) {
	Convey("Given training whose catalog has no object storage", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := &Training{
			System:  runtime.NewSystem(ctx, "training"),
			grid:    store.NewGrid(),
			catalog: tablestest.New(t),
		}

		Convey("restoreGrid reports no checkpoint, not an error, every time", func() {
			for range 3 {
				restored, err := training.restoreGrid()
				So(err, ShouldBeNil)
				So(restored, ShouldBeFalse)
			}
		})
	})

	Convey("Given training with no catalog", t, func() {
		training := &Training{grid: store.NewGrid()}

		Convey("restoreGrid reports no checkpoint", func() {
			restored, err := training.restoreGrid()
			So(err, ShouldBeNil)
			So(restored, ShouldBeFalse)
		})
	})
}

func TestChannelsFrom_KeysByMetricLabel(t *testing.T) {
	Convey("Given the same metric from two symbols and two producers", t, func() {
		at := time.Now().UTC()
		write := func(label, source string, raw float64) *data.Measurement {
			measurement := data.NewMeasurement(1, label, source, 1, 1)
			measurement.At = at
			measurement.From = at
			return measurement.Write(
				data.NewMetric("spread", raw, data.UnitCount, data.TimescaleTick),
			)
		}

		channels := channelsFrom(
			write("BTC/USD", "liquidity", 1),
			write("ETH/USD", "pumpdump", 2),
		)

		Convey("They map onto one cell keyed by the metric label alone", func() {
			So(channels, ShouldHaveLength, 1)
			So(channels, ShouldContainKey, store.CellKey("spread"))
		})
	})
}

func TestChannelsFrom_CollapsesPeerQualifiedFacts(t *testing.T) {
	Convey("Given one correlation fact published against three peer symbols", t, func() {
		at := time.Now().UTC()
		measurement := data.NewMeasurement(1, "BTC/USD", "correlation", 1, 1)
		measurement.At = at
		measurement.From = at
		measurement = measurement.Write(
			data.NewMetric("signed_correlation@ETH/USD", 0.2, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("signed_correlation@SOL/USD", 0.4, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("signed_correlation@XRP/USD", 0.9, data.UnitCorrelation, data.TimescaleRollingWindow),
			data.NewMetric("cohort_peer_count", 3, data.UnitCount, data.TimescaleInstantaneous),
		)

		channels := channelsFrom(measurement)

		Convey("They land in one fact cell holding the mean across peers", func() {
			So(channels, ShouldHaveLength, 2)
			So(channels["signed_correlation"], ShouldAlmostEqual, 0.5, 1e-12)
			So(channels["cohort_peer_count"], ShouldEqual, 3)
		})
	})
}
