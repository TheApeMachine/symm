package hawkes

import "github.com/theapemachine/symm/nomagique/types"

/*
Excitation-scoped Frame facts, per signal/hawkes/README.md sections 10-12.
*/
var (
	SymbolExcitationIntensityBuy  = types.MustIntern("hawkes/obs/excitation_intensity_buy")
	SymbolExcitationIntensitySell = types.MustIntern("hawkes/obs/excitation_intensity_sell")
	SymbolExcitationFractionBuy   = types.MustIntern("hawkes/obs/excitation_fraction_buy")
	SymbolExcitationFractionSell  = types.MustIntern("hawkes/obs/excitation_fraction_sell")

	SymbolExcitationAmplitudeBB = types.MustIntern("hawkes/obs/excitation_amplitude_buy_from_buy")
	SymbolExcitationAmplitudeBS = types.MustIntern("hawkes/obs/excitation_amplitude_buy_from_sell")
	SymbolExcitationAmplitudeSB = types.MustIntern("hawkes/obs/excitation_amplitude_sell_from_buy")
	SymbolExcitationAmplitudeSS = types.MustIntern("hawkes/obs/excitation_amplitude_sell_from_sell")

	SymbolExcitationDecay     = types.MustIntern("hawkes/obs/excitation_decay")
	SymbolExcitationTimescale = types.MustIntern("hawkes/obs/excitation_timescale")
)

/*
Excitation reports excess intensity above the fitted background rate
(section 10) and the fitted excitation amplitudes and shared decay rate
(section 12). This implementation constrains beta_xy = beta across all four
target/source pairs — a common-decay model per section 3 — so all four
excitation_decay:* / excitation_timescale:* metrics in the README's naming
scheme are the SAME fitted decay parameter; SymbolExcitationDecay and
SymbolExcitationTimescale are published once rather than four times to avoid
presenting one constrained parameter as four independent degrees of freedom.
*/
func Excitation(input *types.Frame) {
	_, _, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return
	}

	lambdaBuy, hasBuy := input.Get(SymbolConditionalIntensityBuy)
	lambdaSell, hasSell := input.Get(SymbolConditionalIntensitySell)
	muBuy, hasMuBuy := input.Get(SymbolBackgroundRateBuy)
	muSell, hasMuSell := input.Get(SymbolBackgroundRateSell)

	if !hasBuy || !hasSell || !hasMuBuy || !hasMuSell {
		return
	}

	excessBuy := lambdaBuy - muBuy
	excessSell := lambdaSell - muSell

	input.Put(SymbolExcitationIntensityBuy, excessBuy)
	input.Put(SymbolExcitationIntensitySell, excessSell)

	if lambdaBuy > 0 {
		input.Put(SymbolExcitationFractionBuy, excessBuy/lambdaBuy)
	}

	if lambdaSell > 0 {
		input.Put(SymbolExcitationFractionSell, excessSell/lambdaSell)
	}

	input.Put(SymbolExcitationAmplitudeBB, alphaXX)
	input.Put(SymbolExcitationAmplitudeBS, alphaXY)
	input.Put(SymbolExcitationAmplitudeSB, alphaYX)
	input.Put(SymbolExcitationAmplitudeSS, alphaYY)

	if beta <= 0 {
		return
	}

	input.Put(SymbolExcitationDecay, beta)
	input.Put(SymbolExcitationTimescale, 1/beta)
}
