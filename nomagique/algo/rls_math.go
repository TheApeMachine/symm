package algo

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

/*
RLSState is the posterior and the query design used to forecast.
*/
type RLSState struct {
	Beta         []float64
	Design       []float64
	Root         [][]float64
	NoiseShape   float64
	NoiseScale   float64
	Observations float64
}

/*
RLSForecast is the projection of a posterior through a design.
*/
type RLSForecast struct {
	RLSState
	Prediction         float64
	Factor             []float64
	Scale              float64
	DegreesOfFreedom   float64
	PredictiveVariance float64
	Ready              bool
}

/*
RLSObservation is a forecast plus the target and forgetting that close the update.
*/
type RLSObservation struct {
	RLSForecast
	Lambda float64
	Target float64
}

/*
RLSPosterior is the symmetric square-root rank-one update.
*/
type RLSPosterior struct {
	RLSForecast
	Alpha            float64
	Innovation       float64
	RootLambda       float64
	GammaDenominator float64
	Gain             []float64
}

/*
NewRLSPrediction creates a Value closure forecasting from the supplied posterior before any model update.
No structs, pure Value closure.
*/
type RLSPredictionServer struct{}

func (s *RLSPredictionServer) Forecast(state RLSState) RLSForecast {
	if len(state.Beta) != len(state.Design) || len(state.Root) != len(state.Design) {
		return RLSForecast{RLSState: state}
	}

	factor := make([]float64, len(state.Design))
	value := 0.0

	for row, feature := range state.Design {
		if len(state.Root[row]) != len(state.Design) {
			return RLSForecast{RLSState: state}
		}

		value += state.Beta[row] * feature

		for column, coefficient := range state.Root[row] {
			factor[column] += coefficient * feature
		}
	}

	forecast := RLSForecast{
		RLSState:   state,
		Prediction: value,
		Factor:     factor,
	}

	if state.NoiseShape > 0 && state.NoiseScale > 0 {
		energy := 0.0
		for _, member := range factor {
			energy += member * member
		}

		variance := (state.NoiseScale / state.NoiseShape) * (state.Observations + energy)
		if variance > 0 {
			forecast.PredictiveVariance = variance
			forecast.Scale = math.Sqrt(variance)
			forecast.DegreesOfFreedom = 2 * state.NoiseShape
			forecast.Ready = true
		}
	}

	return forecast
}

func NewRLSPrediction() *RLSPredictionServer {
	return &RLSPredictionServer{}
}

/*
NewRLSUpdate creates a Value closure executing the symmetric square-root rank-one update.
No structs, pure Value closure.
*/
type RLSUpdateServer struct{}

func (s *RLSUpdateServer) Update(obs RLSObservation) RLSPosterior {
	beta := obs.Beta
	root := obs.Root
	factor := obs.Factor
	lambda := obs.Lambda
	innovation := obs.Target - obs.Prediction

	if len(root) != len(beta) || len(factor) != len(beta) {
		return RLSPosterior{RLSForecast: obs.RLSForecast}
	}

	energy := 0.0
	for _, value := range factor {
		energy += value * value
	}

	alpha := lambda + energy
	if !(alpha > 0) {
		return RLSPosterior{RLSForecast: obs.RLSForecast}
	}

	rootLambda := math.Sqrt(lambda)
	denominator := alpha + rootLambda*math.Sqrt(alpha)
	gain := make([]float64, len(beta))
	coefficients := make([]float64, len(beta))
	posterior := make([][]float64, len(root))
	storage := make([]float64, len(root)*len(root))

	for row := range root {
		if len(root[row]) != len(beta) {
			return RLSPosterior{RLSForecast: obs.RLSForecast}
		}

		for column, coefficient := range root[row] {
			gain[row] += coefficient * factor[column]
		}

		gain[row] /= alpha
		coefficients[row] = beta[row] + gain[row]*innovation
		posterior[row] = storage[row*len(root) : (row+1)*len(root)]

		for column, coefficient := range root[row] {
			posterior[row][column] = (coefficient - gain[row]*(alpha/denominator)*factor[column]) / rootLambda
		}
	}

	noise := lambda*obs.NoiseScale + 0.5*innovation*innovation/alpha
	result := obs.RLSForecast
	result.Beta = coefficients
	result.Root = posterior
	result.NoiseShape = lambda*obs.NoiseShape + 0.5
	result.NoiseScale = noise
	result.Observations++

	return RLSPosterior{
		RLSForecast:      result,
		Alpha:            alpha,
		Innovation:       innovation,
		RootLambda:       rootLambda,
		GammaDenominator: denominator,
		Gain:             gain,
	}
}

func NewRLSUpdate() *RLSUpdateServer {
	return &RLSUpdateServer{}
}

type RLSServer struct {
	theta float64
	p     float64
	out   float64
}

func NewRLS() *RLSServer {
	return &RLSServer{
		theta: 0.0,
		p:     1.0,
	}
}

func (server *RLSServer) Write(ctx context.Context, call RLS_write) error {
	args := call.Args()
	valX := args.X()
	target := args.Target()
	lambda := args.Lambda()

	if lambda <= 0 {
		lambda = 1.0
	}

	prediction := server.theta * valX
	residual := target - prediction

	denom := lambda + server.p*valX*valX
	var gain float64

	if denom > 0 {
		gain = (server.p * valX) / denom
	}

	server.theta += gain * residual
	server.p = (server.p - gain*valX*server.p) / lambda

	server.out = prediction
	return nil
}

func (server *RLSServer) Done(ctx context.Context, call RLS_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"rls: alloc results failed",
			err,
		))
	}

	results.SetOut(server.out)
	server.out = 0
	return nil
}
