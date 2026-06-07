package broker

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestExecutionStressMultiplier(t *testing.T) {
	Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := ExecutionStressMultiplier(snapshots)

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})

	Convey("Given hostile symbol stress in a bearish regime", t, func() {
		perspectives.PublishRegime(types.RegimeBearish)

		multiplier := ExecutionStressFromSymbol(SymbolStress{
			FluidCategory: types.CategoryTurbulent,
			FluidSNR:      2,
		})

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})
}

func TestEffectiveNetworkLatency(t *testing.T) {
	Convey("Given a latency ring file", t, func() {
		tempDir := t.TempDir()
		latencyPath := filepath.Join(tempDir, "network_latency.json")
		err := os.WriteFile(latencyPath, []byte("10000000\n50000000\n20000000\n"), 0o600)
		So(err, ShouldBeNil)

		originalWD, wdErr := os.Getwd()
		So(wdErr, ShouldBeNil)

		runsDir := filepath.Join(tempDir, "runs")
		mkdirErr := os.Mkdir(runsDir, 0o755)
		So(mkdirErr, ShouldBeNil)

		copyErr := os.WriteFile(
			filepath.Join(runsDir, "network_latency.json"),
			[]byte("10000000\n50000000\n20000000\n"),
			0o600,
		)
		So(copyErr, ShouldBeNil)

		chdirErr := os.Chdir(tempDir)
		So(chdirErr, ShouldBeNil)

		defer func() {
			_ = os.Chdir(originalWD)
		}()

		latency := EffectiveNetworkLatency()

		Convey("It should return the p95 sample instead of the sum", func() {
			So(latency, ShouldEqual, 20_000_000)
			So(latency, ShouldNotEqual, 80_000_000)
		})
	})
}

func BenchmarkExecutionStressMultiplier(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
		{Category: types.CategoryLaminar, SNR: 0.5},
	}

	for b.Loop() {
		_ = ExecutionStressMultiplier(snapshots)
	}
}
