package cvd

import (
	"context"
	"fmt"
	"iter"
	"testing"

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
	Convey("Given a CVD signal", t, func() {
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

				Convey("Then no CVD measurements should be emitted", func() {
					So(count, ShouldEqual, 0)
				})
			})
		}
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a CVD signal", t, func() {
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
					return trade.NewFixture(trade.UPDATE, 30).Artifacts()
				},
				wantCount:  28,
				wantOutput: true,
			},
			{
				name: "trade snapshot artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.SNAPSHOT, 1).Artifacts()
				},
				wantCount: 1,
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
						origin, originErr := result.Origin()
						So(originErr, ShouldBeNil)
						So(origin, ShouldEqual, string(logic.SourceCVD))
						So(result.Timestamp(), ShouldBeGreaterThan, 0)

						if datura.Peek[string](result, "root") != "output" {
							continue
						}

						classified++
						So(datura.Peek[float64](result, "output", "netFraction"), ShouldBeGreaterThanOrEqualTo, 0)
						So(datura.Peek[float64](result, "output", "strength"), ShouldBeGreaterThanOrEqualTo, 0)
						So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
						So(cvdCategory(datura.Peek[float64](result, "output", "category")), ShouldBeTrue)
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
	Convey("Given a CVD trade role", t, func() {
		type tradeCase struct {
			name         string
			rows         []datura.Map[any]
			wantCategory logic.CategoryType
		}

		cases := []tradeCase{
			{
				name: "hidden absorption",
				rows: []datura.Map[any]{
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 10.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 10.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 10.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 10.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 10.0},
				},
				wantCategory: logic.CategoryHiddenAbsorption,
			},
			{
				name: "aggressive drive",
				rows: []datura.Map[any]{
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 101.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 102.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 103.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 104.0, "qty": 2.0},
				},
				wantCategory: logic.CategoryAggressiveDrive,
			},
			{
				name: "stochastic balance",
				rows: []datura.Map[any]{
					{"symbol": "BTC/USD", "side": "buy", "price": 100.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "sell", "price": 100.0, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "buy", "price": 100.1, "qty": 2.0},
					{"symbol": "BTC/USD", "side": "sell", "price": 100.1, "qty": 2.0},
				},
				wantCategory: logic.CategoryStochasticBalance,
			},
			{
				name: "volume starvation",
				rows: []datura.Map[any]{
					{"symbol": "BTC/USD", "side": "buy", "price": 100.1, "qty": 0.001},
					{"symbol": "BTC/USD", "side": "sell", "price": 100.1, "qty": 0.001},
				},
				wantCategory: logic.CategoryVolumeStarvation,
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				role := NewTrade()
				var result *datura.Artifact

				for _, row := range testCase.rows {
					if result != nil {
						result.Release()
					}

					result = role.Measure(cvdRow(row), &market.CrossSection{})
				}

				Convey(fmt.Sprintf("Then CVD should classify %s", testCase.name), func() {
					So(result, ShouldNotBeNil)
					So(datura.Peek[string](result, "root"), ShouldEqual, "output")
					So(int(datura.Peek[float64](result, "output", "category")), ShouldEqual, logic.CategoryIndex(testCase.wantCategory))
					So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

					result.Release()
				})
			})
		}
	})
}

func cvdRow(row datura.Map[any]) *datura.Artifact {
	symbol, _ := row["symbol"].(string)

	return datura.Acquire(
		"cvd", datura.APPJSON,
	).WithRole(
		"measurement",
	).WithScope(
		symbol,
	).WithPayload(
		row.Marshal(),
	)
}

func cvdCategory(category float64) bool {
	switch int(category) {
	case logic.CategoryIndex(logic.CategoryHiddenAbsorption),
		logic.CategoryIndex(logic.CategoryAggressiveDrive),
		logic.CategoryIndex(logic.CategoryStochasticBalance),
		logic.CategoryIndex(logic.CategoryVolumeStarvation):
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

	artifacts := make([]*datura.Artifact, 0, 8)
	for artifact := range trade.NewFixture(trade.UPDATE, 8).Artifacts() {
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
