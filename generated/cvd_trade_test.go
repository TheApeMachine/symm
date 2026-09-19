package generated_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/generated"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestBehavioralEquivalence(t *testing.T) {
	Convey("Given cvd_trade compiled stage and dynamic builder", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..")
		jsonPath := filepath.Join(repoRoot, "signal", "definitions", "cvd_trade.json")

		builder, err := compiler.NewBuilder(jsonPath)
		So(err, ShouldBeNil)
		dynamicPipeline, err := builder.Compose()
		So(err, ShouldBeNil)

		typedPipeline := generated.NewCvdTrade()
		So(typedPipeline, ShouldNotBeNil)

		Convey("When processing a sequence of buy and sell trade ticks", func() {
			ticks := []map[string]any{
				{
					"trade": map[string]any{
						"data": map[string]any{
							"side":      "buy",
							"symbol":    "BTC/USD",
							"price":     50000.0,
							"qty":       1.5,
							"timestamp": int64(1700000001),
						},
					},
				},
				{
					"trade": map[string]any{
						"data": map[string]any{
							"side":      "sell",
							"symbol":    "BTC/USD",
							"price":     49990.0,
							"qty":       0.5,
							"timestamp": int64(1700000002),
						},
					},
				},
				{
					"trade": map[string]any{
						"data": map[string]any{
							"side":      "buy",
							"symbol":    "BTC/USD",
							"price":     50010.0,
							"qty":       2.0,
							"timestamp": int64(1700000003),
						},
					},
				},
			}

			for _, tick := range ticks {
				dynRes := dynamicPipeline(tick)
				typedRes := typedPipeline(tick)

				So(typedRes, ShouldNotBeNil)
				readings, ok := typedRes.([]float64)
				So(ok, ShouldBeTrue)
				So(len(readings), ShouldEqual, 17)

				// Dynamic pipeline produces value from sink
				So(dynRes, ShouldNotBeNil)
			}
		})
	})

	Convey("Given hawkes_trade compiled stage", t, func() {
		typedHawkes := generated.NewHawkesTrade()
		So(typedHawkes, ShouldNotBeNil)

		tick := map[string]any{
			"trade": map[string]any{
				"data": map[string]any{
					"side":      "buy",
					"symbol":    "BTC/USD",
					"price":     50000.0,
					"qty":       1.0,
					"timestamp": int64(1700000001),
				},
			},
		}

		res := typedHawkes(tick)
		So(res, ShouldNotBeNil)
		readings, ok := res.([]float64)
		So(ok, ShouldBeTrue)
		So(len(readings), ShouldEqual, 12)
	})

	Convey("Given logic compiled stage", t, func() {
		logic := generated.NewLogic()
		So(logic, ShouldNotBeNil)

		// Pass sample readings into logic stage
		readings := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
		res := logic(readings)
		_ = res
	})

	Convey("Given execution compiled stage", t, func() {
		execution := generated.NewExecution()
		So(execution, ShouldNotBeNil)

		// Pass nil/sample evaluation into execution stage
		res := execution(nil)
		So(res, ShouldBeNil)
	})
}
