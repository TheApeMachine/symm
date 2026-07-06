package logic

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func decisionManifoldConfig() (pmanifold.Config, error) {
	bookDepth := viper.GetInt("market.book.depth")
	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	if bookDepth <= 0 {
		return pmanifold.Config{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: market book depth required",
			nil,
		))
	}

	interval := viper.GetDuration("telemetry.gauge.publish_interval")
	if interval <= 0 {
		return pmanifold.Config{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: telemetry.gauge.publish_interval required",
			nil,
		))
	}

	gridX := uint32(bookDepth)
	gridY := uint32(len(boundarySourceOrder))
	gridZ := uint32(bookDepth)

	config := pmanifold.Config{
		GridX:    gridX,
		GridY:    gridY,
		GridZ:    gridZ,
		DomainX:  float64(gridX),
		DomainY:  float64(gridY),
		DomainZ:  float64(gridZ),
		DeltaT:   interval.Seconds(),
		Gamma:    5.0 / 3.0,
		MaxModes: uint32(len(boundarySourceOrder)),
	}
	pmanifold.ApplyDerivedGasParams(&config)

	if err := config.Validate(); err != nil {
		return pmanifold.Config{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: manifold configuration validation failed",
			err,
		))
	}

	config.SetSnapshotPublishInterval(interval)
	return config, nil
}
