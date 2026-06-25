package manifold

import (
	"fmt"
	"math"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func (field *Field) whalesFromSolverReadback(
	solverCarriers []fieldCarrier,
	readOscillators []mkernel.Oscillator,
) []whaleCarrier {
	whales := make([]whaleCarrier, 0)

	for index, carrier := range solverCarriers {
		if carrier.role != "whale" || index >= len(readOscillators) {
			continue
		}

		oscillator := readOscillators[index]

		if !oscillatorStateFinite(oscillator) {
			continue
		}

		whales = append(whales, whaleCarrier{
			symbol:     carrier.symbol,
			oscillator: oscillator,
		})
	}

	return whales
}

func displayOscillator(fallback, read mkernel.Oscillator) mkernel.Oscillator {
	if !oscillatorFullyFinite(read) {
		return fallback
	}

	merged := read
	merged.PosX = fallback.PosX
	merged.PosY = fallback.PosY
	merged.PosZ = fallback.PosZ

	return merged
}

func oscillatorStateFinite(oscillator mkernel.Oscillator) bool {
	return oscillatorFullyFinite(oscillator)
}

func oscillatorFullyFinite(oscillator mkernel.Oscillator) bool {
	values := []float64{
		oscillator.PosX,
		oscillator.PosY,
		oscillator.PosZ,
		oscillator.VelX,
		oscillator.VelY,
		oscillator.VelZ,
		oscillator.Phase,
		oscillator.Omega,
		oscillator.Amplitude,
		oscillator.Heat,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func (field *Field) liquidityRho(state *UniverseState, qty float64, activeCarriers int) (float64, error) {
	if qty <= 0 {
		return 0, fmt.Errorf("manifold: qty must be positive for %q", state.symbol)
	}

	if activeCarriers <= 0 {
		return 0, fmt.Errorf("manifold: active carrier count must be positive")
	}

	reference := visibleBookQty(state)

	tradeQtys := state.GetTradeQtys()
	if reference <= 0 && len(tradeQtys) > 0 {
		reference = median(tradeQtys) * float64(len(tradeQtys))
	}

	if reference <= 0 {
		return 0, fmt.Errorf("manifold: liquidity reference unavailable for %q", state.symbol)
	}

	carrierCapacity := activeCarriers

	if uint32(activeCarriers) < field.config.MaxModes {
		carrierCapacity = int(field.config.MaxModes)
	}

	return (qty / reference) * field.config.RhoMin / float64(carrierCapacity), nil
}

func (field *Field) displayCarriers(
	symbolCarriers []fieldCarrier,
	solverCarriers []fieldCarrier,
	readOscillators []mkernel.Oscillator,
) []fieldCarrier {
	solverSymbols := make(map[string]mkernel.Oscillator, len(solverCarriers))
	whaleDisplay := make([]fieldCarrier, 0)

	for index, carrier := range solverCarriers {
		if index >= len(readOscillators) {
			break
		}

		readOscillator := readOscillators[index]

		if carrier.role == "whale" {
			if !oscillatorStateFinite(readOscillator) {
				continue
			}

			whaleDisplay = append(whaleDisplay, fieldCarrier{
				role:       "whale",
				symbol:     carrier.symbol,
				oscillator: readOscillator,
			})

			continue
		}

		solverSymbols[carrier.symbol] = readOscillator
	}

	display := make([]fieldCarrier, 0, len(symbolCarriers)+len(whaleDisplay))

	for _, carrier := range symbolCarriers {
		oscillator, inSolver := solverSymbols[carrier.symbol]

		if !inSolver {
			display = append(display, carrier)
			continue
		}

		display = append(display, fieldCarrier{
			role:       "symbol",
			symbol:     carrier.symbol,
			oscillator: displayOscillator(carrier.oscillator, oscillator),
		})
	}

	display = append(display, whaleDisplay...)

	return display
}

func (field *Field) whaleOscillatorFromTrade(
	state *UniverseState,
	trade *TradeUpdate,
	coords Coords,
	rho float64,
) mkernel.Oscillator {
	omega := returnFrequency(state.GetReturns(), field.config.DeltaT)
	energy := math.Max(rho, field.config.RhoMin)
	speed := math.Sqrt(energy)
	phase := 0.0

	if trade.Side == "sell" {
		phase = math.Pi
	}

	return mkernel.Oscillator{
		Phase:     phase,
		Omega:     omega,
		Amplitude: speed,
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      rho,
		VelX:      tradeSideSign(trade.Side) * speed,
	}
}

func (field *Field) oscillatorFromState(state *UniverseState) mkernel.Oscillator {
	returns := state.GetReturns()
	energy := medianAbsolute(returns)
	omega := returnFrequency(returns, field.config.DeltaT)
	coords := field.universe.coords(state, 0)

	return mkernel.Oscillator{
		Phase:     returnAnalyticPhase(returns),
		Omega:     omega,
		Amplitude: math.Sqrt(math.Max(energy, field.config.RhoMin)),
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      energy,
	}
}
