package trader

import (
	"context"
	"errors"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
)

/*
CapitalProvider supplies quote capital for order sizing.
*/
type CapitalProvider interface {
	QuoteBalance(ctx context.Context, quote string) (float64, error)
	AvailableQuoteBalance(ctx context.Context, quote string) (float64, error)
}

/*
StaticCapitalProvider uses a fixed quote balance for deterministic paper sizing.
Paper trading assumes a single configured quote currency, so QuoteBalance and
AvailableQuoteBalance intentionally ignore the quote parameter.
*/
type StaticCapitalProvider struct {
	quoteBalance float64
}

func NewStaticCapitalProvider(quoteBalance float64) (*StaticCapitalProvider, error) {
	if quoteBalance <= 0 {
		return nil, errnie.Error(errors.New("trader: quote balance must be positive"))
	}

	return &StaticCapitalProvider{quoteBalance: quoteBalance}, nil
}

func NewCapitalProvider(
	tradingConfig config.TradingConfig,
) (CapitalProvider, error) {
	switch tradingConfig.Model {
	case "paper":
		quoteBalance, err := config.PaperWalletBalance()

		if err != nil {
			return nil, errnie.Error(err)
		}

		return NewStaticCapitalProvider(quoteBalance)
	case "live":
		return nil, errnie.Error(errors.New(
			"trader: live capital provider not configured",
		))
	default:
		return nil, errnie.Error(fmt.Errorf(
			"trader: unsupported trading model %q",
			tradingConfig.Model,
		))
	}
}

func (provider *StaticCapitalProvider) QuoteBalance(
	context.Context,
	string,
) (float64, error) {
	return provider.available()
}

func (provider *StaticCapitalProvider) AvailableQuoteBalance(
	context.Context,
	string,
) (float64, error) {
	return provider.available()
}

func (provider *StaticCapitalProvider) available() (float64, error) {
	if provider == nil || provider.quoteBalance <= 0 {
		return 0, errnie.Error(errors.New("trader: quote balance must be positive"))
	}

	return provider.quoteBalance, nil
}

/*
EntrySlotFraction sizes a new entry from the live measurement spectrum, open-book
rank, remaining slot capacity, and regime turbulence.
*/
func EntrySlotFraction(
	holdings *logic.Holdings,
	measurements []logic.Measurement,
	thresholdConfig config.ThresholdConfig,
	tradingConfig config.TradingConfig,
	regimeVolatility float64,
	opportunitySlot bool,
) (float64, error) {
	if holdings == nil {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"holdings": holdings,
		}))
	}

	confidence := logic.PeakConfidence(measurements)
	surprise := logic.PeakSurprise(measurements)
	strength := logic.PeakStrength(measurements)

	if confidence <= 0 || strength <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"confidence": confidence,
			"strength":   strength,
		}))
	}

	spectrum, spectrumErr := measurementSpectrum(measurements)

	if spectrumErr != nil {
		return 0, errnie.Error(spectrumErr)
	}

	confidenceAnchor := spectrumAnchor(
		spectrum.confidences,
		thresholdConfig.EntryConfidenceBaseline+
			thresholdConfig.TurbulenceConfidenceScale*regimeVolatility,
	)
	surpriseAnchor := spectrumAnchor(
		spectrum.surprises,
		thresholdConfig.EntrySurpriseBaseline,
	)
	strengthAnchor := spectrumMedian(spectrum.strengths)

	if confidenceAnchor <= 0 || strengthAnchor <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"confidence_anchor": confidenceAnchor,
			"strength_anchor":   strengthAnchor,
		}))
	}

	totalCapacity := tradingConfig.MaxConcurrentPositions + tradingConfig.OpportunitySlotCount

	if totalCapacity <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"total_capacity": totalCapacity,
		}))
	}

	remainingSlots := totalCapacity - holdings.OpenCount()

	if remainingSlots <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"remaining_slots": remainingSlots,
		}))
	}

	slotEnvelope := 1.0 / float64(totalCapacity)
	confidenceWeight := confidence / confidenceAnchor
	surpriseWeight := surprise / surpriseAnchor

	if surpriseAnchor > 0 && surprise <= 0 {
		surpriseWeight = 0
	}

	strengthWeight := strength / strengthAnchor
	noiseDampening := spectrumClarity(
		spectrum.confidences,
		confidenceAnchor,
	)
	riskDampening := toxicityDampening(
		spectrum.toxicities,
		strength,
	)
	tierScale := confidenceTierScale(
		holdings,
		confidence,
		tradingConfig.MaxConcurrentPositions,
		opportunitySlot,
	)

	fraction := slotEnvelope *
		confidenceWeight *
		surpriseWeight *
		strengthWeight *
		noiseDampening *
		riskDampening *
		tierScale

	if fraction <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"fraction": fraction,
		}))
	}

	if fraction > 1 {
		fraction = 1
	}

	return fraction, nil
}

