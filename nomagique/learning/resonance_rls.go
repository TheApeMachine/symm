package learning

import (
	"context"

	"github.com/theapemachine/symm/nomagique/algo"
)

/*
TaskLearnerServer forecasts or updates a task-head RLS model using the streaming pattern.
*/
type TaskLearnerServer struct {
	Downstream func(context.Context, algo.RLSPosterior) error
	
	state   algo.RLSState
	predict func(algo.RLSState) algo.RLSForecast
	update  func(algo.RLSObservation) algo.RLSPosterior
	lambda  float64
}

func NewTaskLearnerServer(dim int, lambda float64) *TaskLearnerServer {
	state := algo.RLSState{
		Beta:         make([]float64, dim),
		Design:       make([]float64, dim),
		Root:         make([][]float64, dim),
		NoiseShape:   0.001,
		NoiseScale:   0.001,
		Observations: 0,
	}
	for i := range state.Root {
		state.Root[i] = make([]float64, dim)
		state.Root[i][i] = 100.0 // Identity scaled by ridge
	}

	return &TaskLearnerServer{
		state:   state,
		predict: algo.NewRLSPrediction(),
		update:  algo.NewRLSUpdate(),
		lambda:  lambda,
	}
}

func (s *TaskLearnerServer) Evaluate(features []float64, target float64, observed bool) algo.RLSPosterior {
	// Prepare state design
	if len(s.state.Design) == len(features)+1 {
		copy(s.state.Design, features)
		s.state.Design[len(features)] = 1.0 // Bias
	} else if len(s.state.Design) == len(features) {
		copy(s.state.Design, features)
	}

	forecast := s.predict(s.state)
	
	var posterior algo.RLSPosterior
	if !observed {
		posterior = algo.RLSPosterior{RLSForecast: forecast}
	} else {
		obs := algo.RLSObservation{
			RLSForecast: forecast,
			Lambda:      s.lambda,
			Target:      target,
		}

		posterior = s.update(obs)
		s.state = posterior.RLSForecast.RLSState
	}
	return posterior
}

func (s *TaskLearnerServer) Write(ctx context.Context, call TaskLearner_write) error {
	args, err := call.Args().Input()
	if err != nil {
		return err
	}
	
	featuresList, err := args.Features()
	if err != nil {
		return err
	}
	
	n := featuresList.Len()
	features := make([]float64, n)
	for i := 0; i < n; i++ {
		features[i] = featuresList.At(i)
	}
	
	target := args.Target()
	observed := args.Observed()

	posterior := s.Evaluate(features, target, observed)

	if s.Downstream != nil {
		return s.Downstream(ctx, posterior)
	}
	return nil
}

func (s *TaskLearnerServer) Done(ctx context.Context, call TaskLearner_done) error {
	return nil
}
