package physics

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
)

/*
Config holds the 3D torus manifold resolution and ideal-gas constants for the GPU solver.

Domain extents are derived from book depth (X), venue lanes (Y), and universe rank (Z).
Time step comes from the integration interval — the same exchange-time lattice fluid uses.
*/
type Config struct {
	GridX                   uint32
	GridY                   uint32
	GridZ                   uint32
	DomainX                 float64
	DomainY                 float64
	DomainZ                 float64
	DeltaT                  float64
	Gamma                   float64
	CV                      float64
	RhoMin                  float64
	PMin                    float64
	KThermal                float64
	MaxModes                uint32
	snapshotPublishInterval time.Duration
}

/*
NewConfigFromViper builds manifold physics parameters from signals.manifold and market book depth.
*/
func NewConfigFromViper() (Config, error) {
	gridX := uint32(viper.GetInt("signals.manifold.grid_x"))

	if gridX < 4 {
		return Config{}, fmt.Errorf("signals.manifold.grid_x must be at least 4")
	}

	gridY := uint32(viper.GetInt("signals.manifold.grid_y"))

	if gridY < 1 {
		return Config{}, fmt.Errorf("signals.manifold.grid_y must be at least 1")
	}

	gridZ := uint32(viper.GetInt("signals.manifold.grid_z"))

	if gridZ < 4 {
		return Config{}, fmt.Errorf("signals.manifold.grid_z must be at least 4")
	}

	tickSize := viper.GetFloat64("signals.manifold.tick_size")

	if tickSize <= 0 {
		tickSize = viper.GetFloat64("signals.fluid.tick_size")
	}

	if tickSize <= 0 {
		return Config{}, fmt.Errorf("signals.manifold.tick_size must be positive")
	}

	halfWidth := viper.GetInt("signals.manifold.grid_half_width")

	if halfWidth <= 0 {
		halfWidth = viper.GetInt("signals.fluid.grid_half_width")
	}

	if halfWidth <= 0 {
		return Config{}, fmt.Errorf("signals.manifold.grid_half_width must be positive")
	}

	deltaT := viper.GetDuration("signals.manifold.integration_interval").Seconds()

	if deltaT <= 0 {
		deltaT = viper.GetDuration("signals.fluid.integration_interval").Seconds()
	}

	if deltaT <= 0 {
		return Config{}, fmt.Errorf("signals.manifold.integration_interval must be positive")
	}

	gamma := 5.0 / 3.0
	cellVolume := tickSize
	rhoMin := 1.0 / cellVolume
	pMin := (gamma - 1.0) * rhoMin * cellVolume

	maxModes := uint32(viper.GetInt("signals.manifold.max_modes"))

	if maxModes < gridZ {
		maxModes = gridZ
	}

	return Config{
		GridX:                   gridX,
		GridY:                   gridY,
		GridZ:                   gridZ,
		DomainX:                 float64(halfWidth*2+1) * tickSize,
		DomainY:                 float64(gridY),
		DomainZ:                 float64(gridZ),
		DeltaT:                  deltaT,
		Gamma:                   gamma,
		CV:                      1.0 / (gamma - 1.0),
		RhoMin:                  rhoMin,
		PMin:                    pMin,
		KThermal:                rhoMin / deltaT,
		MaxModes:                maxModes,
		snapshotPublishInterval: viper.GetDuration("signals.manifold.snapshot_interval"),
	}, nil
}

func (config Config) CellVolume() float64 {
	return config.DomainX / float64(config.GridX) *
		config.DomainY / float64(config.GridY) *
		config.DomainZ / float64(config.GridZ)
}

func (config Config) GridSpacing() float64 {
	return math.Pow(config.CellVolume(), 1.0/3.0)
}

func (config Config) HbarEffective() float64 {
	return config.GridSpacing() * config.GridSpacing() / config.DeltaT
}

func (config Config) GInteraction() float64 {
	return 1.0 / (config.HbarEffective() * float64(config.MaxModes))
}

func (config Config) EnergyDecay() float64 {
	return 1.0 / (config.DeltaT * float64(config.MaxModes))
}

func (config Config) MetabolicRate() float64 {
	return 1.0 / config.DeltaT
}

func (config Config) CouplingScale() float64 {
	return config.HbarEffective() / config.GridSpacing()
}

func (config Config) GateWidthMin() float64 {
	return 2.0 * math.Pi / (config.DeltaT * float64(config.MaxModes))
}

func (config Config) GateWidthMax() float64 {
	return 2.0 * math.Pi / config.DeltaT
}

func (config Config) IntegrationInterval() time.Duration {
	return time.Duration(config.DeltaT * float64(time.Second))
}

func (config Config) SnapshotPublishInterval() time.Duration {
	return config.snapshotPublishInterval
}
