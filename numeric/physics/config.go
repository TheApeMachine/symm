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
	GasEnvelopeRhoMin       float64
	GasPMin                 float64
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

	maxModes := uint32(viper.GetInt("signals.manifold.max_modes"))

	if maxModes < gridZ {
		maxModes = gridZ
	}

	config := Config{
		GridX:                   gridX,
		GridY:                   gridY,
		GridZ:                   gridZ,
		DomainX:                 float64(halfWidth*2+1) * tickSize,
		DomainY:                 float64(gridY),
		DomainZ:                 float64(gridZ),
		DeltaT:                  deltaT,
		Gamma:                   gamma,
		MaxModes:                maxModes,
		snapshotPublishInterval: viper.GetDuration("signals.manifold.snapshot_interval"),
	}

	cellVolume := config.CellVolume()

	if cellVolume <= 0 {
		return Config{}, fmt.Errorf("signals.manifold grid produced non-positive cell volume")
	}

	ApplyDerivedGasParams(&config)

	if validateErr := config.Validate(); validateErr != nil {
		return Config{}, validateErr
	}

	return config, nil
}

/*
ApplyDerivedGasParams sets thermodynamic floors and CFL-stable thermal conductivity.

RhoMin is the cell-volume reference density used for deposits and PIC mass scale.
GasEnvelopeRhoMin is the per-carrier low-density threshold for primitive recovery.
KThermal is chosen so explicit 3D diffusion satisfies the von Neumann limit 1/6.
*/
func ApplyDerivedGasParams(config *Config) {
	if config == nil {
		return
	}

	gamma := config.Gamma

	if gamma <= 1.0 {
		gamma = 5.0 / 3.0
		config.Gamma = gamma
	}

	cellVolume := config.CellVolume()
	rhoMin := 1.0 / cellVolume

	config.CV = 1.0 / (gamma - 1.0)
	config.RhoMin = rhoMin
	config.PMin = (gamma - 1.0) * rhoMin * cellVolume

	carrierCount := config.MaxModes

	if carrierCount == 0 {
		carrierCount = 1
	}

	envelopeRho := rhoMin / float64(carrierCount)
	config.GasEnvelopeRhoMin = envelopeRho
	config.GasPMin = (gamma - 1.0) * envelopeRho * cellVolume

	gasCellSpacing := config.GasCellSpacing()
	const diffusionCFLSafety = 0.99
	config.KThermal = envelopeRho * config.CV * gasCellSpacing * gasCellSpacing /
		(6.0 * config.DeltaT) * diffusionCFLSafety
}

/*
GasCellSpacing returns the x-axis cell width used by the gas diffusion stencil.
*/
func (config Config) GasCellSpacing() float64 {
	return config.DomainX / float64(config.GridX)
}

/*
DiffusionCFL returns k_thermal·dt / (ρ_envelope·c_v·dx²) for explicit 3D diffusion.
*/
func (config Config) DiffusionCFL() float64 {
	gasCellSpacing := config.GasCellSpacing()
	envelopeRho := config.GasEnvelopeRhoMin

	if envelopeRho <= 0 || config.CV <= 0 || gasCellSpacing <= 0 || config.DeltaT <= 0 {
		return math.Inf(1)
	}

	return config.KThermal * config.DeltaT / (envelopeRho * config.CV * gasCellSpacing * gasCellSpacing)
}

/*
Validate rejects configs whose explicit thermal diffusion violates von Neumann stability.
*/
func (config Config) Validate() error {
	diffusionCFL := config.DiffusionCFL()
	const vonNeumannLimit = 1.0 / 6.0

	if diffusionCFL > vonNeumannLimit+1e-9 {
		return fmt.Errorf(
			"physics: diffusion CFL %.6g exceeds von Neumann limit %.6g (k_thermal=%.6g rho_envelope=%.6g)",
			diffusionCFL,
			vonNeumannLimit,
			config.KThermal,
			config.GasEnvelopeRhoMin,
		)
	}

	return nil
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
