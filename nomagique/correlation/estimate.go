package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LagEstimate is what an estimator publishes for one timestamp offset: the
covariance, its standard error, and their ratio (Score). Score is only
meaningful when ScoreDefined.
*/
type LagEstimate struct {
	Covariance    float64
	StandardError float64
	Score         float64
	Support       float64
	LeftEnergy    float64
	RightEnergy   float64
	Defined       bool
	ScoreDefined  bool
}

/*
EstimateInput is one pair of decoded return paths, their energies, and the
timestamp offset at which the estimator is evaluated.
*/
type EstimateInput struct {
	Left        []temporal.LogReturn
	Right       []temporal.LogReturn
	LeftEnergy  float64
	RightEnergy float64
	Lag         int64
}

/*
LagProfileInput is two price paths searched at every lag.
*/
type LagProfileInput struct {
	Left  []temporal.Price
	Right []temporal.Price
}

/*
LagCandidate retains the complete estimator record and its own support. Y is
the covariance at the candidate's lag: the profile the lead-lag search
maximizes in absolute value.
*/
type LagCandidate struct {
	LagEstimate
	Index    float64
	LagIndex float64
	X        float64
	Y        float64
}

/*
decodePath decodes one price path into its returns and their energy.
*/
func decodePath(decoder core.Primitive, prices []temporal.Price) (temporal.ReturnPath, error) {
	var path temporal.ReturnPath

	for out := range decoder.Next(transport.NewValues(temporal.PricePath{Prices: prices}).Next(nil)) {
		path = *(*temporal.ReturnPath)(out)
	}

	if err := decoder.Error(); err != nil {
		return temporal.ReturnPath{}, err
	}

	return path, nil
}

/*
estimateAt drives the configured estimator primitive at one timestamp offset.
*/
func estimateAt(
	executor core.Primitive, left, right temporal.ReturnPath, lag int64,
) (LagEstimate, error) {
	var reading LagEstimate

	for out := range executor.Next(transport.NewValues(EstimateInput{
		Left:        left.Returns,
		Right:       right.Returns,
		LeftEnergy:  left.Energy,
		RightEnergy: right.Energy,
		Lag:         lag,
	}).Next(nil)) {
		reading = *(*LagEstimate)(out)
	}

	if err := executor.Error(); err != nil {
		return LagEstimate{}, err
	}

	return reading, nil
}
