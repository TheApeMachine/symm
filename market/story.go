package market

import "github.com/theapemachine/symm/market/perspectives"

/*
Story is the set of latest per-signal verdicts for the market.
It give us an insightful view of the market's current state.
This view must frame the dynamic construction of perspectives
from the incoming signal measurements.
*/
type Story struct {
	perspectives map[string]perspectives.Perspective
}

func NewStory() *Story {
	return &Story{
		perspectives: make(map[string]perspectives.Perspective),
	}
}
