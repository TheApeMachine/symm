package pumpdump

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

func TestSignalMeasure(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		tree := dmt.NewTree("")
		signal := NewSignal(t.Context(), tree)

		type strengthMode int

		const (
			ignoreStrength strengthMode = iota
			requirePositiveStrength
			requireNonnegativeStrength
		)

		type measureCase struct {
			name             string
			artifacts        func() iter.Seq[*datura.Artifact]
			repeat           int
			wantCount        int
			wantScope        string
			wantSymbol       string
			wantUUID         bool
			wantSourceInputs []string
			wantScoreInputs  []string
			wantCategoryKeys []int
			wantTickerFields bool
			wantStrength     strengthMode
		}

		cases := []measureCase{
			{
				name: "ticker update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return ticker.NewFixture(ticker.UPDATE, 3).Artifacts()
				},
				repeat:    40,
				wantCount: 120,
				wantScope: "ALGO/USD",
				wantUUID:  true,
				wantSourceInputs: []string{
					"volume",
					"last",
					"bid",
					"ask",
				},
				wantScoreInputs: []string{
					"ignition",
					"compression",
					"trend",
					"exhaustion",
				},
				wantCategoryKeys: []int{
					logic.CategoryIndex(logic.CategoryVerticalIgnition),
					logic.CategoryIndex(logic.CategoryCoiledCompression),
					logic.CategoryIndex(logic.CategoryOrganicTrend),
					logic.CategoryIndex(logic.CategoryFadedExhaustion),
				},
				wantTickerFields: true,
			},
			{
				name: "book update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return book.NewFixture(book.UPDATE, 3).Artifacts()
				},
				wantCount:  0,
				wantScope:  "MATIC/USD",
				wantSymbol: "MATIC/USD",
				wantScoreInputs: []string{
					"loadedScore",
					"spoofScore",
					"thinScore",
					"neutralScore",
				},
				wantCategoryKeys: []int{
					logic.CategoryIndex(logic.CategoryLoadedImbalance),
					logic.CategoryIndex(logic.CategorySpoofTrap),
					logic.CategoryIndex(logic.CategoryBookThinning),
					logic.CategoryIndex(logic.CategoryDenseNeutrality),
				},
			},
			{
				name: "repeated book snapshot artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return book.NewFixture(book.SNAPSHOT, 1).Artifacts()
				},
				repeat:       6,
				wantCount:    6,
				wantScope:    "MATIC/USD",
				wantSymbol:   "MATIC/USD",
				wantStrength: requirePositiveStrength,
				wantScoreInputs: []string{
					"loadedScore",
					"spoofScore",
					"thinScore",
					"neutralScore",
				},
				wantCategoryKeys: []int{
					logic.CategoryIndex(logic.CategoryLoadedImbalance),
					logic.CategoryIndex(logic.CategorySpoofTrap),
					logic.CategoryIndex(logic.CategoryBookThinning),
					logic.CategoryIndex(logic.CategoryDenseNeutrality),
				},
			},
			{
				name: "trade update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.UPDATE, 3).Artifacts()
				},
				wantCount:  1,
				wantScope:  "MATIC/USD",
				wantSymbol: "MATIC/USD",
				wantScoreInputs: []string{
					"absorption",
					"drive",
					"balance",
					"starvation",
				},
				wantCategoryKeys: []int{
					logic.CategoryIndex(logic.CategoryHiddenAbsorption),
					logic.CategoryIndex(logic.CategoryAggressiveDrive),
					logic.CategoryIndex(logic.CategoryStochasticBalance),
					logic.CategoryIndex(logic.CategoryVolumeStarvation),
				},
			},
			{
				name: "trade update sequence",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.UPDATE, 30).Artifacts()
				},
				wantCount:    28,
				wantScope:    "MATIC/USD",
				wantSymbol:   "MATIC/USD",
				wantStrength: requireNonnegativeStrength,
				wantScoreInputs: []string{
					"absorption",
					"drive",
					"balance",
					"starvation",
				},
				wantCategoryKeys: []int{
					logic.CategoryIndex(logic.CategoryHiddenAbsorption),
					logic.CategoryIndex(logic.CategoryAggressiveDrive),
					logic.CategoryIndex(logic.CategoryStochasticBalance),
					logic.CategoryIndex(logic.CategoryVolumeStarvation),
				},
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				repeat := testCase.repeat

				if repeat == 0 {
					repeat = 1
				}

				count := 0
				for range repeat {
					for artifact := range testCase.artifacts() {
						for result := range signal.Measure(artifact, &market.CrossSection{}) {
							count++

							if testCase.wantUUID {
								_, err := result.Uuid()
								So(err, ShouldBeNil)
							}

							So(datura.Peek[string](result, "role"), ShouldEqual, "measurement")
							So(datura.Peek[string](result, "scope"), ShouldEqual, testCase.wantScope)
							origin, originErr := result.Origin()
							So(originErr, ShouldBeNil)
							So(origin, ShouldEqual, string(logic.SourcePumpDump))
							So(result.Timestamp(), ShouldBeGreaterThan, 0)

							if testCase.wantSymbol != "" {
								So(datura.Peek[string](result, "symbol"), ShouldEqual, testCase.wantSymbol)
							}

							So(datura.Peek[string](result, "root"), ShouldEqual, "output")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "probabilities")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "category")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "confidence")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "confidence_baseline")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "distribution")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "entry_baseline")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "exit_baseline")
							So(datura.Peek[[]string](result, "inputs"), ShouldContain, "strength")

							if len(testCase.wantSourceInputs) > 0 {
								So(datura.Peek[[]string](result, "sourceInputs"), ShouldResemble, testCase.wantSourceInputs)
							}

							for _, input := range testCase.wantScoreInputs {
								So(datura.Peek[[]string](result, "inputs"), ShouldContain, input)
							}

							if testCase.wantTickerFields {
								So(datura.Peek[float64](result, "output", "rvol"), ShouldBeGreaterThanOrEqualTo, 0)
								So(datura.Peek[float64](result, "output", "precursor"), ShouldBeGreaterThanOrEqualTo, 0)
								So(datura.Peek[float64](result, "output", "spread"), ShouldBeGreaterThan, 0)
								So(datura.Peek[float64](result, "output", "compression"), ShouldBeGreaterThanOrEqualTo, 0)
							}

							So(testCase.wantCategoryKeys, ShouldContain, int(datura.Peek[float64](result, "output", "category")))
							So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThanOrEqualTo, 0.25)
							So(datura.Peek[float64](result, "output", "confidence"), ShouldBeLessThanOrEqualTo, 1)
							So(datura.Peek[float64](result, "output", "confidence_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
							So(datura.Peek[float64](result, "output", "entry_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
							So(datura.Peek[float64](result, "output", "exit_baseline"), ShouldAlmostEqual, 0.25, 1e-12)

							probabilities := datura.Peek[[]float64](result, "output", "probabilities")
							distribution := datura.Peek[map[string]any](result, "output", "distribution")
							So(len(probabilities), ShouldEqual, 4)
							So(len(distribution), ShouldEqual, 4)

							probabilitySum := 0.0
							for index, probability := range probabilities {
								So(probability, ShouldBeGreaterThanOrEqualTo, 0)
								So(probability, ShouldBeLessThanOrEqualTo, 1)
								So(distribution[fmt.Sprintf("%d", testCase.wantCategoryKeys[index])], ShouldAlmostEqual, probability, 1e-12)
								probabilitySum += probability
							}

							So(probabilitySum, ShouldAlmostEqual, 1, 1e-12)

							switch testCase.wantStrength {
							case requirePositiveStrength:
								So(datura.Peek[float64](result, "output", "strength"), ShouldBeGreaterThan, 0)
							case requireNonnegativeStrength:
								So(datura.Peek[float64](result, "output", "strength"), ShouldBeGreaterThanOrEqualTo, 0)
							}
						}
					}
				}

				Convey(fmt.Sprintf("Then %s should yield expected measurements", testCase.name), func() {
					So(count, ShouldEqual, testCase.wantCount)
				})
			})
		}
	})
}

