package grid

import "time"

// FormatVersion identifies the coordinate, region and volume-clock replay contract.
const FormatVersion = 1

// Impulse is a borrowed region sequence, valid until its producer steps again.
// SeqIdx identifies the recorded input boundary; time fields are display facts.
type Impulse struct {
	Label    string
	SeqIdx   int64
	At, From time.Time
	Version  uint64
	Ready    bool
	Regions  []Region
}

// Region is the current activation of one geometric watershed basin.
type Region struct {
	Condition uint64  `json:"condition"`
	Level     float64 `json:"level"`
	Change    float64 `json:"change"`
	ID        uint64  `json:"id"`
	Strength  float64 `json:"strength"`
	Authority float64 `json:"authority"`
	Members   int     `json:"members"`
}

// Snapshot is an immutable visualization captured at a publication boundary.
type Snapshot struct {
	Label    string
	Sequence int64
	Volume   string
	Cells    []Quantity
	Regions  []Region
}

type Quantity struct {
	ID                             uint64
	Source, Label                  string
	X, Y, Value, Activity, Quality float64
	Present                        bool
}
