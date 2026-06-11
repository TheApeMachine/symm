package logic

import (
	"fmt"
	"strings"
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
	Source     SourceType          `yaml:"source"`
	Symbol     string              `yaml:"symbol"`
	Price      float64             `yaml:"price"`
	Strength   float64             `yaml:"strength"`
	Volume     float64             `yaml:"volume"`
	Spread     float64             `yaml:"spread"`
	Elapsed    float64             `yaml:"elapsed"`
	Category   CategoryType        `yaml:"category"`
	Regime     RegimeType          `yaml:"regime"`
	Position   PositionType        `yaml:"position"`
	Confidence float64             `yaml:"confidence"`
	Surprise   float64             `yaml:"surprise"`
	ObservedAt time.Time           `yaml:"observed_at"`
	Market     krakenmarket.Symbol `yaml:"market"`
	BestEffort bool                `yaml:"best_effort,omitempty"`
	GapReason  string              `yaml:"gap_reason,omitempty"`
}

/*
Publishable reports whether a measurement is complete enough for downstream consumers.
*/
func (measurement Measurement) Publishable() bool {
	if measurement.Source == "" || measurement.Symbol == "" {
		return false
	}

	if measurement.ObservedAt.IsZero() {
		return false
	}

	if measurement.Price <= 0 ||
		measurement.Strength <= 0 ||
		measurement.Volume <= 0 ||
		measurement.Spread <= 0 ||
		measurement.Elapsed <= 0 ||
		measurement.Confidence <= 0 ||
		measurement.Surprise <= 0 {
		return false
	}

	return measurement.Market.Validate() == nil
}

/*
PublishGap explains why a classifier candidate was not publishable.
*/
func (measurement Measurement) PublishGap() string {
	var gaps []string

	if measurement.Source == "" {
		gaps = append(gaps, "missing source")
	}

	if measurement.Symbol == "" {
		gaps = append(gaps, "missing symbol")
	}

	if measurement.ObservedAt.IsZero() {
		gaps = append(gaps, "missing observed_at")
	}

	if measurement.Price <= 0 {
		gaps = append(gaps, "non-positive price")
	}

	if measurement.Strength <= 0 {
		gaps = append(gaps, "non-positive strength")
	}

	if measurement.Volume <= 0 {
		gaps = append(gaps, "non-positive volume")
	}

	if measurement.Spread <= 0 {
		gaps = append(gaps, "non-positive spread")
	}

	if measurement.Elapsed <= 0 {
		gaps = append(gaps, "non-positive elapsed")
	}

	if measurement.Confidence <= 0 {
		gaps = append(gaps, "non-positive confidence")
	}

	if measurement.Surprise <= 0 {
		gaps = append(gaps, "non-positive surprise")
	}

	if err := measurement.Market.Validate(); err != nil {
		gaps = append(gaps, fmt.Sprintf("invalid market: %v", err))
	}

	if len(gaps) == 0 {
		return "candidate not publishable"
	}

	return strings.Join(gaps, ", ")
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
) *Measurement {
	return &Measurement{
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
