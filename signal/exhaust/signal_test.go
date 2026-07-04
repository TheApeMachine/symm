package exhaust

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	bookfixtures "github.com/theapemachine/symm/tests/fixtures/book"
	tickerfixtures "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixtures "github.com/theapemachine/symm/tests/fixtures/trade"
)

func datapoint(role, payload string, timestamp int64) *datura.Artifact {
	artifact := datura.Acquire("fixture", datura.APPJSON)
	artifact.WithRole(role)
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func bookFrame(symbol string, bid, ask, bidQty, askQty float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"%s","bids":[{"price":%g,"qty":%g}],"asks":[{"price":%g,"qty":%g}]}]}`,
		symbol,
		bid,
		bidQty,
		ask,
		askQty,
	)
}

func spreadFrame(symbol string, ask float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"%s","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":0},{"price":%g,"qty":10}]}]}`,
		symbol,
		ask,
	)
}

func tradeFrame(symbol, side string, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"%s","side":%q,"price":100,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol,
		side,
		quantity,
	)
}

func classified(result *datura.Artifact) bool {
	return datura.Peek[string](result, "root") == "output" &&
		datura.Peek[float64](result, "output", "confidence") > 0
}

func exhaustCategory(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

func replay(signal *Signal, frames []struct {
	role    string
	payload string
}) *datura.Artifact {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
	var result *datura.Artifact

	for index, frame := range frames {
		artifact := datapoint(frame.role, frame.payload, base+int64(index))

		for measurement := range signal.Measure(artifact, nil) {
			if !classified(measurement) {
				continue
			}

			result = measurement
		}

		artifact.Release()
	}

	return result
}

func TestSignalRoleContract(testingTB *testing.T) {
	Convey("Given an exhaust signal", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		Convey("It declares only book and trade ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"book", "trade"})
		})

		Convey("It ignores ticker artifacts", func() {
			count := 0

			for artifact := range tickerfixtures.NewFixture(tickerfixtures.UPDATE, 1).Artifacts() {
				for range signal.Measure(artifact, nil) {
					count++
				}

				artifact.Release()
			}

			So(count, ShouldEqual, 0)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given book and trade fixtures", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		count := 0

		for artifact := range bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts() {
			for measurement := range signal.Measure(artifact, nil) {
				count++
				So(datura.Peek[string](measurement, "role"), ShouldEqual, "measurement")
				scope, scopeErr := measurement.Scope()
				origin, originErr := measurement.Origin()
				So(scopeErr, ShouldBeNil)
				So(scope, ShouldNotEqual, "")
				So(originErr, ShouldBeNil)
				So(origin, ShouldEqual, string(logic.SourceExhaustion))
				So(datura.Peek[float64](measurement, "output", "value"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "confidence"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "entry_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "exit_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[[]float64](measurement, "output", "probabilities"), ShouldHaveLength, 4)
			}

			artifact.Release()
		}

		for artifact := range tradefixtures.NewFixture(tradefixtures.UPDATE, 8).Artifacts() {
			for measurement := range signal.Measure(artifact, nil) {
				count++
				So(datura.Peek[float64](measurement, "output", "value"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "confidence"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "entry_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "exit_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[[]float64](measurement, "output", "probabilities"), ShouldHaveLength, 4)
			}

			artifact.Release()
		}

		for artifact := range bookfixtures.NewFixture(bookfixtures.UPDATE, 8).Artifacts() {
			for measurement := range signal.Measure(artifact, nil) {
				count++
				So(datura.Peek[float64](measurement, "output", "value"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "confidence"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "entry_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](measurement, "output", "exit_baseline"), ShouldBeGreaterThan, 0)
				So(datura.Peek[[]float64](measurement, "output", "probabilities"), ShouldHaveLength, 4)
				if classified(measurement) {
					So(logic.Categories[exhaustCategory(measurement)], ShouldNotEqual, logic.CategoryTypeNone)
					So(datura.Peek[float64](measurement, "output", "urgency"), ShouldBeGreaterThanOrEqualTo, 0)
				}
			}

			artifact.Release()
		}

		Convey("It emits measurement artifacts from fixture-backed book and trade rows", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given crumbling bid-side depth", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 100, 101, 20, 20)},
			{"book", bookFrame("BTC/USD", 100, 101, 18, 18)},
			{"book", bookFrame("BTC/USD", 100, 101, 16, 16)},
			{"book", bookFrame("BTC/USD", 100, 101, 14, 14)},
			{"book", bookFrame("BTC/USD", 100, 101, 12, 12)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 8, 8)},
			{"book", bookFrame("BTC/USD", 100, 101, 6, 6)},
		}

		result := replay(signal, frames)

		Convey("It classifies mechanical collapse", func() {
			So(result, ShouldNotBeNil)
			So(exhaustCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryMechanicalCollapse))
			So(datura.Peek[float64](result, "output", "mechanical"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given widening spread without book collapse", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", spreadFrame("BTC/USD", 104)},
		}

		result := replay(signal, frames)

		Convey("It classifies fragile expansion", func() {
			So(result, ShouldNotBeNil)
			So(exhaustCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryFragileExpansion))
			So(datura.Peek[float64](result, "output", "fragile"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given support-side imbalance flipping against the position", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 100, 101, 20, 5)},
			{"book", bookFrame("BTC/USD", 100, 101, 20, 5)},
			{"book", bookFrame("BTC/USD", 100, 101, 20, 5)},
			{"book", bookFrame("BTC/USD", 100, 101, 20, 5)},
			{"book", bookFrame("BTC/USD", 100, 101, 5, 20)},
		}

		result := replay(signal, frames)

		Convey("It classifies active reversal", func() {
			So(result, ShouldNotBeNil)
			So(exhaustCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryActiveReversal))
			So(datura.Peek[float64](result, "output", "reversal"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given fading aggressive buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"trade", tradeFrame("BTC/USD", "buy", 20)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"trade", tradeFrame("BTC/USD", "buy", 18)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"trade", tradeFrame("BTC/USD", "buy", 16)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"trade", tradeFrame("BTC/USD", "buy", 4)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"trade", tradeFrame("BTC/USD", "buy", 1)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
		}

		result := replay(signal, frames)

		Convey("It classifies thermal exhaustion", func() {
			So(result, ShouldNotBeNil)
			So(exhaustCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryThermalExhaustion))
			So(datura.Peek[float64](result, "output", "thermal"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureStableBook(testingTB *testing.T) {
	Convey("Given stable book history without decay", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
			{"book", bookFrame("BTC/USD", 100, 101, 10, 10)},
		}
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		var last *datura.Artifact

		for index, frame := range frames {
			artifact := datapoint(frame.role, frame.payload, base+int64(index))

			for measurement := range signal.Measure(artifact, nil) {
				last = measurement
			}

			artifact.Release()
		}

		Convey("It emits the random-baseline contract", func() {
			So(last, ShouldNotBeNil)
			So(datura.Peek[float64](last, "output", "category"), ShouldEqual, logic.CategoryIndex(logic.CategoryMechanicalCollapse))
			So(datura.Peek[float64](last, "output", "confidence"), ShouldAlmostEqual, 0.25, 1e-12)
			So(datura.Peek[float64](last, "output", "entry_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
			So(datura.Peek[float64](last, "output", "exit_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
			So(datura.Peek[[]float64](last, "output", "probabilities"), ShouldResemble, []float64{0.25, 0.25, 0.25, 0.25})
		})
	})
}
