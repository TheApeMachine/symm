package logic

import (
	"errors"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

type SourceType string

const (
	SourceNone        SourceType = ""
	SourceFluid       SourceType = "fluid"
	SourceHawkes      SourceType = "hawkes"
	SourcePumpDump    SourceType = "pumpdump"
	SourceDepthFlow   SourceType = "depthflow"
	SourceSentiment   SourceType = "sentiment"
	SourceCorrelation SourceType = "correlation"
	SourceCausal      SourceType = "causal"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourceExhaustion  SourceType = "exhaustion"
	SourcePrediction  SourceType = "prediction"
	SourceCVD         SourceType = "cvd"
	SourceToxicity    SourceType = "toxicity"
	SourceManifold    SourceType = "manifold"
)

type Measurement struct {
	Source     SourceType          `yaml:"source" json:"source"`
	Symbol     string              `yaml:"symbol" json:"symbol"`
	Price      float64             `yaml:"price" json:"price"`
	Strength   float64             `yaml:"strength" json:"strength"`
	Volume     float64             `yaml:"volume" json:"volume"`
	Spread     float64             `yaml:"spread" json:"spread"`
	Elapsed    float64             `yaml:"elapsed" json:"elapsed"`
	Category   CategoryType        `yaml:"category" json:"category"`
	Regime     RegimeType          `yaml:"regime" json:"regime"`
	Position   PositionType        `yaml:"position" json:"position"`
	Confidence float64             `yaml:"confidence" json:"confidence"`
	Surprise   float64             `yaml:"surprise" json:"surprise"`
	ObservedAt time.Time           `yaml:"observed_at" json:"observed_at"`
	Market     krakenmarket.Symbol `yaml:"market" json:"market"`
	BestEffort bool                `yaml:"best_effort,omitempty" json:"best_effort,omitempty"`
	GapReason  string              `yaml:"gap_reason,omitempty" json:"gap_reason,omitempty"`
}

/*
Publish sends a complete measurement to the measurements bus.
*/
func (measurement Measurement) Publish(bus *internal.Bus) error {
	if err := errnie.Error(errnie.Require(map[string]any{
		"source":      measurement.Source,
		"symbol":      measurement.Symbol,
		"observed_at": measurement.ObservedAt,
		"price":       measurement.Price,
		"strength":    measurement.Strength,
		"volume":      measurement.Volume,
		"spread":      measurement.Spread,
		"elapsed":     measurement.Elapsed,
		"confidence":  measurement.Confidence,
		"surprise":    measurement.Surprise,
	})); err != nil {
		return err
	}

	if err := measurement.Market.Validate(); err != nil {
		return errnie.Error(err)
	}

	return bus.Send(internal.ChannelMeasurements, "measurements", measurement)
}

func NewMeasurement(
	source SourceType,
	symbol string,
	price float64,
	strength float64,
	volume float64,
	spread float64,
	elapsed float64,
	category CategoryType,
	regime RegimeType,
	position PositionType,
	confidence float64,
	surprise float64,
) Measurement {
	return Measurement{
		Source:     source,
		Symbol:     symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    elapsed,
		Category:   category,
		Regime:     regime,
		Position:   position,
		Confidence: confidence,
		Surprise:   surprise,
	}
}

/*
PeakConfidence returns the highest confidence across a measurement spectrum.
*/
func PeakConfidence(measurements []Measurement) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if measurement.Confidence > peak {
			peak = measurement.Confidence
		}
	}

	return peak
}

/*
ReferencePrice returns the latest positive price from a measurement spectrum.
*/
func ReferencePrice(measurements []Measurement) (float64, error) {
	for index := len(measurements) - 1; index >= 0; index-- {
		if measurements[index].Price > 0 {
			return measurements[index].Price, nil
		}
	}

	return 0, errnie.Error(errors.New("logic: reference price is required for sizing"))
}

/*
SymbolFromMeasurements returns the symbol shared by a complete spectrum.
*/
func SymbolFromMeasurements(measurements []Measurement) (string, error) {
	for _, measurement := range measurements {
		if measurement.Symbol != "" {
			return measurement.Symbol, nil
		}
	}

	return "", errnie.Error(errors.New("logic: symbol is required for actions"))
}
