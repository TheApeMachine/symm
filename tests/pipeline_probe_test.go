package tests

import (
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
			ui := runtime.ChannelOf[*types.UIFrame](
				system.Bus, types.ChannelUI,
				func(frame *types.UIFrame) string { return "" },
			)

			counts := make(map[wire.Frame]int)
			ui.Subscribe("frame-inventory", func(frame *types.UIFrame) error {
				counts[frame.Type]++

				return nil
			})

			measurements := make(map[string]int)
			runtime.ChannelOf[*nmtypes.Measurement](
				system.Bus, types.ChannelMeasurements,
				func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
			).Subscribe("measurement-inventory", func(measurement *nmtypes.Measurement) error {
				measurements[measurement.Source]++

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
				categoryCount++

				return nil
			})

			for index := 0; index < 200; index++ {
				market.Tick()
			}

			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				total := 0

				for _, count := range counts {
					total += count
				}

				if total > 200 {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			t.Logf("UI frame type counts: %v", counts)
			t.Logf("measurements by source: %v", measurements)
			t.Logf("category batches: %d", categoryCount)
		}),
	)
}
