package relation

import (
	"fmt"
	"iter"
	"maps"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Influence measures directed temporal predictive contribution of Source on
Target beyond Target's own history and explicit Controls.
*/
type Influence struct {
	*core.PrimitiveError
	version   string
	store     core.Primitive
	align     core.Primitive
	snr       core.Primitive
	median    core.Primitive
	target    []float64
	source    []float64
	controls  [][]float64
	lags      []float64
	series    [][]float64
	gaps      []float64
	full      []float64
	restrict  []float64
	fit       []float64
	residuals [2][]float64
	keys      []string
	candidate map[string]float64
	best      map[string]float64
}

/*
NewInfluence builds the estimator over store.
*/
func NewInfluence(version string, store core.Primitive) *Influence {
	op := &Influence{
		PrimitiveError: core.NewPrimitiveError(),
		version:        version,
		store:          store,
		align:          NewAlign(),
		snr:            statistic.NewCoefficientSNR(),
		median:         statistic.NewMedian(),
		keys: []string{
			"lag", "lag_resolution", "lag_search_span", "lag_support_bound",
			"lag_candidate_count", "defined_steps", "effective_sample_count", "maturity",
			"coefficient", "coefficient_variance", "coefficient_snr",
			"restricted_residual_variance", "full_residual_variance", "predictive_gain",
			"from", "at", "source_observed_at", "target_observed_at", "source_age",
		},
		candidate: make(map[string]float64),
		best:      make(map[string]float64),
	}

	if version == "" {
		op.Error(fmt.Errorf("%w: relation: influence estimator requires a version", core.ErrDomain))
	}

	if store == nil {
		op.Error(fmt.Errorf("%w: relation: influence estimator requires a store", core.ErrDomain))
	}

	return op
}

func (op *Influence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			req := (*Candidate)(arriving)

			if req == nil {
				op.Error(core.ErrShape)
				return
			}

			source := req.Source
			target := req.Target
			minLag := req.MinLag
			maxLag := req.MaxLag
			controlKeys := req.Controls
			controlLags := req.ControlLags
			controlsCount := len(controlKeys)

			for len(op.controls) < controlsCount {
				op.controls = append(op.controls, nil)
			}

			op.target = op.target[:0]
			op.source = op.source[:0]

			for index := range controlsCount {
				op.controls[index] = op.controls[index][:0]
			}

			for pointer := range op.store.Next(nil) {
				windows := *(*map[string][]float64)(pointer)
				op.target = append(op.target, windows[target]...)
				op.source = append(op.source, windows[source]...)

				for index, key := range controlKeys {
					op.controls[index] = append(op.controls[index], windows[key]...)
				}
			}

			if err := op.store.Error(); err != nil {
				op.Error(err)
				return
			}

			metrics := make(map[string]float64)

			for _, key := range op.keys {
				metrics[key] = math.NaN()
			}

			status := FitOK

		estimate:
			for range 1 {
				if len(op.source) == 0 {
					status = FitNoSourceHistory
					break estimate
				}

				if len(op.target) == 0 {
					status = FitNoTargetHistory
					break estimate
				}

				for index := range controlsCount {
					if len(op.controls[index]) == 0 {
						status = FitControlUnavailable
						break estimate
					}
				}

				cadences := [2]float64{}

				for side, history := range [2][]float64{op.source, op.target} {
					op.gaps = op.gaps[:0]

					for index := 2; index+1 < len(history); index += 2 {
						if gap := history[index] - history[index-2]; gap > 0 {
							op.gaps = append(op.gaps, gap)
						}
					}

					if len(op.gaps) == 0 {
						continue
					}

					for pointer := range op.median.Next(data.NewValue(op.gaps...).Next(nil)) {
						cadences[side] = *(*float64)(pointer)
					}

					if err := op.median.Error(); err != nil {
						op.Error(err)
						return
					}
				}

				resolution := max(cadences[0], cadences[1])

				if resolution <= 0 {
					status = FitNoPositiveLag
					break estimate
				}

				searchSpan := op.target[len(op.target)-2] - op.source[0]

				if searchSpan <= 0 {
					status = FitNoPositiveLag
					break estimate
				}

				restrictedParameters := 2 + controlsCount
				fullParameters := 3 + controlsCount
				supportBound := float64(max(0, len(op.target)/2-(4+controlsCount))) * resolution

				if maxLag <= 0 || maxLag > searchSpan {
					maxLag = searchSpan
				}

				maxLag = min(maxLag, supportBound)
				startLag := max(minLag, resolution)
				candidates := 0
				found := false

				op.series = append(op.series[:0], nil, op.target, op.target)
				op.series = append(op.series, op.controls[:controlsCount]...)
				op.series = append(op.series, op.source)

				for lag := startLag; startLag > 0 && lag <= maxLag; lag += resolution {
					surface := "lag_surface." + strconv.Itoa(candidates)
					metrics[surface] = lag
					metrics[surface+".gain"] = math.NaN()
					metrics[surface+".steps"] = 0
					candidates++

					op.lags = append(op.lags[:0], lag)

					for _, cLag := range controlLags {
						if cLag > 0 {
							op.lags = append(op.lags, cLag)
							continue
						}

						op.lags = append(op.lags, lag)
					}

					op.lags = append(op.lags, lag)
					op.series[0] = op.lags
					var rows [][]float64

					for pointer := range op.align.Next(data.NewValue(op.series).Next(nil)) {
						rows = *(*[][]float64)(pointer)
					}

					if err := op.align.Error(); err != nil {
						op.Error(err)
						return
					}

					if len(rows) == 0 {
						continue
					}

					restricted := statistic.NewRegressionAccumulator(restrictedParameters)
					full := statistic.NewRegressionAccumulator(fullParameters)
					op.residuals[0], op.residuals[1] = op.residuals[0][:0], op.residuals[1][:0]
					rankDeficient := false

					for _, row := range rows {
						op.full = append(op.full[:0], 1, row[3])

						for index := range controlsCount {
							op.full = append(op.full, row[5+2*index])
						}

						op.restrict = append(append(op.restrict[:0], op.full...), row[1])
						op.full = append(op.full, row[len(row)-1], row[1])

						var restrictedReading, fullReading []float64

						for pointer := range restricted.Next(data.NewValue(op.restrict).Next(nil)) {
							restrictedReading = *(*[]float64)(pointer)
						}

						if err := restricted.Error(); err != nil {
							op.Error(err)
							return
						}

						for pointer := range full.Next(data.NewValue(op.full).Next(nil)) {
							fullReading = *(*[]float64)(pointer)
						}

						if err := full.Error(); err != nil {
							op.Error(err)
							return
						}

						if int(restrictedReading[3])-1 > restrictedParameters && restrictedReading[1] != 1 {
							rankDeficient = true
						}

						if int(fullReading[3])-1 > fullParameters && fullReading[1] != 1 {
							rankDeficient = true
						}

						if restrictedReading[1] == 1 && fullReading[1] == 1 {
							op.residuals[0] = append(op.residuals[0], row[1]-restrictedReading[0])
							op.residuals[1] = append(op.residuals[1], row[1]-fullReading[0])
						}

						op.fit = append(op.fit[:0], fullReading...)
					}

					clear(op.candidate)

					for _, key := range op.keys {
						op.candidate[key] = math.NaN()
					}

					steps := float64(len(op.residuals[0]))
					op.candidate["status"] = FitOK
					op.candidate["lag"] = lag
					op.candidate["defined_steps"] = steps
					metrics[surface+".steps"] = steps

				fit:
					for range 1 {
						if rankDeficient {
							op.candidate["status"] = FitRankDeficient
							break fit
						}

						if len(op.residuals[0]) == 0 {
							op.candidate["status"] = FitResidualVarianceUnavailable
							break fit
						}

						variances := [2]float64{}

						for side, residuals := range op.residuals {
							for _, residual := range residuals {
								variances[side] += residual * residual
							}

							variances[side] /= float64(len(residuals))
						}

						op.candidate["restricted_residual_variance"] = variances[0]
						op.candidate["full_residual_variance"] = variances[1]

						if variances[0] > 0 && variances[1] > 0 &&
							!math.IsInf(variances[0], 0) && !math.IsInf(variances[1], 0) {
							op.candidate["predictive_gain"] = math.Log(variances[0] / variances[1])
							metrics[surface+".gain"] = op.candidate["predictive_gain"]
						}

						last := rows[len(rows)-1]
						op.candidate["from"] = rows[0][0]
						op.candidate["at"] = last[0]
						op.candidate["target_observed_at"] = last[0]
						op.candidate["source_observed_at"] = last[len(last)-2]
						op.candidate["source_age"] = last[0] - last[len(last)-2]

						effective := float64(len(rows))
						op.candidate["effective_sample_count"] = effective
						op.candidate["maturity"] = 0

						if len(rows) > 1 {
							op.candidate["maturity"] = 1 - 1/effective
						}

						if len(op.fit) < 8 || op.fit[2] != 1 {
							op.candidate["status"] = FitRankDeficient

							if len(rows) <= restrictedParameters {
								op.candidate["status"] = FitInsufficientSupport
							}

							break fit
						}

						column := restrictedParameters
						parameters := int(op.fit[4])
						coefficient := op.fit[8+column]
						op.candidate["coefficient"] = coefficient

						if column < parameters && op.fit[7] == 1 {
							variance := op.fit[8+parameters+column]

							if !math.IsNaN(variance) && variance > 0 {
								op.candidate["coefficient_variance"] = variance

								for pointer := range op.snr.Next(data.NewValue([2]float64{coefficient, variance}).Next(nil)) {
									op.candidate["coefficient_snr"] = *(*float64)(pointer)
								}

								if err := op.snr.Error(); err != nil {
									op.Error(err)
									return
								}
							}
						}
					}

					better := !found

					if found {
						candidateGain, bestGain := op.candidate["predictive_gain"], op.best["predictive_gain"]
						candidateDefined, bestDefined := !math.IsNaN(candidateGain), !math.IsNaN(bestGain)

						switch {
						case candidateDefined != bestDefined:
							better = candidateDefined
						case candidateDefined && candidateGain != bestGain:
							better = candidateGain > bestGain
						case op.candidate["defined_steps"] != op.best["defined_steps"]:
							better = op.candidate["defined_steps"] > op.best["defined_steps"]
						default:
							better = op.candidate["lag"] < op.best["lag"]
						}
					}

					if better {
						clear(op.best)
						maps.Copy(op.best, op.candidate)
						found = true
					}
				}

				if !found {
					status = FitNoPositiveLag
					break estimate
				}

				for key, value := range op.best {
					metrics[key] = value
				}

				status = int(op.best["status"])
				metrics["lag_resolution"] = resolution
				metrics["lag_search_span"] = searchSpan
				metrics["lag_support_bound"] = supportBound
				metrics["lag_candidate_count"] = float64(candidates)
			}

			metrics["status"] = float64(status)

			estimateResult := &Estimate{
				Status:           status,
				Lag:              metrics["lag"],
				EstimatorVersion: op.version,
				Metrics:          metrics,
			}

			for value := range data.NewValue(unsafe.Pointer(estimateResult)).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
