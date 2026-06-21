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
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
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

func feedTrade(
	signal *Signal,
	symbol, side string,
	price, qty float64,
	at time.Time,
) {
	raw, err := json.Marshal(map[string]any{
		"channel": "trade",
		"type":    "update",
		"data": []tradeUpdate{{
			Symbol:    symbol,
			Side:      side,
			Price:     price,
			Qty:       qty,
			Timestamp: at,
		}},
	})

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "trade", "update", raw)
}

func seedDefaultTrades(signal *Signal, symbol string, baseTime time.Time) {
	for index := range causalMinHistory {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		feedTrade(
			signal,
			symbol,
			side,
			100+float64(index),
			1,
			baseTime.Add(time.Duration(index)*time.Second),
		)
	}
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
				result = measured
			}

			datapoint.Release()
		}

		Convey("It should emit calibrated causal classification", func() {
			So(result, ShouldNotBeNil)
			So(
				datura.Peek[float64](result, "output", "intervention")+
					datura.Peek[float64](result, "output", "association")+
					datura.Peek[float64](result, "output", "uplift"),
				ShouldBeGreaterThan,
				0,
			)
		})
	})
}

func TestHydrateNodeStoreFromTreeResetsFresh(testingTB *testing.T) {
	Convey("Given trades indexed in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedDefaultTrades(signal, "BTC/USD", baseTime)

		signal.hydrateNodeStoreFromTree()
		nodes := signal.nodeStore.Nodes("BTC/USD")
		firstLength := nodes.AlignedLength()

		signal.hydrateNodeStoreFromTree()
		secondLength := signal.nodeStore.Nodes("BTC/USD").AlignedLength()

		Convey("It should rebuild without duplicating ladder history", func() {
			So(firstLength, ShouldBeGreaterThanOrEqualTo, causalMinHistory)
			So(secondLength, ShouldEqual, firstLength)
		})
	})
}
