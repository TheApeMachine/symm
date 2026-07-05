package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPositionsReadings(t *testing.T) {
	Convey("Given positions fed by balance, execution, ticker, and stop frames", t, func() {
		positions := NewPositions("USD")
		at := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

		So(positions.Observe(map[string]any{
			"role":      "balances",
			"timestamp": at.Format(time.RFC3339Nano),
			"data": []any{
				map[string]any{"asset": "USD", "balance": 1000.0},
				map[string]any{"asset": "XBT", "balance": 2.0},
			},
		}), ShouldBeNil)

		So(positions.Observe(map[string]any{
			"role": "executions",
			"data": []any{
				map[string]any{"symbol": "XBT/USD", "side": "buy", "avg_price": 100.0},
			},
		}), ShouldBeNil)

		So(positions.Observe(map[string]any{
			"role": "ticker",
			"data": []any{
				map[string]any{"symbol": "XBT/USD", "last": 150.0},
			},
		}), ShouldBeNil)

		So(positions.Observe(map[string]any{
			"role":   "stoploss",
			"symbol": "XBT/USD",
			"stop":   140.0,
			"peak":   155.0,
			"offset": 0.1,
			"side":   "sell",
		}), ShouldBeNil)

		Convey("When readings are requested", func() {
			readings, err := positions.Readings()

			Convey("Then the backend derives the position economics directly", func() {
				So(err, ShouldBeNil)
				So(readings, ShouldHaveLength, 1)
				So(readings[0]["symbol"], ShouldEqual, "XBT/USD")
				So(readings[0]["quantity"], ShouldEqual, 2.0)
				So(readings[0]["entry"], ShouldEqual, 100.0)
				So(readings[0]["mark"], ShouldEqual, 150.0)
				So(readings[0]["value"], ShouldEqual, 300.0)
				So(readings[0]["unrealizedPnl"], ShouldEqual, 100.0)
				So(readings[0]["changePct"], ShouldEqual, 50.0)
				So(readings[0]["status"], ShouldEqual, "marked")
				So(readings[0]["stop"], ShouldEqual, 140.0)
				So(readings[0]["peak"], ShouldEqual, 155.0)
				So(readings[0]["offset"], ShouldEqual, 0.1)
				So(readings[0]["stopSide"], ShouldEqual, "sell")
				So(readings[0]["updatedAt"], ShouldEqual, at.UnixNano())
			})
		})
	})

	Convey("Given a flat ledger", t, func() {
		positions := NewPositions("USD")

		So(positions.Observe(map[string]any{
			"role": "balances",
			"data": []any{
				map[string]any{"asset": "USD", "balance": 1000.0},
			},
		}), ShouldBeNil)

		Convey("When readings are requested", func() {
			readings, err := positions.Readings()

			Convey("Then no open positions are reported", func() {
				So(err, ShouldBeNil)
				So(readings, ShouldBeEmpty)
			})
		})
	})
}

func TestPositionsObserve(t *testing.T) {
	Convey("Given a wrapped executions frame", t, func() {
		positions := NewPositions("USD")

		So(positions.Observe(map[string]any{
			"role": "balances",
			"data": []any{
				map[string]any{"asset": "USD", "balance": 1000.0},
				map[string]any{"asset": "ALGO", "balance": 10.0},
			},
		}), ShouldBeNil)

		So(positions.Observe(map[string]any{
			"role": "executions",
			"executions": map[string]any{
				"exec-1": map[string]any{
					"symbol":     "ALGO/USD",
					"side":       "buy",
					"last_price": 0.10,
				},
			},
		}), ShouldBeNil)

		So(positions.Observe(map[string]any{
			"channel": "quote",
			"data": []any{
				map[string]any{"symbol": "ALGO/USD", "price": 0.12},
			},
		}), ShouldBeNil)

		Convey("When readings are requested", func() {
			readings, err := positions.Readings()

			Convey("Then the entry and mark come from the observed frames", func() {
				So(err, ShouldBeNil)
				So(readings, ShouldHaveLength, 1)
				So(readings[0]["symbol"], ShouldEqual, "ALGO/USD")
				So(readings[0]["entry"], ShouldEqual, 0.10)
				So(readings[0]["mark"], ShouldEqual, 0.12)
				So(readings[0]["status"], ShouldEqual, "marked")
			})
		})
	})

	Convey("Given a malformed balances frame", t, func() {
		positions := NewPositions("USD")

		Convey("When it is observed", func() {
			err := positions.Observe(map[string]any{
				"role": "balances",
				"data": []any{
					map[string]any{"balance": 10.0},
				},
			})

			Convey("Then the missing asset is reported", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}
