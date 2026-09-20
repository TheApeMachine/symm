package learning

import (
	"context"

	"github.com/theapemachine/symm/nomagique/algo"
)

/*
TaskLearnerServer forecasts or updates a task-head RLS model using the streaming pattern.
*/
type TaskLearnerServer struct {
	state   algo.RLSState
	predict *algo.RLSPredictionServer
	update  *algo.RLSUpdateServer
	lambda  float64
	out     float64
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

func NewTaskLearner() *TaskLearnerServer {
	return NewTaskLearnerServer(1, 0.99)
}

func (s *TaskLearnerServer) Evaluate(features []float64, target float64, observed bool) algo.RLSPosterior {
	s.state.Design = features
	forecast := s.predict.Forecast(s.state)

	if !observed {
		return algo.RLSPosterior{RLSForecast: forecast}
	}

	obs := algo.RLSObservation{
		RLSForecast: forecast,
		Lambda:      s.lambda,
		Target:      target,
	}
	posterior := s.update.Update(obs)
	if !posterior.Ready {
		return algo.RLSPosterior{RLSForecast: forecast}
	}

	s.state = posterior.RLSForecast.RLSState
	return posterior
}

func (s *TaskLearnerServer) Write(ctx context.Context, call TaskLearner_write) error {
	args := call.Args()
	feature := args.Feature()
	target := args.Target()
	observed := args.Observed()

	posterior := s.Evaluate([]float64{feature}, target, observed)
	s.out = posterior.Innovation
	return nil
}

func (s *TaskLearnerServer) Done(ctx context.Context, call TaskLearner_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return err
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
