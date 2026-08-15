package manifold

import (
	"fmt"
	"math"
	"time"
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
	BoundaryXLow            uint32
	BoundaryXHigh           uint32
	BoundaryYLow            uint32
	BoundaryYHigh           uint32
	BoundaryZLow            uint32
	BoundaryZHigh           uint32
	snapshotPublishInterval time.Duration
}

const (
	GasBoundaryPeriodic   = 0
	GasBoundaryOutflow    = 1
	GasBoundaryReflecting = 2
)

/*
GasBoundaries configures per-face gas-grid boundary topology.
*/
type GasBoundaries struct {
	XLow  uint32
	XHigh uint32
	YLow  uint32
	YHigh uint32
	ZLow  uint32
	ZHigh uint32
}

/*
DefaultMarketGasBoundaries returns the recommended open-market gas topology.
*/
func DefaultMarketGasBoundaries() GasBoundaries {
	return GasBoundaries{
		XLow:  GasBoundaryOutflow,
		XHigh: GasBoundaryOutflow,
		YLow:  GasBoundaryOutflow,
		YHigh: GasBoundaryOutflow,
		ZLow:  GasBoundaryOutflow,
		ZHigh: GasBoundaryOutflow,
	}
}

func (boundaries GasBoundaries) Apply(config *Config) {
	if config == nil {
		return
	}

	config.BoundaryXLow = boundaries.XLow
	config.BoundaryXHigh = boundaries.XHigh
	config.BoundaryYLow = boundaries.YLow
	config.BoundaryYHigh = boundaries.YHigh
	config.BoundaryZLow = boundaries.ZLow
	config.BoundaryZHigh = boundaries.ZHigh
}

type RuntimeControls struct {
	DeltaT             float64
	MetabolicRate      float64
	TopdownPhaseScale  float64
	TopdownEnergyScale float64
	GInteraction       float64
	EnergyDecay        float64
}

func (config Config) RuntimeControls() RuntimeControls {
	return RuntimeControls{
		DeltaT:        config.DeltaT,
		MetabolicRate: config.MetabolicRate(),
		GInteraction:  config.GInteraction(),
		EnergyDecay:   config.EnergyDecay(),
	}
}

func (controls RuntimeControls) Validate() error {
	values := map[string]float64{
		"delta_t":              controls.DeltaT,
		"metabolic_rate":       controls.MetabolicRate,
		"topdown_phase_scale":  controls.TopdownPhaseScale,
		"topdown_energy_scale": controls.TopdownEnergyScale,
		"g_interaction":        controls.GInteraction,
		"energy_decay":         controls.EnergyDecay,
	}

	for name, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("physics: runtime control %s must be finite", name)
		}
	}

	if controls.DeltaT <= 0 {
		return fmt.Errorf("physics: runtime control delta_t must be positive")
	}

	if controls.MetabolicRate < 0 {
		return fmt.Errorf("physics: runtime control metabolic_rate must be non-negative")
	}

	if controls.TopdownPhaseScale < 0 {
		return fmt.Errorf("physics: runtime control topdown_phase_scale must be non-negative")
	}

	if controls.TopdownEnergyScale < 0 {
		return fmt.Errorf("physics: runtime control topdown_energy_scale must be non-negative")
	}

	if controls.GInteraction <= 0 {
		return fmt.Errorf("physics: runtime control g_interaction must be positive")
	}

	if controls.EnergyDecay < 0 {
		return fmt.Errorf("physics: runtime control energy_decay must be non-negative")
	}

	return nil
}

