package trader

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
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
		return NewWalletCapitalProvider(), nil
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: unsupported trading model",
			errnie.Require(map[string]any{
				"model": tradingConfig.Model,
			}),
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
EntrySlotFraction sizes a new entry from the best coherent candidate, remaining slot
capacity pressure, and regime turbulence instead of independent spectrum peaks.
*/
func EntrySlotFraction(
	holdings *logic.Holdings,
	occupancy logic.EntrySlotOccupancy,
	measurements []logic.Measurement,
	thresholdCtx logic.ThresholdContext,
	tradingConfig config.TradingConfig,
	opportunitySlot bool,
) (float64, error) {
	if holdings == nil {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"holdings": holdings,
		}))
	}

	costBps := logic.ExecutionCostFromMeasurements(measurements, 0, 0, 0)
	candidate, ok := logic.BestEntryCandidate(measurements, costBps)

	if !ok || candidate.Confidence <= 0 || candidate.Strength <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"candidate": candidate,
		}))
	}

	totalCapacity := tradingConfig.MaxConcurrentPositions + tradingConfig.OpportunitySlotCount

	if totalCapacity <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"total_capacity": totalCapacity,
		}))
	}

	remainingSlots := totalCapacity - occupancy.CommittedCount()

	if remainingSlots <= 0 {
		return 0, errnie.Error(errnie.Require(map[string]any{
			"remaining_slots": remainingSlots,
		}))
	}

	slotEnvelope := 1.0 / float64(totalCapacity)
	capacityPressure := 1.0 - float64(remainingSlots-1)/float64(totalCapacity)
	capacityPressure = clampUnit(capacityPressure, 0, 1)

	confidenceAnchor := thresholdCtx.EntryConfidenceBaseline
	surpriseAnchor := logic.SurpriseAnchorForCandidate(candidate, thresholdCtx)
	strengthAnchor := math.Max(candidate.Strength, 1e-9)

	confidenceWeight := candidate.Confidence / math.Max(confidenceAnchor, candidate.Confidence)
	surpriseWeight := 1.0

	if candidate.Novelty > 0 && surpriseAnchor > 0 {
		surpriseWeight = candidate.Novelty / surpriseAnchor
	}

	strengthWeight := candidate.Strength / strengthAnchor
	noiseDampening := 1.0 / (1.0 + math.Abs(candidate.Confidence-confidenceAnchor))
	riskDampening := 1.0

	if candidate.Toxicity > 0 {
		riskDampening = candidate.Strength / (candidate.Strength + candidate.Toxicity)
	}

	tierScale := confidenceTierScale(
		holdings,
		candidate.Confidence,
		tradingConfig.MaxConcurrentPositions,
		opportunitySlot,
	)

	score := logicCandidateScore(candidate)
	hurdle := 0.35 + capacityPressure*0.25
	fraction := slotEnvelope * sigmoid(score-hurdle) * (1.0 - 0.5*capacityPressure)
	fraction *= confidenceWeight * surpriseWeight * strengthWeight * noiseDampening * riskDampening * tierScale

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

func logicCandidateScore(candidate logic.EntryCandidate) float64 {
	costRatio := 1.0

	if candidate.CostBps > 0 {
		costRatio = math.Max(0, candidate.EdgeBps/candidate.CostBps)
	}

	confidence := clampUnit(candidate.Confidence, 0.01, 0.99)
	strengthAnchor := math.Max(candidate.Strength, 1e-9)

	return 0.45*math.Log1p(costRatio) +
		0.30*logit(confidence) +
		0.20*math.Log1p(candidate.Strength/strengthAnchor) +
		0.05*math.Log1p(candidate.Novelty) -
		0.35*candidate.Toxicity
}

func sigmoid(value float64) float64 {
	return 1 / (1 + math.Exp(-value))
}

func clampUnit(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}

	if value > upper {
		return upper
	}

	return value
}

func logit(probability float64) float64 {
	return math.Log(probability / (1 - probability))
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

		novelty := measurement.NoveltySurprise

		if novelty <= 0 {
			novelty = measurement.Surprise
		}

		if novelty > 0 {
			scalars.surprises = append(scalars.surprises, novelty)
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
