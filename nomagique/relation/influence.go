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
Target beyond Target's own history and the explicit Controls. It never infers
roles from names and never claims causality.

Each arrival is **data.Adapter carrying the explicit roles:

	text    "source", "target", "control.<i>"   coordinate keys
	number  "controls"                          control count
	number  "control.<i>.lag"                   control lag (ns); <= 0 aligns at the source lag
	number  "min_lag", "max_lag"                candidate lag domain (ns); 0 derives it

The history is read from the ObservationStore. The evaluation is
causal/prequential: each target is predicted by models fitted strictly on
earlier rows (restricted: intercept, target past, controls; full: plus
source), and the best candidate lag ranks by defined predictive gain, then
defined steps, then the smaller lag.

It publishes on the same adapter, then yields it:

	status                       one of the Fit* constants
	lag, lag_resolution, lag_search_span, lag_support_bound   (ns)
	lag_candidate_count, defined_steps, effective_sample_count, maturity
	coefficient, coefficient_variance, coefficient_snr
	restricted_residual_variance, full_residual_variance, predictive_gain
	from, at, source_observed_at, target_observed_at, source_age   (ns)
	lag_surface.<i>, lag_surface.<i>.gain, lag_surface.<i>.steps
	text estimator_version

Mathematically undefined numbers are NaN; undefined is never zero. Publish
into a fresh adapter per estimate so nothing stale survives.
*/
type Influence struct {
	*core.PrimitiveError
	version   string
	store     core.Primitive
	align     core.Primitive
	snr       core.Primitive
	median    core.Primitive
	roles     data.Map[string]
	domain    data.Map[string]
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
NewInfluence builds the estimator over store. The version string is
provenance published with every estimate; an empty version or a missing store
is a domain failure and every run yields nothing.
*/
func NewInfluence(version string, store core.Primitive) *Influence {
	op := &Influence{
		PrimitiveError: core.NewPrimitiveError(),
		version:        version,
		store:          store,
		align:          NewAlign(),
		snr:            statistic.NewCoefficientSNR(),
		median:         statistic.NewMedian(),
		roles:          data.NewLiteral("source", "target"),
		domain:         data.NewMap("controls", "controls", "min_lag", "min_lag", "max_lag", "max_lag"),
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
			if arriving == nil || *(**data.Adapter)(arriving) == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)
			var source, target string
			var count, minLag, maxLag float64

			for pointer := range adapter.Next(data.NewValue(op.roles)) {
				roles := *(*data.Map[string])(pointer)
				source, target = roles.Values["source"], roles.Values["target"]
			}

			for pointer := range adapter.Next(data.NewValue(op.domain)) {
				domain := *(*data.Map[float64])(pointer)
				count, minLag, maxLag = domain.Values["controls"], domain.Values["min_lag"], domain.Values["max_lag"]
			}

			controls := int(count)
			controlKeys := make([]string, controls)
			controlLags := make([]float64, controls)

			if controls > 0 {
				keyRequest, lagRequest := data.NewLiteral(), data.NewMap()

				for index := range controls {
					name := "control." + strconv.Itoa(index)
					keyRequest.Values[name] = name
					lagRequest.Values[name+".lag"] = name + ".lag"
				}

				for pointer := range adapter.Next(data.NewValue(keyRequest)) {
					keys := *(*data.Map[string])(pointer)

					for index := range controls {
						controlKeys[index] = keys.Values["control."+strconv.Itoa(index)]
					}
				}

				for pointer := range adapter.Next(data.NewValue(lagRequest)) {
					lags := *(*data.Map[float64])(pointer)

					for index := range controls {
						controlLags[index] = lags.Values["control."+strconv.Itoa(index)+".lag"]
					}
				}
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			for len(op.controls) < controls {
				op.controls = append(op.controls, nil)
			}

			op.target, op.source = op.target[:0], op.source[:0]

			for index := range controls {
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

			published := data.NewOutputMap()

			for _, key := range op.keys {
				published.Values[key] = math.NaN()
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

				for index := range controls {
					if len(op.controls[index]) == 0 {
						status = FitControlUnavailable
						break estimate
					}
				}

				// The lag resolution is the slower median positive cadence of
				// Source and Target; fixed bar counts are never truth.
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

					for pointer := range op.median.Next(data.NewValue(op.gaps...)) {
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

				// Each resolution step of lag consumes at least one target
				// observation from the alignment, and a fit needs more rows
				// than parameters: the bound is derived provenance.
				restrictedParameters := 2 + controls
				fullParameters := 3 + controls
				supportBound := float64(max(0, len(op.target)/2-(4+controls))) * resolution

				if maxLag <= 0 || maxLag > searchSpan {
					maxLag = searchSpan
				}

				maxLag = min(maxLag, supportBound)
				startLag := max(minLag, resolution)
				candidates := 0
				found := false

				// series: lags, target, target past, controls..., source.
				op.series = append(op.series[:0], nil, op.target, op.target)
				op.series = append(op.series, op.controls[:controls]...)
				op.series = append(op.series, op.source)

				for lag := startLag; startLag > 0 && lag <= maxLag; lag += resolution {
					surface := "lag_surface." + strconv.Itoa(candidates)
					published.Values[surface] = lag
					published.Values[surface+".gain"] = math.NaN()
					published.Values[surface+".steps"] = 0
					candidates++

					op.lags = append(op.lags[:0], lag)

					for _, controlLag := range controlLags {
						if controlLag > 0 {
							op.lags = append(op.lags, controlLag)
							continue
						}

						op.lags = append(op.lags, lag)
					}

					op.lags = append(op.lags, lag)
					op.series[0] = op.lags
					var rows [][]float64

					for pointer := range op.align.Next(data.NewValue(op.series)) {
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
						// Design row: intercept, target past, controls..., source, target.
						op.full = append(op.full[:0], 1, row[3])

						for index := range controls {
							op.full = append(op.full, row[5+2*index])
						}

						op.restrict = append(append(op.restrict[:0], op.full...), row[1])
						op.full = append(op.full, row[len(row)-1], row[1])

						// Prequential step: the reading predicts with the model
						// fitted strictly on earlier rows, then incorporates
						// the row, so it never trains the model that scored it.
						var restrictedReading, fullReading []float64

						for pointer := range restricted.Next(data.NewValue(op.restrict)) {
							restrictedReading = *(*[]float64)(pointer)
						}

						if err := restricted.Error(); err != nil {
							op.Error(err)
							return
						}

						for pointer := range full.Next(data.NewValue(op.full)) {
							fullReading = *(*[]float64)(pointer)
						}

						if err := full.Error(); err != nil {
							op.Error(err)
							return
						}

						// A singular design with more rows than parameters is
						// rank deficiency; warm-up rows are merely undefined.
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

						// The full accumulator's reading after the last row is
						// the final full fit over every aligned row.
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
					published.Values[surface+".steps"] = steps

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

						// log(Vr / Vf) is defined only for positive finite
						// variances; every degenerate case is undefined.
						if variances[0] > 0 && variances[1] > 0 &&
							!math.IsInf(variances[0], 0) && !math.IsInf(variances[1], 0) {
							op.candidate["predictive_gain"] = math.Log(variances[0] / variances[1])
							published.Values[surface+".gain"] = op.candidate["predictive_gain"]
						}

						last := rows[len(rows)-1]
						op.candidate["from"] = rows[0][0]
						op.candidate["at"] = last[0]
						op.candidate["target_observed_at"] = last[0]
						op.candidate["source_observed_at"] = last[len(last)-2]
						op.candidate["source_age"] = last[0] - last[len(last)-2]

						// Every aligned row carries unit weight, so the Kish
						// effective sample size is the aligned row count.
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

								for pointer := range op.snr.Next(data.NewValue([2]float64{coefficient, variance})) {
									op.candidate["coefficient_snr"] = *(*float64)(pointer)
								}

								if err := op.snr.Error(); err != nil {
									op.Error(err)
									return
								}
							}
						}
					}

					// Rank by defined gain, then defined steps, then smaller lag.
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
					published.Values[key] = value
				}

				status = int(op.best["status"])
				published.Values["lag_resolution"] = resolution
				published.Values["lag_search_span"] = searchSpan
				published.Values["lag_support_bound"] = supportBound
				published.Values["lag_candidate_count"] = float64(candidates)
			}

			published.Values["status"] = float64(status)
			version := data.NewTextMap()
			version.Values["estimator_version"] = op.version

			for range adapter.Next(data.NewValue(published)) {
			}

			for range adapter.Next(data.NewValue(version)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
