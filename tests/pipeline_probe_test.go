package tests

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/data"
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

			var mu sync.Mutex
			counts := make(map[wire.Frame]int)
			system.Bus.Wire(types.ChannelUI, "", func(value any) any {
				frame, ok := value.(*types.UIFrame)
				if !ok || frame == nil {
					return nil
				}
				mu.Lock()
				counts[frame.Type]++
				mu.Unlock()

				return nil
			})

			measurements := make(map[string]int)
			system.Bus.Wire(types.ChannelMeasurements, "", func(value any) any {
				measurement, ok := value.(*data.Measurement[float64])
				if !ok || measurement == nil {
					return nil
				}
				mu.Lock()
				measurements[measurement.Source]++
				mu.Unlock()

				return nil
			})

			categoryCount := 0
			system.Bus.Wire(types.ChannelCategories, "", func(_ any) any {
				mu.Lock()
				categoryCount++
				mu.Unlock()

				return nil
			})

			causalCount := 0
			system.Bus.Wire(types.ChannelCausal, "", func(_ any) any {
				mu.Lock()
				causalCount++
				mu.Unlock()

				return nil
			})

			graphCount := 0
			system.Bus.Wire(types.ChannelRelations, "", func(_ any) any {
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

			measurementTotal := 0
			for _, count := range measurements {
				measurementTotal += count
			}

			uiTotal := 0
			for _, count := range counts {
				uiTotal += count
			}
			mu.Unlock()

			// These assertions are the point of this probe: a stalled pipeline that
			// produces no measurements and no UI frames must FAIL here, not merely
			// log a zero and pass. The downstream category/causal/graph counts may
			// legitimately be zero on mock fixture data (ReadyForSearch needs a real
			// opportunity), so they are not asserted.
			Convey("The assembled system must flow measurements into the bus", func() {
				So(measurementTotal, ShouldBeGreaterThan, 0)
			})

			Convey("The assembled system must publish UI frames to the bus", func() {
				So(uiTotal, ShouldBeGreaterThan, 0)
			})
		}),
	)
}