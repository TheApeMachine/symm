package engine

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

/*
Snapshot holds the latest broadcast market values for one symbol.
*/
type Snapshot struct {
	Last         float64
	LastAt       time.Time
	LastOK       bool
	VolumeBase   float64
	VolumeOK     bool
	BatchVolume  float64
	TradesAt     time.Time
	BatchOK      bool
	BuyPressure  float64
	PressureOK   bool
	SpreadBPS    float64
	BookAt       time.Time
	SpreadOK     bool
	Imbalance    float64
	ImbalanceOK  bool
	Density      float64
	DensityOK    bool
	ChangePct    float64
	ChangeOK     bool
	BidLevels    []market.BookLevel
	AskLevels    []market.BookLevel
	DepthOK      bool
	DepthSlope   float64
	DepthSlopeOK bool
}
