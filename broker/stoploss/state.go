package stoploss

import (
	"container/ring"
	"encoding/json"
	"math"
)

const pathCap = 4096

/*
Path is a bounded mark series owned by a stop. Structural and Learning
each keep their own.
*/
type Path struct {
	ring *ring.Ring
	n    int
}

/*
Mark appends one finite sample; once full, the oldest drops.
*/
func (path *Path) Mark(value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	if path.ring == nil {
		path.ring = ring.New(pathCap)
	}

	path.ring.Value = value
	path.ring = path.ring.Next()

	if path.n < pathCap {
		path.n++
	}
}

func (path Path) IsZero() bool {
	return path.n == 0
}

func (path Path) MarshalJSON() ([]byte, error) {
	if path.n == 0 {
		return []byte("null"), nil
	}

	out := make([]float64, 0, path.n)
	r := path.ring.Move(-path.n)

	for i := 0; i < path.n; i++ {
		if v, ok := r.Value.(float64); ok {
			out = append(out, v)
		}

		r = r.Next()
	}

	return json.Marshal(out)
}

func (path *Path) UnmarshalJSON(raw []byte) error {
	if path == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var samples []float64

	if err := json.Unmarshal(raw, &samples); err != nil {
		return err
	}

	path.ring = nil
	path.n = 0

	for _, sample := range samples {
		path.Mark(sample)
	}

	return nil
}

/*
State is the live proximity of the two stop checks, for the dashboard.
Each ratio is current-over-threshold: ≥1 means firing. Damage is how much of
the invalidation budget the lot has already burned on its worst print.
*/
type State struct {
	Invalidation         float64 `json:"invalidation"`
	Trailing             float64 `json:"trailing"`
	Damage               float64 `json:"damage"`
	PeakPct              float64 `json:"peakPct"`
	CurrentPct           float64 `json:"currentPct"`
	EntryPct             float64 `json:"entryPct"`
	GivebackPct          float64 `json:"givebackPct"`
	AllowedGivebackPct   float64 `json:"allowedGivebackPct"`
	RemainingGivebackPct float64 `json:"remainingGivebackPct"`
	FloorPct             float64 `json:"floorPct"`
	Proximity            float64 `json:"proximity"`
	RoomPct              float64 `json:"roomPct"`
	MaxLossPct           float64 `json:"maxLossPct"`
}
