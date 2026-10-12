package tables

import "time"

/*
PositionRecord captures the immutable state of a traded position for Iceberg persistence.
*/
type PositionRecord struct {
	ID            string     `json:"id"`
	Symbol        string     `json:"symbol"`
	Status        string     `json:"status"`
	OpenedAt      time.Time  `json:"openedAt"`
	ClosedAt      *time.Time `json:"closedAt,omitempty"`
	Events        string     `json:"events"`
	VenueCost     float64    `json:"venueCost"`
	VenueProceeds float64    `json:"venueProceeds"`
	VenueFees     float64    `json:"venueFees"`
	VenueRealized float64    `json:"venueRealized"`
	HoldNs        int64      `json:"holdNs"`
	Outcome       string     `json:"outcome,omitempty"`
}
