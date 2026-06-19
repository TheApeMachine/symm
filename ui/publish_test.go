package ui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func walletArtifactFromAssets(rows []map[string]any) *datura.Artifact {
	payload, _ := json.Marshal(map[string]any{"asset": rows})

	return datura.Acquire("test", datura.Artifact_Type_json).
		WithRole("balances").
		WithPayload(payload)
}

func ohlcArtifactFromWire(wire map[string]any) *datura.Artifact {
	payload, _ := json.Marshal(wire)

	return datura.Acquire("test", datura.Artifact_Type_json).
		WithRole("ohlc").
		WithPayload(payload)
}

func TestWalletFrameFromAssetRows(t *testing.T) {
	Convey("Given paper balances with asset rows only", t, func() {
		viper.Set("market.quote_currency", "USD")

		frame := WalletFrame(walletArtifactFromAssets([]map[string]any{{
			"asset":   "USD",
			"balance": 200,
		}}))

		Convey("It should expose dashboard wallet fields", func() {
			So(frame["type"], ShouldEqual, "wallet")
			So(frame["Currency"], ShouldEqual, "USD")
			So(frame["Balance"], ShouldEqual, 200)
		})
	})
}

func TestPublishWallet(t *testing.T) {
	Convey("Given a wallet snapshot", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "wallet" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishWallet is called", func() {
			err := PublishWallet(pool, walletArtifactFromAssets([]map[string]any{{
				"asset":   "USD",
				"balance": 200,
			}}))

			Convey("It should emit one wallet frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui wallet frame", ShouldEqual, "received")
				}

				So(frame["Balance"], ShouldEqual, 200)
			})
		})
	})
}

func TestPublishWalletSkipsEmptySnapshot(t *testing.T) {
	Convey("Given an empty wallet snapshot", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "wallet" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishWallet is called", func() {
			err := PublishWallet(pool, walletArtifactFromAssets(nil))

			Convey("It should not emit a wallet frame", func() {
				So(err, ShouldBeNil)

				select {
				case frame := <-received:
					So(frame, ShouldBeNil)
				case <-time.After(100 * time.Millisecond):
				}
			})
		})
	})
}

func TestPublishOhlc(t *testing.T) {
	Convey("Given one candle update", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "ohlc" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishOhlc is called", func() {
			err := PublishOhlc(pool, ohlcArtifactFromWire(map[string]any{
				"symbol":         "BTC/USD",
				"open":           1.0,
				"high":           2.0,
				"low":            0.5,
				"close":          1.5,
				"volume":         10.0,
				"interval_begin": "2026-06-16T12:00:00.000000Z",
			}))

			Convey("It should emit numeric sec for the chart", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui ohlc frame", ShouldEqual, "received")
				}

				So(frame["symbol"], ShouldEqual, "BTC/USD")
				So(frame["sec"], ShouldEqual, time.Date(
					2026, 6, 16, 12, 0, 0, 0, time.UTC,
				).Unix())
			})
		})
	})
}

func TestGaugeReadingsFromMeasurements(t *testing.T) {
	Convey("Given publishable measurements", t, func() {
		now := time.Now()
		readings := gaugeReadingsFromMeasurements([]logic.Measurement{
			{
				Source:     logic.SourceFluid,
				Symbol:     "BTC/USD",
				Price:      1,
				Strength:   0.2,
				Volume:     1,
				Spread:     0.1,
				Elapsed:    1,
				Confidence: 0.7,
				Surprise:   1.2,
				ObservedAt: now,
			},
		})

		Convey("It should mark calibrated gauge evidence", func() {
			So(len(readings), ShouldEqual, 1)
			So(readings[0]["source"], ShouldEqual, "fluid")
			So(readings[0]["calibrated"], ShouldBeTrue)
		})
	})
}

func TestStateFrame(t *testing.T) {
	Convey("Given publishable measurements", t, func() {
		frame := StateFrame([]logic.Measurement{
			{
				Source:     logic.SourceFluid,
				Symbol:     "BTC/USD",
				Price:      1,
				Strength:   0.2,
				Volume:     1,
				Spread:     0.1,
				Elapsed:    1,
				Confidence: 0.7,
				Surprise:   1.2,
			},
		}, 4, 1, logic.WalkTrace{})

		Convey("It should include gauge readings on the heartbeat frame", func() {
			So(frame["type"], ShouldEqual, "state")
			So(frame["story_ticks"], ShouldEqual, 4)

			gaugeReadings, ok := frame["gauge_readings"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 1)
			So(gaugeReadings[0]["source"], ShouldEqual, "fluid")
		})
	})
}
