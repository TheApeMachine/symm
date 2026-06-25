package manifold

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/statutil"
)

func NewField() (*Field, error) {
	bookDepth := activeBookDepth()

	if bookDepth <= 0 {
		return nil, fmt.Errorf("manifold: book depth must be positive")
	}

	symbolCount := activeSymbolCount()

	gridX := uint32(bookDepth * 4)
	gridY := max(uint32(symbolCount), 3)
	gridZ := uint32(math.Max(3, math.Log2(float64(symbolCount*2))))
	halfWidth := bookDepth * 3

	integrationInterval := viper.GetDuration("signals.manifold.integration_interval")
	deltaT := integrationDeltaT(integrationInterval, bookDepth, symbolCount)
	gamma := 1 + 2.0/float64(gridZ)
	tickSize := fieldTickSize(bookDepth, symbolCount)
	maxModes := gridX * gridY

	kernelConfig, configErr := mkernel.NewConfig(
		gridX,
		gridY,
		gridZ,
		tickSize,
		halfWidth,
		deltaT,
		gamma,
		maxModes,
	)

	if configErr != nil {
		return nil, configErr
	}

	if integrationInterval <= 0 {
		integrationInterval = 100 * time.Millisecond
	}

	kernelConfig.SetSnapshotPublishInterval(integrationInterval * time.Duration(symbolCount))

	universe, universeErr := NewUniverse(kernelConfig)

	if universeErr != nil {
		return nil, universeErr
	}

	return &Field{
		config:               kernelConfig,
		universe:             universe,
		measurementsCapacity: fieldMeasurementsCapacity(integrationInterval, symbolCount),
	}, nil
}

func activeBookDepth() int {
	bookDepth := viper.GetInt("market.book.depth")

	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	return bookDepth
}

func activeSymbolCount() int {
	return max(len(viper.GetStringSlice("market.default_symbols")), 1)
}

func integrationDeltaT(integrationInterval time.Duration, bookDepth, symbolCount int) float64 {
	deltaT := integrationInterval.Seconds()

	if deltaT > 0 {
		return deltaT
	}

	return float64(bookDepth) / float64(symbolCount)
}

func fieldTickSize(bookDepth, symbolCount int) float64 {
	tickSize := viper.GetFloat64("signals.manifold.tick_size")

	if tickSize > 0 {
		return tickSize
	}

	return 1.0 / math.Pow(2, float64(bookDepth*symbolCount))
}

func fieldMeasurementsCapacity(integrationInterval time.Duration, symbolCount int) int {
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

	return statutil.SampleBudgetFromCadence(cadence) * symbolCount
}

func ManifoldBatchCapacity() int {
	symbolCount := activeSymbolCount()
	bookDepth := activeBookDepth()

	if bookDepth <= 0 {
		bookDepth = 1
	}

	cadence := viper.GetDuration("signals.manifold.integration_interval").Seconds()

	if cadence <= 0 {
		cadence = float64(bookDepth)
	}

	return symbolCount * bookDepth * statutil.SampleBudgetFromCadence(cadence)
}

func ManifoldFlushInterval() time.Duration {
	symbolCount := activeSymbolCount()
	integrationInterval := viper.GetDuration("signals.manifold.integration_interval")

	if integrationInterval <= 0 {
		integrationInterval = 100 * time.Millisecond
	}

	return integrationInterval * time.Duration(symbolCount)
}
