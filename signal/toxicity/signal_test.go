package toxicity

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
	levelfixtures "github.com/theapemachine/symm/tests/fixtures/level3"
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

func l3Order(event, id string, price, quantity float64) string {
	if event == "" {
		return fmt.Sprintf(
			`{"order_id":%q,"limit_price":%g,"order_qty":%g,"timestamp":"2026-05-30T12:00:00Z"}`,
			id,
			price,
			quantity,
		)
	}

	return fmt.Sprintf(
		`{"event":%q,"order_id":%q,"limit_price":%g,"order_qty":%g,"timestamp":"2026-05-30T12:00:00Z"}`,
		event,
		id,
		price,
		quantity,
	)
}

func level3Frame(symbol, bids, asks string) string {
	return fmt.Sprintf(
		`{"channel":"level3","type":"update","data":[{"symbol":"%s","bids":[%s],"asks":[%s]}]}`,
		symbol,
		bids,
		asks,
	)
}

func tradeFrame(symbol, side string, price, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"%s","side":%q,"price":%g,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol,
		side,
		price,
		quantity,
	)
}

func toxicityClassified(result *datura.Artifact) bool {
	return datura.Peek[string](result, "root") == "output" &&
		datura.Peek[float64](result, "output", "confidence") > 0 &&
		datura.Peek[float64](result, "output", "strength") > 0
}

func toxicityCategory(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

func toxicityStructuredPayload(result *datura.Artifact) bool {
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
			if !toxicityClassified(measurement) {
				continue
			}

			result = measurement
		}

		artifact.Release()
	}

	return result
}

func appendVacuumWarmup(frames []struct {
	role    string
	payload string
}, index int) []struct {
	role    string
	payload string
} {
	symbol := "BTC/USD"

	return append(frames,
		struct {
			role    string
			payload string
		}{"level3", level3Frame(symbol,
			l3Order("add", fmt.Sprintf("BF-%d", index), 100, 100)+","+
				l3Order("add", fmt.Sprintf("BC-%d", index), 99.9, 10),
			l3Order("add", fmt.Sprintf("A-%d", index), 101, 100),
		)},
		struct {
			role    string
			payload string
		}{"trade", tradeFrame(symbol, "buy", 100, 100)},
		struct {
			role    string
			payload string
		}{"level3", level3Frame(symbol,
			l3Order("delete", fmt.Sprintf("BF-%d", index), 100, 100)+","+
				l3Order("delete", fmt.Sprintf("BC-%d", index), 99.9, 10),
			"",
		)},
	)
}

func TestSignalRoleContract(testingTB *testing.T) {
	Convey("Given a toxicity signal", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		Convey("It declares level3 and trade ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"level3", "trade"})
		})

		Convey("It ignores L2 book artifacts", func() {
			count := 0

			for artifact := range bookfixtures.NewFixture(bookfixtures.UPDATE, 1).Artifacts() {
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
	Convey("Given level3 and trade fixtures", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		count := 0

		measure := func(artifacts iter.Seq[*datura.Artifact]) {
			for artifact := range artifacts {
				for measurement := range signal.Measure(artifact, nil) {
					count++

					role := datura.Peek[string](measurement, "role")
					scope, scopeErr := measurement.Scope()
					origin, originErr := measurement.Origin()

					So(role, ShouldEqual, "measurement")
					So(toxicityStructuredPayload(measurement), ShouldBeTrue)
					So(scopeErr, ShouldBeNil)
					So(scope, ShouldNotEqual, "")
					So(originErr, ShouldBeNil)
					So(origin, ShouldEqual, string(logic.SourceToxicity))

					if toxicityClassified(measurement) {
						So(logic.Categories[toxicityCategory(measurement)], ShouldNotEqual, logic.CategoryTypeNone)
						So(datura.Peek[float64](measurement, "output", "l3"), ShouldEqual, 1)
					}
				}

				artifact.Release()
			}
		}

		measure(levelfixtures.NewFixture(levelfixtures.SNAPSHOT, 1).Artifacts())
		measure(tradefixtures.NewFixture(tradefixtures.UPDATE, 3).Artifacts())
		measure(levelfixtures.NewFixture(levelfixtures.UPDATE, 3).Artifacts())

		Convey("It routes fixture rows into toxicity measurement artifacts", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a near-touch L3 block that disappears without trade", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"level3", level3Frame("BTC/USD",
				l3Order("", "B1", 100, 100)+","+l3Order("", "B2", 100, 20),
				l3Order("", "A1", 101, 100),
			)},
			{"level3", level3Frame("BTC/USD",
				l3Order("delete", "B1", 100, 100),
				"",
			)},
		}

		result := replay(signal, frames)

		Convey("It classifies toxic bluff from L3 cancel evidence", func() {
			So(result, ShouldNotBeNil)
			So(toxicityCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryToxicBluff))
			So(datura.Peek[float64](result, "output", "bluffScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "l3"), ShouldEqual, 1)
		})
	})

	Convey("Given one side retreating much faster than it fills", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{}

		for index := 0; index < 3; index++ {
			frames = appendVacuumWarmup(frames, index)
		}

		frames = append(frames,
			struct {
				role    string
				payload string
			}{"level3", level3Frame("BTC/USD",
				l3Order("add", "BF-final", 100, 10)+","+
					l3Order("add", "BC-final", 99.9, 100),
				l3Order("add", "A-final", 101, 100),
			)},
			struct {
				role    string
				payload string
			}{"trade", tradeFrame("BTC/USD", "buy", 100, 10)},
			struct {
				role    string
				payload string
			}{"level3", level3Frame("BTC/USD",
				l3Order("delete", "BF-final", 100, 10)+","+
					l3Order("delete", "BC-final", 99.9, 100),
				"",
			)},
		)

		result := replay(signal, frames)

		Convey("It classifies liquidity vacuum from L3 cancel/fill asymmetry", func() {
			So(result, ShouldNotBeNil)
			So(toxicityCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLiquidityVacuum))
			So(datura.Peek[float64](result, "output", "vacuumScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "l3"), ShouldEqual, 1)
		})
	})

	Convey("Given touch liquidity that fills without cancels", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{
			{"level3", level3Frame("BTC/USD",
				l3Order("", "B1", 100, 20)+","+l3Order("", "B2", 99.9, 100),
				l3Order("", "A1", 101, 100),
			)},
			{"trade", tradeFrame("BTC/USD", "buy", 100, 20)},
			{"level3", level3Frame("BTC/USD",
				l3Order("delete", "B1", 100, 20),
				"",
			)},
		}

		result := replay(signal, frames)

		Convey("It classifies hard support from filled liquidity", func() {
			So(result, ShouldNotBeNil)
			So(toxicityCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryHardSupport))
			So(datura.Peek[float64](result, "output", "supportScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "l3"), ShouldEqual, 1)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	frames := []struct {
		role    string
		payload string
	}{
		{"level3", level3Frame("BTC/USD",
			l3Order("", "B1", 100, 100)+","+l3Order("", "B2", 100, 20),
			l3Order("", "A1", 101, 100),
		)},
		{"level3", level3Frame("BTC/USD",
			l3Order("delete", "B1", 100, 100),
			"",
		)},
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		result := replay(signal, frames)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
