package resonance

import "time"

type tickerFixture struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Volume    float64   `json:"volume"`
	ChangePct float64   `json:"change_pct"`
	Timestamp time.Time `json:"timestamp"`
}

type bookLevelFixture struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

type bookFixture struct {
	Symbol string             `json:"symbol"`
	Bids   []bookLevelFixture `json:"bids"`
	Asks   []bookLevelFixture `json:"asks"`
}
