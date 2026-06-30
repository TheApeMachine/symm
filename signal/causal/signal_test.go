package causal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

const causalMinHistory = 8

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

type tradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

func artifactPayload(artifact *datura.Artifact) ([]byte, bool) {
	if artifact == nil || !artifact.HasPayload() {
		return nil, false
	}

	payload := artifact.DecryptPayload()

	if len(payload) == 0 {
		return nil, false
	}

	return payload, true
}

func insertTreeArtifact(signal *Signal, role, scope string, payload []byte) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func newTestSignal(testingTB testing.TB) (*Signal, *market.CrossSection) {
	testingTB.Helper()

	crossSection, err := market.NewCrossSection(market.DefaultCrossSectionConfig())

	if err != nil {
		testingTB.Fatal(err)
	}

	return NewSignal(context.Background(), dmt.NewTree("")), crossSection
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given warmed trade and ticker frames", testingTB, func() {
		signal, crossSection := newTestSignal(testingTB)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var result *datura.Artifact

		for index := range 128 {
			at := base.Add(time.Duration(index) * time.Second).UnixNano()

			if index%3 == 0 {
				payload := fmt.Sprintf(
					`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":%g,"volume":1000,"change_pct":0.5}]}`,
					100+float64(index)*0.01,
				)
				datapoint := datura.Acquire("kraken:public", datura.APPJSON)
				datapoint.WithRole("ticker")
				datapoint.WithScope("update")
				datapoint.WithPayload([]byte(payload))
				datapoint.SetTimestamp(at)

				testutil.ObservePeers(crossSection, datapoint)
				measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))

				if measured != nil {
					signal.tree = testutil.StoreMeasurement(signal.tree, measured)
					result = measured
				}

				datapoint.Release()

				continue
			}

			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			raw, err := json.Marshal(map[string]any{
				"channel": "trade",
				"type":    "update",
				"data": []tradeUpdate{{
					Symbol:    "BTC/USD",
					Side:      side,
					Price:     100 + float64(index)*0.01,
					Qty:       1,
					Timestamp: base.Add(time.Duration(index) * time.Second),
				}},
			})

			if err != nil {
				panic(err)
			}

			datapoint := datura.Acquire("kraken:public", datura.APPJSON)
			datapoint.WithRole("trade")
			datapoint.WithScope("update")
			datapoint.WithPayload(raw)
			datapoint.SetTimestamp(at)

			measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)
				result = measured
			}

			datapoint.Release()
		}

		Convey("It should classify causal noise on warmed mixed frames", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(testutil.DominantCategoryIndex(result, []logic.CategoryType{
				logic.CategoryEndogenousAlpha,
				logic.CategorySystemicBeta,
				logic.CategoryLiquidityShock,
				logic.CategoryCausalNoise,
			}), ShouldEqual, logic.CategoryIndex(logic.CategoryCausalNoise))
			So(datura.Peek[float64](result, "output", "beta"), ShouldBeGreaterThan, 0)
			// Mixed unit-flow frames carry no identifiable flow→return effect, so
			// the abductive counterfactual leaves the move unexplained: noise must
			// dominate the causal fraction (alpha). Asserting alpha>0 here would be
			// asserting fabricated endogenous alpha.
			So(
				datura.Peek[float64](result, "output", "noise"),
				ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "alpha"),
			)
		})
	})
}

func TestCounterfactualSkipsDegenerateHistory(testingTB *testing.T) {
	Convey("Given constant flow and return history", testingTB, func() {
		signal, _ := newTestSignal(testingTB)

		defer func() {
			_ = signal.Close()
		}()

		uplift, noise, ok := signal.counterfactual(
			[]float64{1, 1, 1, 1},
			[]float64{0, 0, 0, 0},
		)

		Convey("It should decline the structural fit without fabricating uplift", func() {
			So(ok, ShouldBeFalse)
			So(uplift, ShouldEqual, 0)
			So(noise, ShouldEqual, 0)
		})
	})
}

func TestCounterfactualRequiresLaggedTreatmentHistory(testingTB *testing.T) {
	Convey("Given only enough rows for same-slice flow and return", testingTB, func() {
		signal, _ := newTestSignal(testingTB)

		defer func() {
			_ = signal.Close()
		}()

		uplift, noise, ok := signal.counterfactual(
			[]float64{1, 2, 4},
			[]float64{0.01, 0.02, 0.05},
		)

		Convey("It should decline because lag alignment leaves too few causal rows", func() {
			So(ok, ShouldBeFalse)
			So(uplift, ShouldEqual, 0)
			So(noise, ShouldEqual, 0)
		})
	})
}

func TestCounterfactualStandardizesIllConditionedHistory(testingTB *testing.T) {
	Convey("Given large flow units and tiny return units", testingTB, func() {
		signal, _ := newTestSignal(testingTB)

		defer func() {
			_ = signal.Close()
		}()

		uplift, noise, ok := signal.counterfactual(
			[]float64{10_000_000, 10_100_000, 10_200_000, 10_300_000, 10_400_000},
			[]float64{0.000001, 0.0000012, 0.0000014, 0.0000016, 0.0000018},
		)

		Convey("It should fit in normalized coordinates and return finite return-unit output", func() {
			So(ok, ShouldBeTrue)
			So(math.IsNaN(uplift), ShouldBeFalse)
			So(math.IsInf(uplift, 0), ShouldBeFalse)
			So(math.IsNaN(noise), ShouldBeFalse)
			So(math.IsInf(noise, 0), ShouldBeFalse)
		})
	})
}

