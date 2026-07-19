package logic

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
returnHead owns the strictly chronological next-midpoint RLS learner and its
out-of-sample error and incremental-skill calibration. Resonance supplies only
the normalized physical feature row; this object owns forecast truth.
*/
type returnHead struct {
	learner     *learning.RLS
	confidence  float64
	pending     []float64
	pendingMid  float64
	prediction  float64
	predicted   bool
	mse         *statistic.Mean
	errors      *adaptive.Variance
	skill       *adaptive.Variance
	samples     uint64
	meanMSE     float64
	uncertainty float64
	skillLower  float64
}

/*
newReturnHead validates the configured RLS prior, memory policy, and one-sided
calibration confidence before accepting any market observation.
*/
func newReturnHead() (*returnHead, error) {
	learner, err := learning.NewRLS(learning.RLSConfig{
		Dimension:        resonanceObservables,
		InitialVariance:  viper.GetFloat64("market.forecast.rls.initial_variance"),
		ForgettingFactor: viper.GetFloat64("market.forecast.rls.forgetting_factor"),
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.Validation,
			"logic return: invalid RLS configuration",
			err,
		)
	}

	confidence := viper.GetFloat64("market.forecast.rls.calibration_confidence")

	if confidence <= 0.5 || confidence >= 1 {
		return nil, errnie.Err(
			errnie.Validation,
			"logic return: calibration confidence must be in (0.5,1)",
			nil,
		)
	}

	return &returnHead{
		learner:    learner,
		confidence: confidence,
		mse:        statistic.NewMean(),
		errors:     adaptive.NewVariance(),
		skill:      adaptive.NewVariance(),
	}, nil
}

/*
Resolve scores the prior prediction against the newly observed midpoint, then
teaches that strictly prior feature row its realized log return.
*/
func (head *returnHead) Resolve(midPrice float64) error {
	if len(head.pending) == 0 {
		return nil
	}

	if head.pendingMid <= 0 || midPrice <= 0 {
		return errnie.Err(
			errnie.Validation,
			"logic return: midpoint prices must be strictly positive",
			nil,
		)
	}

	target := math.Log(midPrice / head.pendingMid)

	if !finite(target) {
		return errnie.Err(
			errnie.Validation,
			"logic return: realized midpoint return is not finite",
			nil,
		)
	}

	if err := head.calibrate(target); err != nil {
		return err
	}

	if _, err := head.learner.Measure(learning.RLSSample{
		Features: head.pending,
		Target:   target,
	}); err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"logic return: RLS learn failed",
			err,
		)
	}

	return nil
}

/*
Predict evaluates the current physical row without observing its future target
and retains it for resolution by the next L3 epoch.
*/
func (head *returnHead) Predict(
	features []float64,
	midPrice float64,
) (float64, error) {
	output, err := head.learner.Predict(features)

	if err != nil || !finite(output.Value) {
		return 0, errnie.Err(
			errnie.UnprocessableContent,
			"logic return: RLS prediction failed",
			err,
		)
	}

	head.pending = append(head.pending[:0], features...)
	head.pendingMid = midPrice
	head.prediction = output.Value
	head.predicted = true

	return output.Value, nil
}

/*
calibrate updates strict-prior squared error and the lower confidence bound of
improvement over a zero-return baseline.
*/
func (head *returnHead) calibrate(target float64) error {
	if !head.predicted {
		return nil
	}

	residual := target - head.prediction
	mse, err := head.mse.Measure(residual * residual)

	if err != nil {
		return errnie.Err(errnie.UnprocessableContent, "logic return: MSE update failed", err)
	}

	variance, err := head.errors.Measure(residual)

	if err != nil {
		return errnie.Err(errnie.UnprocessableContent, "logic return: uncertainty update failed", err)
	}

	skill, err := head.skill.Measure(target*target - residual*residual)

	if err != nil {
		return errnie.Err(errnie.UnprocessableContent, "logic return: skill update failed", err)
	}

	head.samples = uint64(mse.Count)
	head.meanMSE = mse.Value
	head.skillLower = head.lowerSkill(skill)

	if variance.Ready {
		head.uncertainty = math.Sqrt(math.Max(variance.Value, 0))
	}

	return nil
}

/*
Ready reports whether the return head's one-sided lower confidence bound shows
positive squared-error skill over the zero-return baseline.
*/
func (head *returnHead) Ready() bool {
	return head != nil && head.samples > 1 && head.skillLower > 0
}

/*
lowerSkill computes the configured one-sided Student-t lower confidence bound
for mean incremental squared-error skill.
*/
func (head *returnHead) lowerSkill(skill adaptive.VarianceOutput) float64 {
	if skill.Count < 2 {
		return 0
	}

	critical := distuv.StudentsT{
		Mu:    0,
		Sigma: 1,
		Nu:    float64(skill.Count - 1),
	}.Quantile(head.confidence)
	standardError := math.Sqrt(skill.Value / float64(skill.Count))

	return skill.Mean - critical*standardError
}
