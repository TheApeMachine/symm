package hindsight

import "time"

/* AccountMark identifies an observed account valuation and its separate funding reference in its producer session. */
type AccountMark struct {
	At         time.Time `json:"at"`
	Version    uint64    `json:"version"`
	Equity     float64   `json:"equity"`
	NetFunding float64   `json:"netFunding"`
	HasFunding bool      `json:"hasFunding"`
}
