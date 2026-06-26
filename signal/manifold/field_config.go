package manifold

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/statutil"
)

func NewField() (*Field, error) {
	bookDepth := activeBookDepth()

	if bookDepth <= 0 {
		return nil, fmt.Errorf("manifold: book depth must be positive")
	}

	// Torus axes describe what they project, not how many symbols exist:
	//   X — price-level depth resolution
	//   Y — instrument lane (spot / perpetual / dated future)
	//   Z — cross-asset rank fidelity (recomputeRanks compresses any live
	//       universe size into these buckets, so Z is resolution, not count)
	gridX := uint32(bookDepth * 4)
	gridY := uint32(LaneCount)
	gridZ := uint32(bookDepth)
	halfWidth := bookDepth * 3

	integrationInterval := viper.GetDuration("signals.manifold.integration_interval")
	deltaT := integrationDeltaT(integrationInterval, bookDepth)
	gamma := 1 + 2.0/float64(gridZ)
	tickSize := fieldTickSize(bookDepth)

	// maxModes is the GPU carrier budget (oscillators per Metal threadgroup),
	// a hardware bound — NOT a grid volume. The solver rejects any value above
	// the device's maxTotalThreadsPerThreadgroup, so it is clamped to the cells
	// that actually carry mass but never above the guaranteed-portable floor.
	// ponytail: 256 is the Metal-guaranteed threadgroup minimum (the ceiling);
	// the upgrade path is querying manifold_max_carriers_for_pipeline before
	// building the config once nomagique exports it.
	const deviceCarrierFloor = 256
	maxModes := min(gridX*gridY, uint32(deviceCarrierFloor))

	kernelConfig, err := mkernel.NewConfig(
		gridX,
		gridY,
		gridZ,
		tickSize,
		halfWidth,
		deltaT,
		gamma,
		maxModes,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create config",
			err,
		))
	}

	if integrationInterval <= 0 {
		integrationInterval = 100 * time.Millisecond
	}

	kernelConfig.SetSnapshotPublishInterval(integrationInterval)

	universe, err := NewUniverse(kernelConfig)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create universe",
			err,
		))
	}

	return &Field{
		config:               kernelConfig,
		universe:             universe,
		measurementsCapacity: fieldMeasurementsCapacity(integrationInterval),
	}, nil
}

func activeBookDepth() int {
	bookDepth := viper.GetInt("market.book.depth")

	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	return bookDepth
}

func integrationDeltaT(integrationInterval time.Duration, bookDepth int) float64 {
	deltaT := integrationInterval.Seconds()

	if deltaT > 0 {
		return deltaT
	}

	// No configured interval: one step per book-depth resolution unit.
	return 1.0 / float64(bookDepth)
}

func fieldTickSize(bookDepth int) float64 {
	tickSize := viper.GetFloat64("signals.manifold.tick_size")

	if tickSize > 0 {
		return tickSize
	}

	// Price-level granularity is a function of book depth alone — the count of
	// symbols in the universe has nothing to do with one pair's tick size.
	return 1.0 / math.Pow(2, float64(bookDepth))
}

func fieldMeasurementsCapacity(integrationInterval time.Duration) int {
	capacity := viper.GetInt("signals.manifold.measurements_capacity")

	if capacity > 0 {
		return capacity
	}

	cadence := integrationInterval.Seconds()

	if cadence <= 0 {
		cadence = float64(activeBookDepth())
	}

	if cadence <= 0 {
		cadence = 1
	}

	// Per-symbol ring-buffer budget: this caps one symbol's return/trade
	// history (AppendReturn / recordTradeQty), so it must not scale with the
	// number of symbols in the universe.
	return statutil.SampleBudgetFromCadence(cadence)
}
