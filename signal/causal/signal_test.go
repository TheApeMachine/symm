package causal

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

func tickerFrame(symbol string, last, bid, ask, bidQty, askQty, changePct float64) string {
	return fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":"%s","bid":%g,"bid_qty":%g,"ask":%g,"ask_qty":%g,"last":%g,"volume":1000,"change_pct":%g}]}`,
		symbol,
		bid,
		bidQty,
		ask,
		askQty,
		last,
		changePct,
	)
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

func tradeFrame(symbol, side string, price, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"%s","side":%q,"price":%g,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol,
		side,
		price,
		quantity,
	)
}

func classified(result *datura.Artifact) bool {
	return datura.Peek[string](result, "root") == "output" &&
		datura.Peek[float64](result, "output", "confidence") > 0
}

func causalCategory(result *datura.Artifact) int {
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
	Convey("Given a causal signal", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		Convey("It declares ticker, book, and trade ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"ticker", "book", "trade"})
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given ticker, book, and trade fixtures", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		count := 0

		measure := func(artifacts iter.Seq[*datura.Artifact]) {
			for artifact := range artifacts {
				for measurement := range signal.Measure(artifact, &market.CrossSection{}) {
					count++

					role := datura.Peek[string](measurement, "role")
					scope, scopeErr := measurement.Scope()
					origin, originErr := measurement.Origin()

					So(role, ShouldEqual, "measurement")
					So(scopeErr, ShouldBeNil)
					So(scope, ShouldNotEqual, "")
					So(originErr, ShouldBeNil)
					So(origin, ShouldEqual, string(logic.SourceCausal))
					So(datura.Peek[float64](measurement, "output", "value"), ShouldBeGreaterThan, 0)
					So(datura.Peek[float64](measurement, "output", "confidence"), ShouldBeGreaterThan, 0)
					So(datura.Peek[float64](measurement, "output", "entry_baseline"), ShouldBeGreaterThan, 0)
					So(datura.Peek[float64](measurement, "output", "exit_baseline"), ShouldBeGreaterThan, 0)
					So(datura.Peek[[]float64](measurement, "output", "probabilities"), ShouldHaveLength, 4)

					if classified(measurement) {
						So(logic.Categories[causalCategory(measurement)], ShouldNotEqual, logic.CategoryTypeNone)
					}
				}

				artifact.Release()
			}
		}

		measure(tickerfixtures.NewFixture(tickerfixtures.UPDATE, 8).Artifacts())
		measure(bookfixtures.NewFixture(bookfixtures.UPDATE, 8).Artifacts())
		measure(tradefixtures.NewFixture(tradefixtures.UPDATE, 8).Artifacts())

		Convey("It routes fixture rows into causal measurement artifacts", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given local flow driving price", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{}

		price := 100.0

		for index := range 12 {
			flow := 1 + float64(index)
			price *= 1 + flow*0.001
			frames = append(frames,
				struct {
					role    string
					payload string
				}{"trade", tradeFrame("BTC/USD", "buy", price, flow)},
			)
		}

		frames = append(frames,
			struct {
				role    string
				payload string
			}{"trade", tradeFrame("BTC/USD", "buy", price, 1)},
		)

		result := replay(signal, frames)

		Convey("It emits an endogenous Pearl category candidate", func() {
			So(result, ShouldNotBeNil)
			So(causalCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryEndogenousAlpha))
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "uplift"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a sudden liquidity void", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{}

		for index := range 8 {
			price := 100 + float64(index)*0.01
			spread := 0.01 + float64(index)*0.001
			depth := 100 - float64(index)
			frames = append(frames,
				struct {
					role    string
					payload string
				}{"ticker", tickerFrame("BTC/USD", price, price-spread, price+spread, depth, depth, 0.01)},
				struct {
					role    string
					payload string
				}{"book", bookFrame("BTC/USD", price-spread, price+spread, depth, depth)},
				struct {
					role    string
					payload string
				}{"trade", tradeFrame("BTC/USD", "buy", price, 1+float64(index)*0.1)},
			)
		}

		frames = append(frames,
			struct {
				role    string
				payload string
			}{"book", bookFrame("BTC/USD", 90, 110, 0.01, 0.01)},
		)

		result := replay(signal, frames)

		Convey("It emits liquidity shock when Pearl inverts the regime", func() {
			So(result, ShouldNotBeNil)
			So(causalCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLiquidityShock))
			So(datura.Peek[float64](result, "output", "shockScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "contagion"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given associated flow already at its peak", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frames := []struct {
			role    string
			payload string
		}{}

		price := 100.0

		for index := range 12 {
			flow := 1 + float64(index)
			price *= 1 + flow*0.001
			frames = append(frames,
				struct {
					role    string
					payload string
				}{"trade", tradeFrame("BTC/USD", "buy", price, flow)},
			)
		}

		result := replay(signal, frames)

		Convey("It emits systemic beta when association dominates intervention", func() {
			So(result, ShouldNotBeNil)
			So(causalCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategorySystemicBeta))
			So(datura.Peek[float64](result, "output", "betaScore"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given unstructured local flow", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		flows := []float64{1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10}
		returns := []float64{0.001, 0.001, -0.001, -0.001, 0.0005, 0.0005, -0.0005, -0.0005, 0.0008, 0.0008, -0.0008, -0.0008}
		price := 100.0
		frames := []struct {
			role    string
			payload string
		}{}

		for index, flow := range flows {
			price *= 1 + returns[index]
			frames = append(frames, struct {
				role    string
				payload string
			}{"trade", tradeFrame("BTC/USD", "buy", price, flow)})
		}

		result := replay(signal, frames)

		Convey("It emits causal noise when no driver dominates", func() {
			So(result, ShouldNotBeNil)
			So(causalCategory(result), ShouldEqual, logic.CategoryIndex(logic.CategoryCausalNoise))
			So(datura.Peek[float64](result, "output", "noiseScore"), ShouldBeGreaterThan, 0)
		})
	})
}
