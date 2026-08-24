package tests

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	tes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
TestPipelineFrameInventory drives the real system against the fixture venue and
reports which UI frame types actually reach the bus, so a dead panel can be
traced to its producer stage without guessing.
*/
func TestPipelineFrameInventory(t *testing.T) {
	symbols := []*tes.Symbol{
		tes.NewSymbol("SMOKE/USD", 1.0, 10),
		tes.NewSymbol("BRICK/USD", 1.0, 10),
		tes.NewSymbol("PLANK/USD", 1.0, 10),
	}

	Convey("Given the assembled system driven by the fixture venue", t,
		WithStack(t, symbols, cmd.Boot, func(market *Market, system *cmd.System) {
			types.SetFocus("PLANK/USD")

			ui := runtime.ChannelOf[*types.UIFrame](
				system.Bus, types.ChannelUI,
				func(frame *types.UIFrame) string { return "" },
			)

			var mu sync.Mutex
			counts := make(map[wire.Frame]int)
			ui.Subscribe("frame-inventory", func(frame *types.UIFrame) error {
				mu.Lock()
				counts[frame.Type]++
				mu.Unlock()

				return nil
			})

			measurements := make(map[string]int)
			runtime.ChannelOf[*nmtypes.Measurement](
				system.Bus, types.ChannelMeasurements,
				func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
			).Subscribe("measurement-inventory", func(measurement *nmtypes.Measurement) error {
				mu.Lock()
				measurements[measurement.Source]++
				mu.Unlock()

				return nil
			})

			categoryCount := 0
			runtime.ChannelOf[[]types.Category](
				system.Bus, types.ChannelCategories,
				func(batch []types.Category) string {
					if len(batch) == 0 {
						return ""
					}
					return batch[0].Symbol
				},
			).Subscribe("category-inventory", func(batch []types.Category) error {
				mu.Lock()
				categoryCount++
				mu.Unlock()

				return nil
			})

			causalCount := 0
			runtime.ChannelOf[types.CausalOutput](
				system.Bus, types.ChannelCausal,
				func(output types.CausalOutput) string { return output.Symbol },
			).Subscribe("causal-inventory", func(output types.CausalOutput) error {
				mu.Lock()
				causalCount++
				mu.Unlock()

				return nil
			})

			graphCount := 0
			runtime.ChannelOf[*types.Graph](
				system.Bus, types.ChannelGraphs,
				func(graph *types.Graph) string { return graph.Symbol },
			).Subscribe("graph-inventory", func(graph *types.Graph) error {
				mu.Lock()
				graphCount++
				mu.Unlock()

				return nil
			})

			tickCount := 200

			if override := os.Getenv("PROBE_TICKS"); override != "" {
				if parsed, err := strconv.Atoi(override); err == nil && parsed > 0 {
					tickCount = parsed
				}
			}

			for index := 0; index < tickCount; index++ {
				market.Tick()
			}

			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				mu.Lock()
				total := 0

				for _, count := range counts {
					total += count
				}
				mu.Unlock()

				if total > 200 {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			mu.Lock()
			t.Logf("UI frame type counts: %v", counts)
			t.Logf("measurements by source: %v", measurements)
			t.Logf("category batches: %d", categoryCount)
			t.Logf("causal outputs: %d", causalCount)
			t.Logf("graphs published: %d", graphCount)
			mu.Unlock()
		}),
	)
}
