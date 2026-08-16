package types

import "time"

/*
MarkFeedback is one executable-mark observation from an owned position. It is
not a realized wallet outcome: the regulator uses it as context for the next
complete account-equity revision, so dense ticker evidence cannot be mistaken
for independent profit observations.
*/
type MarkFeedback struct {
	PositionID    string    `json:"positionId,omitempty"`
	Symbol        string    `json:"symbol"`
	At            time.Time `json:"at"`
	Mark          float64   `json:"mark" validate:"finite,positive"`
	PnL           float64   `json:"pnl" validate:"finite"`
	ReturnPct     float64   `json:"returnPct" validate:"finite"`
	FloorDistance float64   `json:"floorDistance" validate:"finite"`
	PeakDrawdown  float64   `json:"peakDrawdown" validate:"finite,max=0"`
	SurgeArmed    bool      `json:"surgeArmed"`
	StopStatus    Status    `json:"stopStatus"`
	TriggerReason string    `json:"triggerReason,omitempty"`
	Exposed       bool      `json:"exposed"`
}
