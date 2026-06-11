package logic

import (
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
	Source     SourceType
	Symbol     string
	Price      float64
	Strength   float64
	Volume     float64
	Spread     float64
	Elapsed    float64
	Category   CategoryType
	Regime     RegimeType
	Position   PositionType
	Confidence float64
	Surprise   float64
	ObservedAt time.Time
	Market     krakenmarket.Symbol
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
