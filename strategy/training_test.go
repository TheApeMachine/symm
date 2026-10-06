package strategy

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
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
		ctx, price, broker.NewDesk(ctx, nil, price), catalog, storeTee, 1000,
	)

	// Develop the grid on the run's sensory tape through the live path,
	// then freeze it: rehearsal only reads tokens from a settled grid.
	for measurement, err := range catalog.SignalLogic(ctx, epoch, "BTC/USD", 0, math.MaxInt64) {
		So(err, ShouldBeNil)
		training.impulse.observe(measurement)
	}

	training.impulse.grid.Settle()
	training.Transition(runtime.WAITING)

	return training
}

/*
census reads one figure from the training's trie census.
*/
func census(training *Training, key string) float64 {
	value, err := training.Model.Count(key)
	So(err, ShouldBeNil)
	return value
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
graded reads the rehearsal's published grade.
*/
func graded(training *Training) *skill {
	grade := training.Rehearsal.grade.Load()
	So(grade, ShouldNotBeNil)
	return grade
}

/*
sensor writes one producer's rows for the fixture run through an ArenaOwner
of its own, the way a live producer finalizes them, so every row after the
first carries a real SNR and Maturity. The producer idles from tick 1 up to
its first active tick at that tick's level, alternating by a hair (phase
picks the alternation, so idle producers of opposite phase do not co-move);
the idle stretch is every cell's noise floor. On its active ticks the value
rises with the tick, so each active move stands far above that floor.
*/
func sensor(writer *tables.Writer, epoch int64, source string, scale float64, phase int64, active ...int64) {
	level := func(tick int64) float64 { return scale * (1.5 + float64(tick)*0.1) }

	var prior *data.Measurement

	write := func(tick int64, value float64) {
		var measurement *data.Measurement
		if prior == nil {
			measurement = data.NewMeasurement(epoch, "BTC/USD", source, tick*10, tick)
		} else {
			measurement = prior.Next(source)
			measurement.SeqIdx = tick * 10
			measurement.Tick = tick
		}
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		measurement.Write(data.NewMetric(source+"_value", value, data.UnitCount, data.TimescaleTick))
		prior = measurement
		writer.Add("measurements", measurement)
	}

	for tick := int64(1); tick < active[0]; tick++ {
		write(tick, level(active[0])+scale*1e-4*float64((tick+phase)%2))
	}

	for _, tick := range active {
		write(tick, level(tick))
	}
}

/*
uniformTape is an excursion tape (ignition B=10, peak C=15) on which four
sources co-move on every tick, so the grid settles them into a single region
and the precursor and the holding run share one token.
*/
func uniformTape(writer *tables.Writer, epoch int64) {
	for _, source := range []string{"cvd", "hawkes", "depthflow", "liquidity"} {
		sensor(writer, epoch, source, 1, 0, 7, 8, 9, 10, 11, 12, 13, 14, 15)
	}
}

var (
	precursorSources = []string{"cvd", "hawkes", "sentiment", "toxicity"}
	holdingSources   = []string{"depthflow", "liquidity", "morphology", "pumpdump"}
)

/*
regimeTape is an excursion tape (ignition B=10, peak C=15) in which one group
of sources co-moves through the precursor and another group co-moves through
the holding run, so the settled grid gives the two phases distinct tokens.
The groups idle in opposite phase, so they never co-move.
*/
func regimeTape(writer *tables.Writer, epoch int64) {
	for index, source := range precursorSources {
		sensor(writer, epoch, source, float64(index+1), 0, 7, 8, 9, 10, 15)
	}

	for index, source := range holdingSources {
		sensor(writer, epoch, source, float64(index+1), 1, 10, 11, 12, 13, 14, 15)
	}
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
			writer.Add("measurements", trade)
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
		writer.Add("measurements", measurement)
	}
}

