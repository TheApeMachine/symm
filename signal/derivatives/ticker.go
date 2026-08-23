package derivatives

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) consumeTicker(
	symbol *types.Symbol,
	ticker kraken.FuturesTickerData,
	data *DerivativesData,
) error {
	if ticker.OpenInterest > 0 {
		data.OI = ticker.OpenInterest

		oiOutput, err := signal.oi.Step(
			symbol.Symbol,
			sampleFrame(ticker.Timestamp, ticker.OpenInterest),
		)

		if err != nil {
			return err
		}

		if oiVelocity, hasVelocity := oiOutput.Get(equation.SymbolRelativeChange); hasVelocity {
			data.OIVelocity = oiVelocity

			accelOutput, err := signal.oiAcceleration.Step(
				symbol.Symbol,
				sampleFrame(ticker.Timestamp, oiVelocity),
			)

			if err != nil {
				return err
			}

			if oiAccel, hasAccel := accelOutput.Get(SymbolOIAcceleration); hasAccel {
				data.OIAcceleration = oiAccel
			}
		}
	}

	if ticker.Last == nil || ticker.Last.Float64() <= 0 {
		return nil
	}

	perpPrice := ticker.Last.Float64()
	spotPrice := perpPrice

	if ticker.IndexPrice != nil && ticker.IndexPrice.Float64() > 0 {
		spotPrice = ticker.IndexPrice.Float64()
	}

	if spotPrice > 0 {
		basisInput := eventFrame(ticker.Timestamp)
		basisInput.Put(nmtypes.AlphaPrice, perpPrice)
		basisInput.Put(nmtypes.BetaPrice, spotPrice)

		basisOutput, err := signal.basis.Step(symbol.Symbol, basisInput)

		if err != nil {
			return err
		}

		data.Basis, _ = basisOutput.Get(SymbolBasis)

		bvOutput, err := signal.basisVelocity.Step(
			symbol.Symbol,
			sampleFrame(ticker.Timestamp, data.Basis),
		)

		if err != nil {
			return err
		}

		if bv, hasBV := bvOutput.Get(SymbolBasisVelocity); hasBV {
			data.BasisVelocity = bv
		}

		if ticker.IndexPrice != nil && ticker.IndexPrice.Float64() > 0 {
			idxInput := eventFrame(ticker.Timestamp)
			idxInput.Put(nmtypes.AlphaPrice, ticker.IndexPrice.Float64())
			idxInput.Put(nmtypes.BetaPrice, spotPrice)

			idxOutput, err := signal.indexBasis.Step(symbol.Symbol, idxInput)

			if err != nil {
				return err
			}

			data.IndexBasis, _ = idxOutput.Get(SymbolIndexBasis)
			data.TripartiteDivergence = data.Basis - data.IndexBasis
		}
	}

	priceOutput, err := signal.tickerPrice.Step(
		symbol.Symbol,
		sampleFrame(ticker.Timestamp, perpPrice),
	)

	if err != nil {
		return err
	}

	priceVelocity, _ := priceOutput.Get(equation.SymbolRelativeChange)
	data.SampleCount, _ = priceOutput.Get(nomagique.SampleCount)

	ign, sqz, build, delev, decoup := evaluateRegimes(
		priceVelocity,
		data.OIVelocity,
		data.AggressorImbalance,
		0,
		data.LiquidationBuy,
		data.LiquidationSell,
		data.LiquidationIntensity,
		data.Basis,
		data.TripartiteDivergence,
	)

	data.LeveragedIgnition = ign
	data.ShortSqueeze = sqz
	data.AdverseLeverageBuildup = build
	data.LongDeleveraging = delev
	data.DerivativesDecoupling = decoup

	return nil
}
