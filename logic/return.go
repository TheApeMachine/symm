package logic

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
returnHead owns one strictly chronological midpoint-return RLS learner and its
out-of-sample error and incremental-skill calibration. The ladder supplies the
feature row, the strict-prior prediction made when that row was observed, and
the realized future midpoint; this object owns forecast truth for its horizon.
*/
type returnHead struct {
	learner     *learning.RLS
	confidence  float64
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
ResolveAgainst scores the strict-prior prediction made for one historical row
against the midpoint now realized at this head's horizon, then teaches the
learner that row's realized log return.
*/
func (head *returnHead) ResolveAgainst(
	features []float64,
	priorMid *decimal.Decimal,
	midPrice *decimal.Decimal,
	prediction float64,
	predicted bool,
) error {
	if priorMid == nil || priorMid.Sign() <= 0 ||
		midPrice == nil || midPrice.Sign() <= 0 {
		return errnie.Err(
			errnie.Validation,
			"logic return: midpoint prices must be strictly positive",
			nil,
		)
	}

	scale := max(
		int64(decimal.DefaultScale),
		midPrice.GetScale(),
		priorMid.GetScale(),
	)
	ratio := midPrice.SetScale(scale).
		Div(priorMid.SetScale(scale)).
		Float64()
	target := math.Log(ratio)

	if !finite(target) {
		return errnie.Err(
			errnie.Validation,
			"logic return: realized midpoint return is not finite",
			nil,
		)
	}

	if predicted {
		if err := head.calibrate(target, prediction); err != nil {
			return err
		}
	}

	if _, err := head.learner.Measure(learning.RLSSample{
		Features: features,
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
PredictRow evaluates one physical row without observing its future target; the
ladder retains the value so resolution scores exactly what was claimed.
*/
func (head *returnHead) PredictRow(features []float64) (float64, error) {
	output, err := head.learner.Predict(features)

	if err != nil || !finite(output.Value) {
		return 0, errnie.Err(
			errnie.UnprocessableContent,
			"logic return: RLS prediction failed",
			err,
		)
	}

	return output.Value, nil
}

/*
calibrate updates strict-prior squared error and the lower confidence bound of
improvement over a zero-return baseline.
*/
func (head *returnHead) calibrate(target float64, prediction float64) error {
	residual := target - prediction
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
