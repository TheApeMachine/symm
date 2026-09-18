package signal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestCompilerAndLoader(t *testing.T) {
	Convey("Given the embedded signal definitions and a market grid", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()

		Convey("ListDefinitions reports embedded signal definitions", func() {
			defs, err := ListDefinitions()
			So(err, ShouldBeNil)
			So(len(defs), ShouldBeGreaterThanOrEqualTo, 1)
			So(defs, ShouldContain, "correlation_ticker")
		})

		Convey("GetDefinition resolves with or without colon syntax", func() {
			data1, err1 := GetDefinition("correlation_ticker")
			So(err1, ShouldBeNil)
			So(len(data1), ShouldBeGreaterThan, 0)

			data2, err2 := GetDefinition("correlation:ticker")
			So(err2, ShouldBeNil)
			So(len(data2), ShouldEqual, len(data1))
		})

		Convey("Load successfully compiles correlation:ticker and registers on grid", func() {
			sig, err := Load(t.Context(), grid, "correlation:ticker", "BTC/USD")
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeNil)
			So(sig.Name(), ShouldEqual, "correlation:ticker")
			So(sig.Symbol(), ShouldEqual, "BTC/USD")
			So(sig.Holds(), ShouldContainKey, "lastPrice")

			Convey("when a ticker event is routed through the grid", func() {
				query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					nil, core.Execute,
				)
				execPipe := nomagique.NewNumber(query, grid)

				for range execPipe.Next(sequence.NewValue[any](map[string]any{
					"ticker": map[string]any{
						"data": map[string]any{
							"symbol":    "BTC/USD",
							"last":      65432.5,
							"timestamp": time.Now().UnixNano(),
						},
					},
				})) {
				}

				Convey("Signal.Next yields updated Measurement with local metrics", func() {
					var measured *data.Measurement[float64]

					for ptr := range sig.Next(nil) {
						if ptr != nil {
							measured = *(**data.Measurement[float64])(ptr)
							break
						}
					}

					So(measured, ShouldNotBeNil)
					So(measured.Source, ShouldEqual, "correlation:ticker")
					So(measured.Label, ShouldEqual, "BTC/USD")
					So(measured.Metrics, ShouldContainKey, "lastPrice")
					So(measured.Metrics["lastPrice"].Raw, ShouldEqual, 65432.5)
				})
			})
		})

		Convey("Given a graph with constructor-nested child and fan-out", func() {
			nestingJSON := []byte(`{
				"id": "test_nesting",
				"name": "test_nesting",
				"source": "test",
				"nodes": {
					"src": {
						"id": "src",
						"type": "source",
						"connections": {
							"outputs": {
								"value": [{ "nodeId": "pairs_node", "portName": "in" }]
							}
						}
					},
					"estimator_node": {
						"id": "estimator_node",
						"type": "algo.HayashiYoshida",
						"connections": {
							"outputs": {
								"out": [{ "nodeId": "pairs_node", "portName": "estimator" }]
							}
						}
					},
					"pairs_node": {
						"id": "pairs_node",
						"type": "correlation.Pairs",
						"inputData": {
							"_config": {
								"measured": "BTC",
								"reference": "ETH"
							}
						},
						"connections": {
							"inputs": {
								"in": [{ "nodeId": "src", "portName": "value" }],
								"estimator": [{ "nodeId": "estimator_node", "portName": "out" }]
							},
							"outputs": {
								"out": [
									{ "nodeId": "sink1", "portName": "value" },
									{ "nodeId": "sink2", "portName": "value" }
								]
							}
						}
					},
					"sink1": {
						"id": "sink1",
						"type": "sink",
						"inputData": { "_config": { "metric": "m1" } },
						"connections": {
							"inputs": { "value": [{ "nodeId": "pairs_node", "portName": "out" }] }
						}
					},
					"sink2": {
						"id": "sink2",
						"type": "sink",
						"inputData": { "_config": { "metric": "m2" } },
						"connections": {
							"inputs": { "value": [{ "nodeId": "pairs_node", "portName": "out" }] }
						}
					}
				}
			}`)

			sig, err := Compile(t.Context(), grid, nestingJSON, "BTC")
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeNil)
			So(sig.Holds(), ShouldContainKey, "m1")
			So(sig.Holds(), ShouldContainKey, "m2")
		})

		Convey("LoadAll successfully compiles and registers all embedded signal definitions", func() {
			signals, err := LoadAll(t.Context(), grid)
			So(err, ShouldBeNil)
			So(len(signals), ShouldEqual, 15)

			for _, sig := range signals {
				So(sig, ShouldNotBeNil)
				So(len(sig.Holds()), ShouldBeGreaterThan, 0)
			}
		})
	})
}

