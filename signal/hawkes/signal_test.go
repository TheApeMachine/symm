package hawkes

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/tests/fixtures/book"
	"github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/tests/fixtures/trade"
)

func TestSignalRoleContract(t *testing.T) {
	Convey("Given a Hawkes signal", t, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		Convey("When ingest roles are requested", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"trade"})
		})

		type ignoredCase struct {
			name      string
			artifacts func() iter.Seq[*datura.Artifact]
		}

		cases := []ignoredCase{
			{
				name: "ticker update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return ticker.NewFixture(ticker.UPDATE, 1).Artifacts()
				},
			},
			{
				name: "ticker snapshot artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return ticker.NewFixture(ticker.SNAPSHOT, 1).Artifacts()
				},
			},
			{
				name: "book update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return book.NewFixture(book.UPDATE, 1).Artifacts()
				},
			},
			{
				name: "book snapshot artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return book.NewFixture(book.SNAPSHOT, 1).Artifacts()
				},
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				count := 0

				for artifact := range testCase.artifacts() {
					for range signal.Measure(artifact, &market.CrossSection{}) {
						count++
					}
				}

				Convey("Then no Hawkes measurements should be emitted", func() {
					So(count, ShouldEqual, 0)
				})
			})
		}
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a Hawkes signal", t, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		type measureCase struct {
			name       string
			artifacts  func() iter.Seq[*datura.Artifact]
			wantCount  int
			wantOutput bool
		}

		cases := []measureCase{
			{
				name: "trade update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.UPDATE, 128).Artifacts()
				},
				wantCount:  114,
				wantOutput: true,
			},
			{
				name: "trade snapshot artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.SNAPSHOT, 1).Artifacts()
				},
				wantCount: 0,
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				count := 0
				classified := 0

				for artifact := range testCase.artifacts() {
					for result := range signal.Measure(artifact, &market.CrossSection{}) {
						count++

						So(datura.Peek[string](result, "role"), ShouldEqual, "measurement")
						So(datura.Peek[string](result, "scope"), ShouldEqual, "MATIC/USD")
						So(datura.Peek[string](result, "symbol"), ShouldEqual, "MATIC/USD")
						So(datura.Peek[string](result, "root"), ShouldEqual, "output")
						So(datura.Peek[float64](result, "output", "value"), ShouldBeGreaterThan, 0)
						So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
						So(datura.Peek[float64](result, "output", "entry_baseline"), ShouldBeGreaterThan, 0)
						So(datura.Peek[float64](result, "output", "exit_baseline"), ShouldBeGreaterThan, 0)
						So(datura.Peek[[]float64](result, "output", "probabilities"), ShouldHaveLength, 4)
						classified++
						So(hawkesCategory(datura.Peek[float64](result, "output", "category")), ShouldBeTrue)

						if datura.Peek[bool](result, "output", "ready") {
							So(datura.Peek[float64](result, "output", "strength"), ShouldBeGreaterThan, 0)
							So(datura.Peek[float64](result, "output", "branchingRatio"), ShouldBeGreaterThanOrEqualTo, 0)
						}
					}
				}

				Convey(fmt.Sprintf("Then %s should yield expected measurements", testCase.name), func() {
					So(count, ShouldEqual, testCase.wantCount)

					if testCase.wantOutput {
						So(classified, ShouldBeGreaterThan, 0)
					}
				})
			})
		}
	})
}

func TestTradeMeasure(t *testing.T) {
	Convey("Given a Hawkes trade role", t, func() {
		role := NewTrade()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var result *datura.Artifact

		for index := range 160 {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			measured := role.Measure(hawkesRow(datura.Map[any]{
				"symbol":    "BTC/USD",
				"side":      side,
				"price":     100.0,
				"qty":       1.0,
				"timestamp": base.Add(time.Duration(index) * 100 * time.Millisecond).Format(time.RFC3339Nano),
			}), &market.CrossSection{})

			if datura.Peek[string](measured, "root") == "output" {
				if result != nil {
					result.Release()
				}

				result = measured
			}
		}

		Convey("When enough trade arrivals have warmed the excitation pipeline", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "root"), ShouldEqual, "output")
			So(hawkesCategory(datura.Peek[float64](result, "output", "category")), ShouldBeTrue)
			So(datura.Peek[float64](result, "output", "frenzy"), ShouldBeGreaterThanOrEqualTo, 0)
			So(datura.Peek[float64](result, "output", "saturation"), ShouldBeGreaterThanOrEqualTo, 0)
			So(datura.Peek[float64](result, "output", "organic"), ShouldBeGreaterThanOrEqualTo, 0)
			So(datura.Peek[float64](result, "output", "exhaustion"), ShouldBeGreaterThanOrEqualTo, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func hawkesRow(row datura.Map[any]) *datura.Artifact {
	symbol, _ := row["symbol"].(string)

	return datura.Acquire(
		"hawkes", datura.APPJSON,
	).WithRole(
		"measurement",
	).WithScope(
		symbol,
	).WithPayload(
		row.Marshal(),
	)
}

func hawkesCategory(category float64) bool {
	switch int(category) {
	case logic.CategoryIndex(logic.CategoryFrenzy),
		logic.CategoryIndex(logic.CategorySaturation),
		logic.CategoryIndex(logic.CategoryOrganic),
		logic.CategoryIndex(logic.CategoryExhaustion):
		return true
	default:
		return false
	}
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	signal := NewSignal(context.Background(), dmt.NewTree(""))
	defer func() {
		_ = signal.Close()
	}()

	artifacts := make([]*datura.Artifact, 0, 128)
	for artifact := range trade.NewFixture(trade.UPDATE, 128).Artifacts() {
		artifacts = append(artifacts, artifact)
	}
	defer func() {
		for _, artifact := range artifacts {
			artifact.Release()
		}
	}()

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		for _, artifact := range artifacts {
			for range signal.Measure(artifact, &market.CrossSection{}) {
			}
		}
	}
}