func TestCounterfactualFaultsDoNotPanic(testingTB *testing.T) {
	Convey("Given non-finite causal history", testingTB, func() {
		signal, _ := newTestSignal(testingTB)
		defer func() {
			_ = signal.Close()
		}()

		uplift, noise, ok, fault := signal.counterfactualWithFault(
			[]float64{1, 2, math.Inf(1), 4},
			[]float64{0.01, 0.02, 0.03, 0.04},
		)

		Convey("It should fail closed without panic or fabricated output", func() {
			So(ok, ShouldBeFalse)
			So(uplift, ShouldEqual, 0)
			So(noise, ShouldEqual, 0)
			So(fault, ShouldEqual, "non_finite_history")
		})
	})
}

func TestSignalPublishesFaultMeasurement(testingTB *testing.T) {
	Convey("Given an anomalous causal fit failure", testingTB, func() {
		signal, crossSection := newTestSignal(testingTB)
		defer func() {
			_ = signal.Close()
		}()

		symbol := "BTC/USD"
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		datapoint := datura.Acquire("kraken:public", datura.APPJSON)
		datapoint.WithRole("trade")
		datapoint.WithScope("update")
		datapoint.WithPayload([]byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":104,"qty":1,"timestamp":"2026-05-30T12:00:04Z"}]}`))
		datapoint.SetTimestamp(base.Add(4 * time.Second).UnixNano())
		defer datapoint.Release()

		testutil.ObservePeers(crossSection, datapoint)
		measured := signal.emitFault(datapoint, symbol, "non_finite_history", 104)

		Convey("It should emit a fault artifact instead of crashing", func() {
			So(measured, ShouldNotBeNil)
			So(datura.Peek[string](measured, "output", "status"), ShouldEqual, "fault")
			So(datura.Peek[string](measured, "output", "fault"), ShouldEqual, "non_finite_history")
			So(datura.Peek[float64](measured, "output", "confidence"), ShouldEqual, 0)
			So(datura.Peek[bool](measured, "output", "counterfactualReady"), ShouldBeFalse)
		})
	})
}

func TestHistorianBookReadsLatestScopedIndex(testingTB *testing.T) {
	Convey("Given only the latest scoped book index", testingTB, func() {
		signal, _ := newTestSignal(testingTB)
		symbol := "BTC/USD"
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("book")
		artifact.WithScope(symbol)
		artifact.WithPayload([]byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":99,"qty":3}],"asks":[{"price":100,"qty":4}]}]}`))
		artifact.SetTimestamp(100)
		defer artifact.Release()

		signal.tree.InsertArtifact(latestScopedKey("book", symbol), artifact)

		spread, void, ok := signal.historian.book(symbol, 200)

		Convey("It should read spread and void from the latest tree key", func() {
			So(ok, ShouldBeTrue)
			So(spread, ShouldEqual, 1)
			So(void, ShouldEqual, 0)
		})
	})
}

func TestHydrateNodeStoreFromTreeResetsFresh(testingTB *testing.T) {
	Convey("Given trades indexed in the tree", testingTB, func() {
		signal, _ := newTestSignal(testingTB)
		nodeStore := NewNodeStore()

		defer func() {
			_ = signal.Close()
		}()

		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index := range causalMinHistory {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			raw, err := json.Marshal(map[string]any{
				"channel": "trade",
				"type":    "update",
				"data": []tradeUpdate{{
					Symbol:    "BTC/USD",
					Side:      side,
					Price:     100 + float64(index),
					Qty:       1,
					Timestamp: baseTime.Add(time.Duration(index) * time.Second),
				}},
			})

			if err != nil {
				panic(err)
			}

			insertTreeArtifact(signal, "trade", "update", raw)
		}

		for inbound := range signal.tree.Seek([]byte("trade/")) {
			payload, payloadOK := artifactPayload(inbound)

			if !payloadOK {
				continue
			}

			var frame struct {
				Data []tradeUpdate `json:"data"`
			}

			if json.Unmarshal(payload, &frame) != nil {
				continue
			}

			for _, update := range frame.Data {
				raw, err := json.Marshal(update)

				if err != nil {
					continue
				}

				nodeStore.Observe(update.Symbol, raw)
			}
		}

		firstLength := nodeStore.Nodes("BTC/USD").AlignedLength()

		for inbound := range signal.tree.Seek([]byte("trade/")) {
			payload, payloadOK := artifactPayload(inbound)

			if !payloadOK {
				continue
			}

			var frame struct {
				Data []tradeUpdate `json:"data"`
			}

			if json.Unmarshal(payload, &frame) != nil {
				continue
			}

			for _, update := range frame.Data {
				raw, err := json.Marshal(update)

				if err != nil {
					continue
				}

				nodeStore.Observe(update.Symbol, raw)
			}
		}

		secondLength := nodeStore.Nodes("BTC/USD").AlignedLength()

		Convey("It should rebuild without duplicating ladder history", func() {
			So(firstLength, ShouldBeGreaterThanOrEqualTo, causalMinHistory)
			So(secondLength, ShouldEqual, firstLength)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal, crossSection := newTestSignal(b)

		for index := range 32 {
			at := base.Add(time.Duration(index) * time.Second).UnixNano()
			payload := fmt.Sprintf(
				`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":%g,"qty":1.5,"timestamp":"2026-05-30T12:00:00Z"}]}`,
				100+float64(index)*0.01,
			)
			datapoint := datura.Acquire("kraken:public", datura.APPJSON)
			datapoint.WithRole("trade")
			datapoint.WithScope("update")
			datapoint.WithPayload([]byte(payload))
			datapoint.SetTimestamp(at)

			measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			datapoint.Release()
		}

		_ = signal.Close()
	}
}
