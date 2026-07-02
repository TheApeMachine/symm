package pumpdump

import (
	"fmt"
	"iter"
	"strings"
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

func TestSignalMeasure(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		tree := dmt.NewTree("")
		signal := NewSignal(t.Context(), tree)

		type outputMode int
		type strengthMode int

		const (
			ignoreOutput outputMode = iota
			requireEveryOutput
			requireAnyOutput
		)

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
			output           outputMode
			wantSourceInputs []string
			wantTickerFields bool
			wantStrength     strengthMode
		}

		assertStructuredPayload := func(result *datura.Artifact) {
			payload := strings.TrimSpace(string(result.DecryptPayload()))
			So(payload, ShouldNotEqual, "")
			So(payload[:1], ShouldEqual, "{")
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
				output:    requireEveryOutput,
				wantSourceInputs: []string{
					"volume",
					"last",
					"bid",
					"ask",
				},
				wantTickerFields: true,
			},
			{
				name: "book update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return book.NewFixture(book.UPDATE, 3).Artifacts()
				},
				wantCount:  3,
				wantScope:  "MATIC/USD",
				wantSymbol: "MATIC/USD",
				output:     ignoreOutput,
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
				output:       requireAnyOutput,
				wantStrength: requirePositiveStrength,
			},
			{
				name: "trade update artifacts",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.UPDATE, 3).Artifacts()
				},
				wantCount:  3,
				wantScope:  "MATIC/USD",
				wantSymbol: "MATIC/USD",
				output:     ignoreOutput,
			},
			{
				name: "trade update sequence",
				artifacts: func() iter.Seq[*datura.Artifact] {
					return trade.NewFixture(trade.UPDATE, 30).Artifacts()
				},
				wantCount:    30,
				wantScope:    "MATIC/USD",
				wantSymbol:   "MATIC/USD",
				output:       requireAnyOutput,
				wantStrength: requireNonnegativeStrength,
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
				classified := 0

				for range repeat {
					for artifact := range testCase.artifacts() {
						for result := range signal.Measure(artifact, &market.CrossSection{}) {
							count++
							assertStructuredPayload(result)

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

							root := datura.Peek[string](result, "root")

							if testCase.output == ignoreOutput || root != "output" {
								continue
							}

							classified++

							if testCase.output == requireEveryOutput {
								So(root, ShouldEqual, "output")
							}

							if len(testCase.wantSourceInputs) > 0 {
								So(datura.Peek[[]string](result, "sourceInputs"), ShouldResemble, testCase.wantSourceInputs)
							}

							if testCase.wantTickerFields {
								So(datura.Peek[float64](result, "output", "rvol"), ShouldBeGreaterThanOrEqualTo, 0)
								So(datura.Peek[float64](result, "output", "precursor"), ShouldBeGreaterThanOrEqualTo, 0)
								So(datura.Peek[float64](result, "output", "spread"), ShouldBeGreaterThan, 0)
								So(datura.Peek[float64](result, "output", "compression"), ShouldBeGreaterThanOrEqualTo, 0)
							}

							So(int(datura.Peek[float64](result, "output", "category")), ShouldBeBetweenOrEqual, 1, 4)
							So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

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

					switch testCase.output {
					case requireEveryOutput:
						So(classified, ShouldEqual, testCase.wantCount)
					case requireAnyOutput:
						So(classified, ShouldBeGreaterThan, 0)
					}
				})
			})
		}
	})
}