func TestSignalMeasureTradeCategories(t *testing.T) {
	Convey("Given controlled pumpdump trade rows", t, func() {
		type tradeTick struct {
			side  string
			price float64
			qty   float64
		}

		type tradeCase struct {
			name                 string
			ticks                []tradeTick
			wantCategory         int
			wantProbabilityIndex int
			wantScore            string
		}

		assertHighestProbability := func(result *datura.Artifact, category int, probabilityIndex int) {
			probabilities := datura.Peek[[]float64](result, "output", "probabilities")
			distribution := datura.Peek[map[string]any](result, "output", "distribution")
			So(len(probabilities), ShouldEqual, 4)
			So(len(distribution), ShouldEqual, 4)

			categoryIndexes := []int{
				logic.CategoryIndex(logic.CategoryHiddenAbsorption),
				logic.CategoryIndex(logic.CategoryAggressiveDrive),
				logic.CategoryIndex(logic.CategoryStochasticBalance),
				logic.CategoryIndex(logic.CategoryVolumeStarvation),
			}
			selected := probabilities[probabilityIndex]
			for index, probability := range probabilities {
				So(distribution[fmt.Sprintf("%d", categoryIndexes[index])], ShouldAlmostEqual, probability, 1e-12)
				if index != probabilityIndex {
					So(selected, ShouldBeGreaterThan, probability)
				}
			}

			So(int(datura.Peek[float64](result, "output", "category")), ShouldEqual, category)
		}

		cases := []tradeCase{
			{
				name: "absorption",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
				},
				wantCategory:         logic.CategoryIndex(logic.CategoryHiddenAbsorption),
				wantProbabilityIndex: 0,
				wantScore:            "absorption",
			},
			{
				name: "drive",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 101, qty: 1},
					{side: "buy", price: 102, qty: 1},
					{side: "buy", price: 103, qty: 1},
				},
				wantCategory:         logic.CategoryIndex(logic.CategoryAggressiveDrive),
				wantProbabilityIndex: 1,
				wantScore:            "drive",
			},
			{
				name: "balance",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "sell", price: 100.1, qty: 1},
					{side: "buy", price: 100.2, qty: 1},
					{side: "sell", price: 100.3, qty: 1},
				},
				wantCategory:         logic.CategoryIndex(logic.CategoryStochasticBalance),
				wantProbabilityIndex: 2,
				wantScore:            "balance",
			},
			{
				name: "starvation",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "sell", price: 100.1, qty: 1},
				},
				wantCategory:         logic.CategoryIndex(logic.CategoryVolumeStarvation),
				wantProbabilityIndex: 3,
				wantScore:            "starvation",
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s trade flow", testCase.name), func() {
				signal := NewSignal(t.Context(), dmt.NewTree(""))
				var result *datura.Artifact

				for index, tick := range testCase.ticks {
					frame := datura.Acquire(
						"pumpdump-controlled-trade", datura.APPJSON,
					).WithRole(
						"trade",
					).WithPayload(
						datura.Map[any]{
							"data": []datura.Map[any]{
								{
									"symbol": "CONTROL/USD",
									"side":   tick.side,
									"price":  tick.price,
									"qty":    tick.qty,
								},
							},
						}.Marshal(),
					)
					frame.SetTimestamp(int64(index + 1))

					for measurement := range signal.Measure(frame, &market.CrossSection{}) {
						result = measurement
					}
				}

				Convey("Then the intended category should carry the highest probability", func() {
					So(result, ShouldNotBeNil)
					So(datura.Peek[string](result, "root"), ShouldEqual, "output")
					So(int(datura.Peek[float64](result, "output", "category")), ShouldEqual, testCase.wantCategory)
					So(datura.Peek[float64](result, "output", testCase.wantScore), ShouldBeGreaterThan, 0)
					So(datura.Peek[float64](result, "output", "entry_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
					So(datura.Peek[float64](result, "output", "exit_baseline"), ShouldAlmostEqual, 0.25, 1e-12)
					assertHighestProbability(result, testCase.wantCategory, testCase.wantProbabilityIndex)
				})
			})
		}
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	signal := NewSignal(context.Background(), dmt.NewTree(""))
	defer func() {
		_ = signal.Close()
	}()

	artifacts := make([]*datura.Artifact, 0, 3)
	for artifact := range ticker.NewFixture(ticker.UPDATE, 3).Artifacts() {
		artifacts = append(artifacts, artifact)
	}
	defer func() {
		for _, artifact := range artifacts {
			artifact.Release()
		}
	}()

	benchmark.ReportAllocs()
	stamp := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	for benchmark.Loop() {
		for _, artifact := range artifacts {
			stamp++
			artifact.SetTimestamp(stamp)

			for range signal.Measure(artifact, &market.CrossSection{}) {
			}
		}
	}
}
