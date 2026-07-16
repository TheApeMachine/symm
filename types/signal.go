package types

/*
Signal conditions one market input into numerical measurements. Market
interpretations are deliberately absent because they belong to logic.
*/
type Signal interface {
	Measure(*Thesis) chan []*Measurement
}
