package logic

import (
	"time"
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
	Source          SourceType   `yaml:"source" json:"source"`
	Symbol          string       `yaml:"symbol" json:"symbol"`
	Price           float64      `yaml:"price" json:"price"`
	Strength        float64      `yaml:"strength" json:"strength"`
	Volume          float64      `yaml:"volume" json:"volume"`
	Spread          float64      `yaml:"spread" json:"spread"`
	Elapsed         float64      `yaml:"elapsed" json:"elapsed"`
	Category        CategoryType `yaml:"category" json:"category"`
	Regime          RegimeType   `yaml:"regime" json:"regime"`
	Position        PositionType `yaml:"position" json:"position"`
	Confidence      float64      `yaml:"confidence" json:"confidence"`
	Surprise        float64      `yaml:"surprise" json:"surprise"`
	EdgeConfidence  float64      `yaml:"edge_confidence,omitempty" json:"edge_confidence,omitempty"`
	NoveltySurprise float64      `yaml:"novelty_surprise,omitempty" json:"novelty_surprise,omitempty"`
	EdgeSurprise    float64      `yaml:"edge_surprise,omitempty" json:"edge_surprise,omitempty"`
	ExpectedMoveBps float64      `yaml:"expected_move_bps,omitempty" json:"expected_move_bps,omitempty"`
	ObservedAt      time.Time    `yaml:"observed_at" json:"observed_at"`
	Market          string       `yaml:"market" json:"market"`
	BestEffort      bool         `yaml:"best_effort,omitempty" json:"best_effort,omitempty"`
	GapReason       string       `yaml:"gap_reason,omitempty" json:"gap_reason,omitempty"`
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
