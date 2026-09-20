package compiler_test

import (
	"context"
	"path/filepath"
	goruntime "runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestCompile(t *testing.T) {
	Convey("Given the generic in-memory compiler", t, func() {
		reg := compiler.DefaultRegistry()
		So(reg, ShouldNotBeNil)
		repo := definitions.Default()
		reg.SetRepository(repo)

		Convey("Compiling the master system graph", func() {
			systemGraph, err := definitions.Load("system")
			So(err, ShouldBeNil)
			So(len(systemGraph.Nodes), ShouldBeGreaterThan, 0)

			systemPipeline, err := compiler.Compile[any, any](systemGraph, reg, repo)
			So(err, ShouldBeNil)
			So(systemPipeline, ShouldNotBeNil)

			Convey("A market trade tick executes through the compiled closure", func() {
				tick := map[string]any{
					"trade": map[string]any{
						"data": map[string]any{
							"side":      "buy",
							"symbol":    "BTC/USD",
							"price":     50000.0,
							"qty":       1.5,
							"timestamp": int64(1700000000),
						},
					},
				}

				result := systemPipeline.WriteAny(context.Background(), tick)
				_ = result
			})
		})

		Convey("Compiling a linear stage graph (logic)", func() {
			logicGraph, err := definitions.Load("logic")
			So(err, ShouldBeNil)

			logicPipeline, err := compiler.Compile[any, any](logicGraph, reg, repo)
			So(err, ShouldBeNil)
			So(logicPipeline, ShouldNotBeNil)

			readings := []float64{0.5, -0.2, 1.1, 0.0}
			result := logicPipeline.WriteAny(context.Background(), readings)
			_ = result
		})

		Convey("Compiling a fan-out signal graph (derivatives:trade)", func() {
			derivGraph, err := definitions.Load("derivatives:trade")
			So(err, ShouldBeNil)

			derivPipeline, err := compiler.Compile[any, any](derivGraph, reg, repo)
			So(err, ShouldBeNil)
			So(derivPipeline, ShouldNotBeNil)

			fillData := map[string]any{
				"symbol":    "PF_SOLUSD",
				"side":      "buy",
				"price":     150.0,
				"qty":       10.0,
				"type":      "liquidation",
				"timestamp": int64(1700000000),
			}

			err = derivPipeline.WriteAny(context.Background(), fillData)
			So(err, ShouldBeNil)
		})

		Convey("Compiling a fan-out signal graph (cvd:trade)", func() {
			cvdGraph, err := definitions.Load("cvd:trade")
			So(err, ShouldBeNil)

			cvdPipeline, err := compiler.Compile[any, any](cvdGraph, reg, repo)
			So(err, ShouldBeNil)
			So(cvdPipeline, ShouldNotBeNil)

			tradeData := map[string]any{
				"symbol":    "PF_SOLUSD",
				"side":      "buy",
				"price":     150.0,
				"qty":       10.0,
				"timestamp": int64(1700000000),
			}

			err = cvdPipeline.WriteAny(context.Background(), tradeData)
			So(err, ShouldBeNil)
		})

		Convey("Compiling hawkes:trade signal graph", func() {
			hawkesGraph, err := definitions.Load("hawkes:trade")
			So(err, ShouldBeNil)

			hawkesPipeline, err := compiler.Compile[any, []float64](hawkesGraph, reg, repo)
			So(err, ShouldBeNil)
			So(hawkesPipeline, ShouldNotBeNil)

			tradeData := map[string]any{
				"symbol":    "BTC/USD",
				"side":      "buy",
				"price":     50000.0,
				"qty":       1.5,
				"timestamp": int64(1700000000000000000),
			}

			var metrics []any
			hawkesPipeline.SetDownstreamAny(func(ctx context.Context, in any) error {
				// Just capture anything it receives for the test assertion
				if m, ok := in.([]any); ok {
					metrics = append(metrics, m...)
				} else if m, ok := in.([]float64); ok {
					for _, v := range m {
						metrics = append(metrics, v)
					}
				} else if in != nil {
					metrics = append(metrics, in)
				}
				return nil
			})
			err = hawkesPipeline.WriteAny(context.Background(), tradeData)
			So(err, ShouldBeNil)
			So(len(metrics), ShouldBeGreaterThan, 0)
		})

		Convey("Cycle detection in graph", func() {
			cycleGraph := compiler.Graph{
				ID:   "cycle:test",
				Name: "cycle:test",
				Nodes: map[string]compiler.Node{
					"n1": {
						ID:   "n1",
						Type: "arithmetic.Add",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "n2", PortName: "in"}},
							},
						},
					},
					"n2": {
						ID:   "n2",
						Type: "arithmetic.Add",
						Connections: compiler.Connections{
							Outputs: map[string][]compiler.ConnectionTarget{
								"out": {{NodeID: "n1", PortName: "in"}},
							},
						},
					},
				},
			}

			_, err := compiler.Compile[any, any](cycleGraph, reg, repo)
			So(err, ShouldNotBeNil)
		})

		Convey("CompileFile helper compiles directly from path", func() {
			_, thisFile, _, _ := goruntime.Caller(0)
			repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
			systemPath := filepath.Join(repoRoot, "signal", "definitions", "system.json")

			fn, err := compiler.CompileFile[any, any](systemPath, reg, repo)
			So(err, ShouldBeNil)
			So(fn, ShouldNotBeNil)
		})
	})
}