/*
NewConfigFromViper builds manifold physics parameters from signals.manifold and market book depth.
*/
func NewConfig(
	gridX uint32,
	gridY uint32,
	gridZ uint32,
	tickSize float64,
	halfWidth int,
	deltaT float64,
	gamma float64,
	maxModes uint32,
) (Config, error) {
	config := Config{
		GridX:    gridX,
		GridY:    gridY,
		GridZ:    gridZ,
		DomainX:  float64(halfWidth*2+1) * tickSize,
		DomainY:  float64(gridY),
		DomainZ:  float64(gridZ),
		DeltaT:   deltaT,
		Gamma:    gamma,
		MaxModes: maxModes,
	}

	cellVolume := config.CellVolume()

	if cellVolume <= 0 {
		return Config{}, fmt.Errorf("signals.manifold grid produced non-positive cell volume")
	}

	ApplyDerivedGasParams(&config)

	if err := config.Validate(); err != nil {
		return Config{}, err
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

	gasCellSpacing := config.MinGasCellSpacing()
	const diffusionCFLSafety = 0.99
	const vonNeumannLimit = 0.5
	inverseSpacingSum := config.inverseSpacingSum()
	config.KThermal = envelopeRho * config.CV * gasCellSpacing * gasCellSpacing *
		vonNeumannLimit / (config.DeltaT * inverseSpacingSum) * diffusionCFLSafety
}

func (config Config) inverseSpacingSum() float64 {
	dx := config.DomainX / float64(config.GridX)
	dy := config.DomainY / float64(config.GridY)
	dz := config.DomainZ / float64(config.GridZ)

	if dx <= 0 || dy <= 0 || dz <= 0 {
		return math.Inf(1)
	}

	return 1.0/(dx*dx) + 1.0/(dy*dy) + 1.0/(dz*dz)
}

/*
MinGasCellSpacing returns the smallest axis cell width on the gas grid.
*/
func (config Config) MinGasCellSpacing() float64 {
	dx := config.DomainX / float64(config.GridX)
	dy := config.DomainY / float64(config.GridY)
	dz := config.DomainZ / float64(config.GridZ)
	minSpacing := dx

	if dy < minSpacing {
		minSpacing = dy
	}

	if dz < minSpacing {
		minSpacing = dz
	}

	return minSpacing
}

/*
GasCellSpacing returns the x-axis cell width used by the gas diffusion stencil.
*/
func (config Config) GasCellSpacing() float64 {
	return config.DomainX / float64(config.GridX)
}

/*
DiffusionCFL returns the multidimensional explicit diffusion number used by the gas kernel.
*/
func (config Config) DiffusionCFL() float64 {
	envelopeRho := config.GasEnvelopeRhoMin

	if envelopeRho <= 0 || config.CV <= 0 || config.DeltaT <= 0 {
		return math.Inf(1)
	}

	return config.KThermal * config.DeltaT * config.inverseSpacingSum() / (envelopeRho * config.CV)
}

/*
AdvectiveDeltaT returns the largest timestep whose multidimensional Courant
number is one for the supplied characteristic speed.
*/
func (config Config) AdvectiveDeltaT(characteristicSpeed float64) float64 {
	if characteristicSpeed <= 0 || math.IsNaN(characteristicSpeed) || math.IsInf(characteristicSpeed, 0) {
		return 0
	}

	dx := config.DomainX / float64(config.GridX)
	dy := config.DomainY / float64(config.GridY)
	dz := config.DomainZ / float64(config.GridZ)

	if dx <= 0 || dy <= 0 || dz <= 0 {
		return 0
	}

	return 1 / (characteristicSpeed * (1/dx + 1/dy + 1/dz))
}

/*
Validate rejects configs whose explicit thermal diffusion violates von Neumann stability.
*/
func (config Config) Validate() error {
	diffusionCFL := config.DiffusionCFL()
	const vonNeumannLimit = 0.5

	if diffusionCFL > vonNeumannLimit+1e-9 {
		return fmt.Errorf(
			"physics: diffusion CFL %.6g exceeds von Neumann limit %.6g (k_thermal=%.6g rho_envelope=%.6g)",
			diffusionCFL,
			vonNeumannLimit,
			config.KThermal,
			config.GasEnvelopeRhoMin,
		)
	}

	return config.validateBoundaries()
}

func (config Config) validateBoundaries() error {
	boundaries := []struct {
		name  string
		value uint32
	}{
		{"boundary_x_low", config.BoundaryXLow},
		{"boundary_x_high", config.BoundaryXHigh},
		{"boundary_y_low", config.BoundaryYLow},
		{"boundary_y_high", config.BoundaryYHigh},
		{"boundary_z_low", config.BoundaryZLow},
		{"boundary_z_high", config.BoundaryZHigh},
	}

	for _, boundary := range boundaries {
		if boundary.value > GasBoundaryReflecting {
			return fmt.Errorf("physics: %s has invalid gas boundary mode %d", boundary.name, boundary.value)
		}
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

/*
SetSnapshotPublishInterval configures UI snapshot throttling on the field.
*/
func (config *Config) SetSnapshotPublishInterval(interval time.Duration) {
	if config == nil {
		return
	}

	config.snapshotPublishInterval = interval
}
