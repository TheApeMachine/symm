package market

/*
Story is the set of latest per-signal verdicts for the market.
It give us an insightful view of the market's current state.
This view must frame the dynamic construction of perspectives
from the incoming signal measurements.
*/
type Story struct {
	perspectives map[PerspectiveType]*Perspective
}

func NewStory() *Story {
	return &Story{
		perspectives: make(map[PerspectiveType]*Perspective),
	}
}
