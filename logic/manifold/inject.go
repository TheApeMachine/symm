package manifold

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
inject deposits Hawkes excitation into the GPU gas lattice and couples buy/sell
oscillator modes before the transport step runs.
*/
func inject(
	handle *pmanifold.Solver,
	config pmanifold.Config,
	outcome excitation.Outcome,
	oscillators []pmanifold.Oscillator,
) error {
	if handle == nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: solver handle is not initialized",
			nil,
		)
	}

	buyIntensity, sellIntensity := intensities(outcome)
	totalIntensity := buyIntensity + sellIntensity

	if totalIntensity <= 0 || math.IsNaN(totalIntensity) || math.IsInf(totalIntensity, 0) {
		return errnie.Err(
			errnie.Internal,
			"manifold: non-positive Hawkes intensity",
			nil,
		)
	}

	if outcome.Fit.Beta <= 0 {
		return errnie.Err(
			errnie.Internal,
			"manifold: hawkes fit decay rate must be positive",
			nil,
		)
	}

	if len(oscillators) == 0 {
		return errnie.Err(
			errnie.Internal,
			"manifold: oscillator population is empty",
			nil,
		)
	}

	if err := handle.ResetDeposits(); err != nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: failed to reset deposits",
			err,
		)
	}

	if err := depositIntensities(
		handle, config, outcome, buyIntensity, sellIntensity, totalIntensity,
	); err != nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: failed to deposit intensities",
			err,
		)
	}

	return handle.SetOscillators(oscillators)
}

func intensities(outcome excitation.Outcome) (buyIntensity float64, sellIntensity float64) {
	if outcome.Readiness.HawkesFit {
		return outcome.Fit.IntensityX, outcome.Fit.IntensityY
	}

	return outcome.BuyArrivalRate, outcome.SellArrivalRate
}

func depositIntensities(
	handle *pmanifold.Solver,
	config pmanifold.Config,
	outcome excitation.Outcome,
	buyIntensity float64,
	sellIntensity float64,
	totalIntensity float64,
) error {
	buyCellX, sellCellX, cellY, cellZ := hawkesCells(
		config, outcome, buyIntensity, sellIntensity, totalIntensity,
	)
	buyRho := buyIntensity / totalIntensity * config.RhoMin
	sellRho := sellIntensity / totalIntensity * config.RhoMin

	if err := handle.DepositCell(
		buyCellX, cellY, cellZ,
		buyRho, 0, 0, 0, buyRho*config.CV,
	); err != nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: failed to deposit buy cell",
			err,
		)
	}

	return handle.DepositCell(
		sellCellX, cellY, cellZ,
		sellRho, 0, 0, 0, sellRho*config.CV,
	)
}

func hawkesCells(
	config pmanifold.Config,
	outcome excitation.Outcome,
	buyIntensity float64,
	sellIntensity float64,
	totalIntensity float64,
) (buyCellX uint32, sellCellX uint32, cellY uint32, cellZ uint32) {
	buyFraction := buyIntensity / totalIntensity
	sellFraction := sellIntensity / totalIntensity
	crossSum := outcome.Fit.AlphaXY + outcome.Fit.AlphaYX
	crossFraction := 0.5

	if crossSum > 0 {
		crossFraction = outcome.Fit.AlphaXY / crossSum
	}

	maxX := float64(config.GridX - 1)
	maxY := float64(config.GridY - 1)
	maxZ := float64(config.GridZ - 1)

	buyCellX = uint32(math.Round(buyFraction * maxX))
	sellCellX = uint32(math.Round(sellFraction * maxX))
	cellY = uint32(math.Round(crossFraction * maxY))
	cellZ = uint32(math.Round(outcome.Fit.SpectralRadius * maxZ))

	return buyCellX, sellCellX, cellY, cellZ
}

func buildOscillators(
	config pmanifold.Config,
	outcome excitation.Outcome,
	buyIntensity float64,
	sellIntensity float64,
	totalIntensity float64,
) []pmanifold.Oscillator {
	buyCellX, sellCellX, cellY, cellZ := hawkesCells(
		config, outcome, buyIntensity, sellIntensity, totalIntensity,
	)

	buyPosX, buyPosY, buyPosZ := cellCenter(config, buyCellX, cellY, cellZ)
	sellPosX, sellPosY, sellPosZ := cellCenter(config, sellCellX, cellY, cellZ)
	omega := outcome.Fit.Beta
	coupling := config.CouplingScale()
	flow := (buyIntensity - sellIntensity) / totalIntensity * coupling
	buyAmplitude := buyIntensity / totalIntensity
	sellAmplitude := sellIntensity / totalIntensity
	buyHeat := branchingHeat(outcome.Fit.AlphaXX, omega)
	sellHeat := branchingHeat(outcome.Fit.AlphaYY, omega)
	phaseScale := 2 * math.Pi * outcome.Maturity

	return []pmanifold.Oscillator{
		{
			Phase:     phaseScale,
			Omega:     omega,
			Amplitude: buyAmplitude,
			PosX:      buyPosX,
			PosY:      buyPosY,
			PosZ:      buyPosZ,
			Heat:      buyHeat,
			VelX:      flow,
		},
		{
			Phase:     phaseScale + math.Pi,
			Omega:     omega,
			Amplitude: sellAmplitude,
			PosX:      sellPosX,
			PosY:      sellPosY,
			PosZ:      sellPosZ,
			Heat:      sellHeat,
			VelX:      -flow,
		},
	}
}

func branchingHeat(alpha float64, omega float64) float64 {
	if omega <= 0 {
		return 0
	}

	return alpha / omega
}

func cellCenter(
	config pmanifold.Config,
	cellX uint32,
	cellY uint32,
	cellZ uint32,
) (float64, float64, float64) {
	return (float64(cellX) + 0.5) * config.DomainX / float64(config.GridX),
		(float64(cellY) + 0.5) * config.DomainY / float64(config.GridY),
		(float64(cellZ) + 0.5) * config.DomainZ / float64(config.GridZ)
}

func stressAnisotropy(outcome excitation.Outcome) float64 {
	selfSum := outcome.Fit.AlphaXX + outcome.Fit.AlphaYY

	if selfSum <= 0 {
		return 0
	}

	return math.Abs(outcome.Fit.AlphaXX-outcome.Fit.AlphaYY) / selfSum
}

func integrationDeltaT(config pmanifold.Config, outcome excitation.Outcome) float64 {
	if outcome.Fit.Beta > 0 {
		advective := config.AdvectiveDeltaT(outcome.Fit.Beta)

		if advective > 0 && advective < config.DeltaT {
			return advective
		}
	}

	if outcome.Horizon > 0 {
		horizonStep := outcome.Horizon.Seconds()

		if horizonStep > 0 && horizonStep < config.DeltaT {
			return horizonStep
		}
	}

	return config.DeltaT
}
