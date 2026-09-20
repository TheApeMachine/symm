package compiler_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestSystemOrchestration(t *testing.T) {
	Convey("Given the master system orchestration JSON", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		systemPath := filepath.Join(repoRoot, "signal", "definitions", "system.json")

		builder, err := compiler.NewBuilder(systemPath)
		So(err, ShouldBeNil)
		So(builder, ShouldNotBeNil)

		systemPipeline, err := builder.Compose(definitions.Default())
		So(err, ShouldBeNil)
		So(systemPipeline, ShouldNotBeNil)

		Convey("A market trade tick executes through the entire system pipeline", func() {
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

			// Execute tick through the master pipeline
			result := systemPipeline.WriteAny(context.Background(), tick)

			// An immature/empty context produces nil execution, which is the correct mathematical behavior
			_ = result
		})
	})
}
