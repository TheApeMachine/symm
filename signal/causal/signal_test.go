package causal

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
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

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func artifactPayload(artifact *datura.Artifact) ([]byte, bool) {
	if artifact == nil || !artifact.HasEncryptedPayload() {
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

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given warmed trade and ticker frames", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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

				measured := signal.Measure(datapoint)

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

			measured := signal.Measure(datapoint)

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)
				result = measured
			}

			datapoint.Release()
		}

		Convey("It should classify systemic beta on warmed mixed frames", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(testutil.DominantCategoryIndex(result, []logic.CategoryType{
				logic.CategoryEndogenousAlpha,
				logic.CategorySystemicBeta,
				logic.CategoryLiquidityShock,
				logic.CategoryCausalNoise,
			}), ShouldEqual, logic.CategoryIndex(logic.CategorySystemicBeta))
			So(datura.Peek[float64](result, "output", "beta"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "beta"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "alpha"))
		})
	})
}

func TestHydrateNodeStoreFromTreeResetsFresh(testingTB *testing.T) {
	Convey("Given trades indexed in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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
