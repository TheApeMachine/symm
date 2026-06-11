package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/user"
)

func TestUiWireFrame(t *testing.T) {
	Convey("Given dashboard ui bus rows", t, func() {
		Convey("It should preserve payload wire types for fluid snapshots", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "fluid",
				Value: map[string]any{
					"type":    "fluid",
					"symbols": []map[string]any{{"symbol": "BTC/USD"}},
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "fluid")
		})

		Convey("It should fall back to the bus type when payload has no wire type", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "gauge",
				Value: map[string]any{
					"source":     "fluid",
					"confidence": 0.42,
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "gauge")
			So(frame["confidence"], ShouldEqual, 0.42)
		})

		Convey("It should shape balances for the dashboard store", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "balances",
				Value: user.Balances{
					Asset: []user.Balance{{
						Asset:   "ZUSD",
						Balance: 10000,
					}},
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "balances")
			So(frame["balanceLabel"], ShouldEqual, "$10000.00")
			So(frame["symbol"], ShouldEqual, "$")

			assets, ok := frame["assets"].(map[string]any)

			So(ok, ShouldBeTrue)

			assetRows, ok := assets["asset"].([]user.Balance)

			So(ok, ShouldBeTrue)
			So(len(assetRows), ShouldEqual, 1)
		})

		Convey("It should preserve prediction chart rows", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "prediction",
				Value: map[string]any{
					"kind":    "prediction",
					"x":       1_710_000_060.0,
					"value":   0.03,
					"horizon": 60.0,
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "prediction")
			So(frame["kind"], ShouldEqual, "prediction")
			So(frame["x"], ShouldEqual, 1_710_000_060.0)
			So(frame["value"], ShouldEqual, 0.03)
		})

		Convey("It should preserve manifold rho snapshots", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "manifold",
				Value: map[string]any{
					"type": "manifold",
					"grid": map[string]any{
						"x":       32.0,
						"y":       3.0,
						"z":       16.0,
						"spacing": 1.5,
					},
					"rho": [][]float64{
						{0, 1, 2},
						{1, 3, 4},
					},
					"carriers": []map[string]any{{
						"role":   "symbol",
						"symbol": "BTC/USD",
						"x":      1.0,
						"z":      2.0,
					}},
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "manifold")

			rho, ok := frame["rho"].([][]float64)

			So(ok, ShouldBeTrue)
			So(len(rho), ShouldEqual, 2)
			So(rho[0][2], ShouldEqual, 2.0)

			grid, ok := frame["grid"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(grid["spacing"], ShouldEqual, 1.5)
		})

		Convey("It should forward gauge warmup fields unchanged", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "gauge",
				Value: map[string]any{
					"source":      "fluid",
					"confidence":  0.0,
					"surprise":    0.0,
					"samples":     12,
					"min_samples": 64,
					"calibrating": true,
					"calibrated":  false,
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "gauge")
			So(frame["calibrating"], ShouldBeTrue)
			So(frame["samples"], ShouldEqual, 12)
			So(frame["min_samples"], ShouldEqual, 64)
		})

		Convey("It should forward story counters", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "story",
				Value: map[string]any{
					"story_ticks":          42,
					"playbook_evaluations": 7,
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "story")
			So(frame["story_ticks"], ShouldEqual, 42)
			So(frame["playbook_evaluations"], ShouldEqual, 7)
		})

		Convey("It should encode decision tree branches for the dashboard", func() {
			frame, err := uiWireFrame(&qpool.QValue[any]{
				Type: "decision_tree",
				Value: map[string]any{
					"chart": "decision_tree",
					"branches": []map[string]any{{
						"condition_group": map[string]any{
							"boolean": "and",
							"conditions": []map[string]any{{
								"type": "is_true",
								"left": map[string]any{
									"subject": map[string]any{
										"type": "holding",
										"holding": map[string]any{
											"held": true,
										},
									},
								},
							}},
						},
					}},
				},
			})

			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "decision_tree")

			branches, ok := frame["branches"].([]any)

			So(ok, ShouldBeTrue)
			So(len(branches), ShouldEqual, 1)
		})
	})
}
