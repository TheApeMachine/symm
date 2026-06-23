package fluid

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestScoreDump(t *testing.T) {
	configureFluidViper()
	signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	signal.SetInstrumentTickSize("ETH/EUR", 0.01)

	for index, frame := range laminarStabilityFrames("ETH/EUR") {
		at := base.Add(time.Duration(index) * integrationSpacing()).UnixNano()
		datapoint := marketDatapoint(frame.channel, frame.payload, at)
		signal.Measure(datapoint)

		state := signal.registry.loadSymbol("ETH/EUR")
		reading, ok := state.Reading()

		if ok {
			fmt.Printf("%d re=%.4f div=%.4f visc=%.4f hist=%d\n", index,
				reading.reynolds, reading.divergence, reading.viscosity, len(reading.dynamics.reynoldsHistory))
		}

		datapoint.Release()

		measured := measureMarketFrame(signal, frame.channel, frame.payload, at)

		if measured == nil {
			fmt.Printf("%d nil\n", index)

			continue
		}

		fmt.Printf("%d lam=%.4f inert=%.4f turb=%.4f visc=%.4f conf=%.4f\n", index,
			outputScore(measured, "laminarScore"), outputScore(measured, "inertialScore"),
			outputScore(measured, "turbulentScore"), outputScore(measured, "viscousScore"),
			datura.Peek[float64](measured, "output", "confidence"))
		measured.Release()
	}

	_ = signal.Close()
}
