package logic

import (
	"fmt"
	"math"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/internal"
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
Warmup stubs must not be published on the measurements bus.
*/
func (measurement Measurement) Publishable() bool {
	if measurement.Source == "" || measurement.Symbol == "" {
		return false
	}

	return !measurement.ObservedAt.IsZero()
}

/*
Publish sends a complete measurement to the measurements bus.
Non-finite floats are rejected because they indicate a broken signal calculation.
*/
func (measurement Measurement) Publish(bus *internal.Bus) error {
	if !measurement.Publishable() {
		return nil
	}

	if err := measurement.requireFinite(); err != nil {
		return err
	}

	if err := measurement.Market.Validate(); err != nil {
		return fmt.Errorf(
			"logic: measurement %s/%s: %w",
			measurement.Source,
			measurement.Symbol,
			err,
		)
	}

	return bus.Send(internal.ChannelMeasurements, "measurements", measurement)
}

func (measurement Measurement) requireFinite() error {
	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Price", measurement.Price,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Strength", measurement.Strength,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Volume", measurement.Volume,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Spread", measurement.Spread,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Elapsed", measurement.Elapsed,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Confidence", measurement.Confidence,
	); err != nil {
		return err
	}

	if err := requireFiniteField(
		measurement.Source, measurement.Symbol, "Surprise", measurement.Surprise,
	); err != nil {
		return err
	}

	return nil
}

func requireFiniteField(
	source SourceType,
	symbol string,
	fieldName string,
	value float64,
) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf(
			"logic: measurement %s/%s has non-finite %s",
			source,
			symbol,
			fieldName,
		)
	}

	return nil
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
