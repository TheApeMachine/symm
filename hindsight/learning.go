package hindsight

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/*
LearningEvent records a prospectively issued virtual decision, its execution,
or its completed return-to-go. At is the local producer's actual decision or
valuation time. MarketAt is the triggering market message time; a resident
book may already be newer when read. These are separate facts.
*/
type LearningEvent struct {
	ID          uint64                `json:"id"`
	Symbol      string                `json:"symbol"`
	Lane        int                   `json:"lane"`
	Mode        string                `json:"mode"`
	Kind        string                `json:"kind"`
	At          time.Time             `json:"at"`
	MarketAt    time.Time             `json:"marketAt"`
	GridVersion uint64                `json:"gridVersion"`
	Context     []uint64              `json:"context,omitempty"`
	Action      string                `json:"action"`
	Power       uint16                `json:"power"`
	Reduce      bool                  `json:"reduce"`
	Quantity    string                `json:"quantity,omitempty"`
	Gross       string                `json:"gross,omitempty"`
	Fee         string                `json:"fee,omitempty"`
	Cash        string                `json:"cash"`
	Inventory   string                `json:"inventory"`
	Authority   float64               `json:"authority"`
	Target      float64               `json:"target,omitempty"`
	Profit      float64               `json:"profit"`
	Complete    bool                  `json:"complete"`
	ValuedAt    time.Time             `json:"valuedAt"`
	Prior       learning.PriorReading `json:"prior"`
}