type measurementScalars struct {
	confidences []float64
	surprises   []float64
	strengths   []float64
	toxicities  []float64
}

func measurementSpectrum(
	measurements []logic.Measurement,
) (measurementScalars, error) {
	scalars := measurementScalars{
		confidences: make([]float64, 0, len(measurements)),
		surprises:   make([]float64, 0, len(measurements)),
		strengths:   make([]float64, 0, len(measurements)),
		toxicities:  make([]float64, 0, len(measurements)),
	}

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 || measurement.Strength <= 0 {
			continue
		}

		scalars.confidences = append(scalars.confidences, measurement.Confidence)
		scalars.strengths = append(scalars.strengths, measurement.Strength)

		if measurement.Surprise > 0 {
			scalars.surprises = append(scalars.surprises, measurement.Surprise)
		}

		if measurement.Source == logic.SourceToxicity {
			scalars.toxicities = append(scalars.toxicities, measurement.Strength)
		}
	}

	if len(scalars.confidences) == 0 || len(scalars.strengths) == 0 {
		return measurementScalars{}, errnie.Require(map[string]any{
			"measurements": measurements,
		})
	}

	return scalars, nil
}

func spectrumMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	return float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(values...)...))
}

func spectrumAnchor(values []float64, baseline float64) float64 {
	median := spectrumMedian(values)

	if median > baseline {
		return median
	}

	return baseline
}

func spectrumClarity(values []float64, anchor float64) float64 {
	if len(values) == 0 || anchor <= 0 {
		return 0
	}

	noise := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(values...)...))

	return anchor / (anchor + noise)
}

func toxicityDampening(toxicities []float64, entryStrength float64) float64 {
	if len(toxicities) == 0 || entryStrength <= 0 {
		return 1
	}

	peakToxicity := float64(statistic.NewMax().Observe(nomagique.Numbers(toxicities...)...))
	medianToxicity := spectrumMedian(toxicities)
	toxicityNoise := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(toxicities...)...))
	toxicityCeiling := medianToxicity + toxicityNoise

	if peakToxicity <= toxicityCeiling {
		return 1
	}

	return entryStrength / (entryStrength + peakToxicity)
}

func confidenceTierScale(
	holdings *logic.Holdings,
	entryConfidence float64,
	primaryCapacity int,
	opportunitySlot bool,
) float64 {
	if opportunitySlot {
		return 1
	}

	if holdings.StrictlyHigherConfidenceCount(entryConfidence) < primaryCapacity {
		return 1
	}

	peakOpenConfidence := holdings.PeakOpenConfidence()

	if peakOpenConfidence <= 0 {
		return 1
	}

	return entryConfidence / peakOpenConfidence
}

/*
OrderQuantityFromFraction converts quote wallet notional into base quantity.
*/
func OrderQuantityFromFraction(
	walletQuote float64,
	fraction float64,
	price float64,
) (float64, error) {
	if walletQuote <= 0 {
		return 0, errnie.Error(errors.New("trader: wallet quote balance must be positive"))
	}

	if fraction <= 0 {
		return 0, errnie.Error(errors.New("trader: position fraction must be positive"))
	}

	if price <= 0 {
		return 0, errnie.Error(errors.New("trader: reference price must be positive"))
	}

	notional := walletQuote * fraction
	quantity := notional / price

	if quantity <= 0 {
		return 0, errnie.Error(fmt.Errorf(
			"trader: computed quantity must be positive (wallet=%.4f fraction=%.4f price=%.4f)",
			walletQuote,
			fraction,
			price,
		))
	}

	return quantity, nil
}

/*
QuoteWalletBalance returns the configured paper quote balance for sizing.
*/
func QuoteWalletBalance(model string) (float64, error) {
	if model != "paper" {
		return 0, errnie.Error(fmt.Errorf(
			"trader: wallet sizing is only wired for paper model, got %q",
			model,
		))
	}

	return config.PaperWalletBalance()
}
