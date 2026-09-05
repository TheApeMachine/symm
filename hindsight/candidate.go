package hindsight

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/* CandidateRecord is immutable prospective input; later outcomes are separate events. */
type CandidateRecord struct {
	ID             string                `json:"id"`
	Decision       uint64                `json:"decision"`
	Symbol         string                `json:"symbol"`
	Action         string                `json:"action"`
	Power          uint16                `json:"power"`
	At             time.Time             `json:"at"`
	MarketAt       time.Time             `json:"marketAt"`
	Capture        CaptureIdentity       `json:"capture"`
	GridVersion    uint64                `json:"gridVersion"`
	Context        []uint64              `json:"context"`
	Quantities     [][2]string           `json:"quantities"`
	Scope          string                `json:"scope"`
	Global         learning.PriorReading `json:"global"`
	SymbolPrior    learning.PriorReading `json:"symbolPrior"`
	Prior          learning.PriorReading `json:"prior"`
	Authority      float64               `json:"authority"`
	Quantity       string                `json:"quantity"`
	Notional       string                `json:"notional"`
	Reference      string                `json:"reference"`
	Horizon        time.Duration         `json:"horizonNs"`
	QtyMinimum     string                `json:"qtyMinimum"`
	QtyIncrement   string                `json:"qtyIncrement"`
	CostMinimum    string                `json:"costMinimum"`
	FeeRate        string                `json:"feeRate"`
	AccountCash    string                `json:"accountCash,omitempty"`
	AccountEquity  string                `json:"accountEquity,omitempty"`
	AccountVersion uint64                `json:"accountVersion"`
}

/* CandidateResult labels an existing candidate without rewriting its input. */
type CandidateResult struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	At          time.Time `json:"at"`
	PortfolioID string    `json:"portfolioId,omitempty"`
	Detail      string    `json:"detail,omitempty"`
}
