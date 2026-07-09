package types

import (
	"time"
)

type Measurement struct {
	Source        SourceType         `json:"source,omitempty"`
	Stream        string             `json:"stream,omitempty"`
	Symbol        string             `json:"symbol,omitempty"`
	At            time.Time          `json:"at,omitempty"`
	Status        string             `json:"status,omitempty"`
	Elapsed       float64            `json:"elapsed,omitempty"`
	EntryBaseline float64            `json:"entryBaseline,omitempty"`
	ExitBaseline  float64            `json:"exitBaseline,omitempty"`
	Categories    []Category         `json:"categories"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`
}
