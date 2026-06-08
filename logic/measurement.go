package logic

type SourceType uint8

const (
	SourceNone SourceType = iota
	SourceFluid
	SourceHawkes
	SourcePumpDump
	SourceDepthFlow
	SourceSentiment
	SourceCorrelation
	SourceCausal
	SourceLeadLag
	SourceLiquidity
	SourceExhaustion
	SourcePrediction
	SourceCVD
	SourceToxicity
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
