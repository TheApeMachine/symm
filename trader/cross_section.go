package trader

import (
	"runtime"
	"sync"
	"time"

	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
crossSectionSnapshot is an immutable cross-section view rebuilt on the UI
heartbeat. calibrateOpportunity reads it instead of re-running Decisions for
every symbol on each measurement.
*/
type crossSectionSnapshot struct {
	Scores            []float64
	Baseline          float64
	Spread            float64
	Candidates        int
	Measurements      int
	AvgPrediction     float64
	AvgRequiredReturn float64
	AvgPredictionMult float64
	ForecastSymbols   int
}

type symbolReading struct {
	symbol string
	set    map[perspectives.SourceType]timedMeasurement
}

type crossSectionWorkerResult struct {
	scores        []float64
	measurements  int
	candidates    int
	predictionSum float64
	requiredSum   float64
	forecastCount int
}

func (crypto *Crypto) ensureCrossSection() *crossSectionSnapshot {
	snapshot := crypto.crossSection.Load()

	if snapshot != nil {
		return snapshot
	}

	crypto.refreshCrossSection()

	return crypto.crossSection.Load()
}

func (crypto *Crypto) refreshCrossSection() {
	now := time.Now()

	crypto.mu.RLock()
	rows := make([]symbolReading, 0, len(crypto.readings))

	for symbol, set := range crypto.readings {
		rows = append(rows, symbolReading{symbol: symbol, set: copyReadingSet(set)})
	}

	crypto.mu.RUnlock()

	if len(rows) == 0 {
		crypto.crossSection.Store(&crossSectionSnapshot{})

		return
	}

	scores := make([]float64, len(rows))
	workerCount := runtime.NumCPU()

	if workerCount > 8 {
		workerCount = 8
	}

	if workerCount < 1 {
		workerCount = 1
	}

	if len(rows) < workerCount*4 {
		workerCount = 1
	}

	chunk := (len(rows) + workerCount - 1) / workerCount
	type crossSectionJob struct {
		start int
		end   int
	}

	jobs := make([]crossSectionJob, 0, workerCount)

	for worker := 0; worker < workerCount; worker++ {
		start := worker * chunk

		if start >= len(rows) {
			break
		}

		end := start + chunk

		if end > len(rows) {
			end = len(rows)
		}

		jobs = append(jobs, crossSectionJob{start: start, end: end})
	}

	results := make([]crossSectionWorkerResult, len(jobs))
	var waitGroup sync.WaitGroup

	for jobIndex, job := range jobs {
		waitGroup.Go(func() {
			local := crossSectionWorkerResult{
				scores: make([]float64, job.end-job.start),
			}

			for index := job.start; index < job.end; index++ {
				row := rows[index]
				measurements := snapshotTimedMeasurements(row.set, now)

				for _, slot := range row.set {
					if !slot.Stale(now) {
						local.measurements++
					}
				}

				context := func(playbook string) perspectives.DecisionContext {
					return crypto.entryDecisionContext(row.symbol, measurements, playbook, 0)
				}
				names := entryNames(decision.DecisionsWithContext(measurements, nil, context))
				score := thesisScore(measurements, names)
				local.scores[index-job.start] = score

				if len(names) == 0 || score <= 0 {
					continue
				}

				local.candidates++

				feePct := crypto.takerFeePct(row.symbol)
				spreadBPS := crypto.quotes.spreadBPS(row.symbol)
				required := crypto.scopedRuntime().Risk.EntryEdgeMultiple * roundTripFrictionPct(feePct, spreadBPS)

				if required <= 0 {
					continue
				}

				local.forecastCount++
				local.predictionSum += score / 100
				local.requiredSum += required
			}

			results[jobIndex] = local
		})
	}

	waitGroup.Wait()

	measurementCount := 0
	candidateCount := 0
	predictionSum := 0.0
	requiredSum := 0.0
	forecastSymbols := 0

	for jobIndex, job := range jobs {
		local := results[jobIndex]

		for offset, score := range local.scores {
			scores[job.start+offset] = score
		}

		measurementCount += local.measurements
		candidateCount += local.candidates
		predictionSum += local.predictionSum
		requiredSum += local.requiredSum
		forecastSymbols += local.forecastCount
	}

	baseline, spread := robustCenter(scores)
	avgPrediction := 0.0
	avgRequired := 0.0

	if forecastSymbols > 0 {
		avgPrediction = predictionSum / float64(forecastSymbols)
		avgRequired = requiredSum / float64(forecastSymbols)
	}

	avgPredictionMult := 0.0

	if avgRequired > 0 {
		avgPredictionMult = avgPrediction / avgRequired
	}

	crypto.crossSection.Store(&crossSectionSnapshot{
		Scores:            scores,
		Baseline:          baseline,
		Spread:            spread,
		Candidates:        candidateCount,
		Measurements:      measurementCount,
		AvgPrediction:     avgPrediction,
		AvgRequiredReturn: avgRequired,
		AvgPredictionMult: avgPredictionMult,
		ForecastSymbols:   forecastSymbols,
	})
}