func TestTraining_Train(t *testing.T) {
	Convey("Given a past run with a stored excursion whose phases the grid tells apart", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))

		Convey("It loads enter and exit associations, beats the constant-policy baseline, and opens trading", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(census(training, "records"), ShouldBeGreaterThan, 0)
			So(census(training, "enter"), ShouldBeGreaterThan, 0)
			So(census(training, "exit"), ShouldBeGreaterThan, 0)
			So(graded(training).baseline(), ShouldEqual, 1)
			So(graded(training).hits, ShouldBeGreaterThan, graded(training).baseline())
		})
	})

	Convey("Given a past run with a stored excursion whose precursor and holding run share one token", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, uniformTape, detection(excursionUp, 60000, 63000))

		Convey("It learns records but keeps the skill gate closed and stays in WAITING", func() {
			So(census(training, "records"), ShouldBeGreaterThan, 0)
			So(census(training, "enter"), ShouldBeGreaterThan, 0)
			So(census(training, "exit"), ShouldBeGreaterThan, 0)
			So(graded(training).baseline(), ShouldEqual, 1)
			So(graded(training).hits, ShouldBeLessThanOrEqualTo, graded(training).baseline())
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
			So(training.paper.desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
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
				writer.Add("measurements", trade)
			}
		})

		Convey("It dampens enter on the deadband tape (no wait leaf) and remains in WAITING", func() {
			So(training.Status(), ShouldEqual, runtime.WAITING)
			So(census(training, "records"), ShouldBeGreaterThan, 0)
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			// Dampen-only phases are not skill-graded (no enter/exit truth).
			So(graded(training).trained, ShouldEqual, 0)

			So(len(training.Rehearsal.Chart.Fragments()), ShouldBeGreaterThan, 0)
			for _, fragment := range training.Rehearsal.Chart.Fragments() {
				So(fragment.Class, ShouldNotEqual, excursionUp)
				So(fragment.EntryIdx, ShouldEqual, -1)
				So(fragment.ExitIdx, ShouldEqual, -1)
				So(fragment.PredictedEntryIdx, ShouldEqual, -1)
				So(fragment.PredictedExitIdx, ShouldEqual, -1)
				So(fragment.MarkA, ShouldBeLessThan, fragment.MarkB)
				So(fragment.MarkB, ShouldBeLessThan, fragment.MarkC)
			}
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
				writer.Add("measurements", trade)
			}
		})

		Convey("It detects the excursion and learns from it in the same pass", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(census(training, "enter"), ShouldBeGreaterThan, 0)
			So(census(training, "exit"), ShouldBeGreaterThan, 0)
		})
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
			ctx, price, desk, catalog, storeTee, 1000,
		)

		testUITee := hindsight.NewStoreTee(ctx, "uiTee")
		testUITee.Transition(runtime.READY)
		training.Reporter.SetTee(testUITee)

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

func TestTraining_LosingClasses(t *testing.T) {
	Convey("Given a past up_friction excursion (an ignition that does not clear fees)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionUpShort, 60000, 60010))

		Convey("Its precursor dampens enter — never wait leaf or exit without enter", func() {
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionExit), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(graded(training).trained, ShouldEqual, 0)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Rehearsal.Chart.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionUpShort)
			So(frags[0].MarkA, ShouldBeLessThan, frags[0].MarkB)
			So(frags[0].MarkB, ShouldBeLessThan, frags[0].MarkC)
			So(frags[0].EntryIdx, ShouldEqual, -1)
			So(frags[0].ExitIdx, ShouldEqual, -1)
			So(frags[0].PredictedEntryIdx, ShouldEqual, -1)
			So(frags[0].PredictedExitIdx, ShouldEqual, -1)
		})
	})

	Convey("Given a past down excursion", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection(excursionDown, 63000, 60000))

		Convey("Its precursor before the top dampens enter — never wait leaf or exit", func() {
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionExit), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Rehearsal.Chart.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionDown)
			So(frags[0].Direction, ShouldEqual, "down")
			So(frags[0].EntryIdx, ShouldEqual, -1)
			So(frags[0].ExitIdx, ShouldEqual, -1)
		})
	})

	Convey("Given a past flat stretch", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detectionAt(excursionFlat, 8, 11, 14, 60000, 60000))

		Convey("It dampens enter and opens no paper trading without an enter phase", func() {
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(census(training, actionExit), ShouldEqual, 0)
			So(training.Status(), ShouldEqual, runtime.WAITING)

			frags := training.Rehearsal.Chart.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)
			So(frags[0].Class, ShouldEqual, excursionFlat)
			So(frags[0].MarkA, ShouldBeLessThan, frags[0].MarkB)
			So(frags[0].MarkB, ShouldBeLessThan, frags[0].MarkC)
			So(frags[0].EntryIdx, ShouldEqual, -1)
			So(frags[0].ExitIdx, ShouldEqual, -1)
			So(frags[0].PredictedEntryIdx, ShouldEqual, -1)
			So(frags[0].PredictedExitIdx, ShouldEqual, -1)
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

		Convey("Both are learned; up teaches enter+exit, up_friction dampens enter (no wait leaf)", func() {
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(census(training, actionExit), ShouldBeGreaterThan, 0)
			So(census(training, actionWait), ShouldEqual, 0)
			So(graded(training).asked[actionEnter], ShouldEqual, graded(training).asked[actionExit])
			So(graded(training).baseline(), ShouldEqual, 1)
		})
	})

	Convey("Given a stored detection without an excursion class", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, regimeTape, detection("", 60000, 63000))

		Convey("It is rejected instead of being guessed into a class, and training halts", func() {
			So(census(training, "records"), ShouldEqual, 0)
			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "unknown excursion class")
		})
	})
}
