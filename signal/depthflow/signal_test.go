package depthflow

import (
	"context"
	"fmt"
	"iter"
	"strings"
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

func bookFrame(name string, bidQty, askQty float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"%s","bids":[{"price":100,"qty":%g},{"price":99,"qty":%g}],"asks":[{"price":101,"qty":%g},{"price":102,"qty":%g}]}]}`,
		name,
		bidQty,
		bidQty*0.9,
		askQty,
		askQty*0.8,
	)
}

func spoofFrame(name string) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"%s","bids":[{"price":100,"qty":1},{"price":99,"qty":500},{"price":98,"qty":500}],"asks":[{"price":101,"qty":50},{"price":102,"qty":10}]}]}`,
		name,
	)
}

func tradeFrame(name, side string, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"%s","side":%q,"price":100.5,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		name,
		side,
		quantity,
	)
}

func classified(result *datura.Artifact) bool {
	return datura.Peek[string](result, "root") == "output" &&
		datura.Peek[float64](result, "output", "confidence") > 0
}

func depthflowCategory(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

func structuredPayload(result *datura.Artifact) bool {
	payload := strings.TrimSpace(string(result.DecryptPayload()))

	return payload != "" && payload[:1] == "{"
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
	Convey("Given a depthflow signal", testingTB, func() {
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
		classifiedCount := 0

		measure := func(artifacts iter.Seq[*datura.Artifact]) {
			for artifact := range artifacts {
				for measurement := range signal.Measure(artifact, nil) {
					count++

					role := datura.Peek[string](measurement, "role")
					scope, scopeErr := measurement.Scope()
					origin, originErr := measurement.Origin()

					So(role, ShouldEqual, "measurement")
					So(structuredPayload(measurement), ShouldBeTrue)
					So(scopeErr, ShouldBeNil)
					So(scope, ShouldNotEqual, "")
					So(originErr, ShouldBeNil)
					So(origin, ShouldEqual, string(logic.SourceDepthFlow))

					if classified(measurement) {
						classifiedCount++
						So(logic.Categories[depthflowCategory(measurement)], ShouldNotEqual, logic.CategoryTypeNone)
						So(datura.Peek[float64](measurement, "output", "strength"), ShouldBeGreaterThan, 0)
					}
				}

				artifact.Release()
			}
		}

		measure(bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts())
		measure(tradefixtures.NewFixture(tradefixtures.UPDATE, 8).Artifacts())
		measure(bookfixtures.NewFixture(bookfixtures.UPDATE, 8).Artifacts())

		Convey("It emits measurement artifacts and classified depthflow output", func() {
			So(count, ShouldBeGreaterThan, 0)
			So(classifiedCount, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given bid-heavy depth confirmed by buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"trade", tradeFrame("BTC/USD", "buy", 4)},
			{"book", bookFrame("BTC/USD", 20, 8)},
		}

		result := replay(signal, frames)

		Convey("It classifies loaded imbalance", func() {
			So(result, ShouldNotBeNil)
			So(depthflowCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLoadedImbalance))
			So(datura.Peek[float64](result, "output", "loadedScore"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid weight contradicted by bearish touch pressure", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", bookFrame("BTC/USD", 20, 8)},
			{"book", spoofFrame("BTC/USD")},
			{"trade", tradeFrame("BTC/USD", "sell", 8)},
			{"book", spoofFrame("BTC/USD")},
		}

		result := replay(signal, frames)

		Convey("It classifies spoof trap", func() {
			So(result, ShouldNotBeNil)
			So(depthflowCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategorySpoofTrap))
			So(datura.Peek[float64](result, "output", "spoofScore"), ShouldBeGreaterThan, 0)
		})
	})
}
