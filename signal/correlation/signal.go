package correlation

import (
	"bytes"
	"context"
	"iter"
	"slices"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
Signal: The "Herd Behavior" Perspective

What it measures exactly (in isolation)

The Correlation signal measures synchronized return correlation across the subscribed universe using a rolling window of log-returns.
It determines if the market is moving as a single, indistinguishable block or if individual assets are exhibiting unique behavior.

*   Synchronized Log-Returns: It aligns price windows onto a shared time grid (e.g., 10-second bars) to calculate the Pearson correlation between pairs.
*   Peak Score: It identifies symbols that are hitting a "peak" in their correlation to the broader market, using an adaptive peak gate.
*   Hayashi-Yoshida Overlap Estimator: For high-frequency, asynchronous data where trades don't align perfectly on time bars, it uses the H-Y estimator to capture overlapping return intervals.

Semantically, what story does it tell?

*   The "Rising Tide" Story: It asks: "Is this asset special, or is it just being dragged along by the herd?". High correlation indicates that macro-systemic forces are dominant.
*   The "De-coupling" Story: It identifies "alpha" opportunities by spotting when an asset stops following its peers, suggesting a local catalyst is at play.
*   The "Liquidation" Story: Sudden spikes in cross-asset correlation toward **1.0** often signal systemic panics or liquidation cascades where everything is sold at once.

# Probability Visualization Categories

| Category         | Correlation Level | Variance | Market "Feel"                       |
|:-----------------|:------------------|:---------|:------------------------------------|
| Systemic Herd    | High ($> 0.85$)   | High     | Global Beta / Momentum Drift        |
| Decoupled Alpha  | Low               | High     | Unique Driver / Leading Move        |
| Stochastic Noise | Low               | Low      | Quiet / Indecisive                  |
| Divergent Stress | Negative          | High     | Contrarian Move / Relative Weakness |
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

func NewSignal(ctx context.Context, tree *dmt.Tree) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if datapoint == nil || crossSection == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		row, err := market.SymbolFromTicker(datapoint, 0)

		if errnie.Error(err) != nil {
			return
		}

		history := signal.tickerHistory(
			symbolsWithCurrent(crossSection.Symbols(), row.Name),
			datapoint,
			crossSection.MaxReturnWindow()+1,
			crossSection.MedianCadence(),
		)

		defer releaseArtifacts(history)

		cohort := signal.measureCohort(
			history,
			datapoint,
			row.Name,
			crossSection.MaxReturnWindow()+1,
		)

		if cohort == nil {
			return
		}

		measurement := measurementFromCohort(datapoint, row.Name, cohort)

		if measurement == nil {
			return
		}

		if !yield(measurement) {
			return
		}
	}
}

func (signal *Signal) measureCohort(
	history []*datura.Artifact,
	datapoint *datura.Artifact,
	symbol string,
	historyCap int,
) *datura.Artifact {
	cohort := nomagique.Number(
		algorithm.NewCohortSample(cohortSampleConfig(historyCap)),
		equation.NewCohort(equation.CohortConfig()),
	)
	defer cohort.Close()

	for _, frame := range history {
		if err := nomagique.RoundTripArtifact(frame, cohort); err != nil {
			signal.err = err

			continue
		}

		if frame.Timestamp() == datapoint.Timestamp() &&
			tickerSymbol(frame) == symbol &&
			datura.Peek[float64](frame, "output", "value") > 0 {
			return frame
		}
	}

	return nil
}

func cohortSampleConfig(historyCap int) *datura.Artifact {
	return datura.Acquire(
		"correlation-cohort-sample", datura.APPJSON,
	).WithAttributes(datura.Map[any]{
		"channel":     "ticker",
		"root":        "data",
		"symbolInput": "symbol",
		"priceInput":  "last",
		"historyCap":  float64(max(historyCap, 2)),
	})
}

func (signal *Signal) tickerHistory(
	symbols []string,
	current *datura.Artifact,
	limit int,
	cadence float64,
) []*datura.Artifact {
	if signal.tree == nil {
		clone, err := current.Clone()
		if err != nil {
			signal.err = err

			return nil
		}

		return []*datura.Artifact{clone}
	}

	currentStamp := current.Timestamp()
	currentSymbol := tickerSymbol(current)
	windowStart := tickerWindowStart(currentStamp, limit, cadence)
	seenCurrent := false
	history := make([]*datura.Artifact, 0, len(symbols)*max(limit, 2))

	for _, symbol := range symbols {
		prefix := tickerHistoryPrefix(symbol)
		var frames []*datura.Artifact

		signal.tree.WalkLowerBound(tickerHistoryLowerBound(symbol, windowStart), func(key, value []byte) bool {
			if !bytes.HasPrefix(key, prefix) {
				return false
			}

			prior := &datura.Artifact{}
			if _, err := prior.Unpack(value); err != nil {
				signal.err = errnie.Error(errnie.Err(errnie.Validation, "correlation: unpack ticker history", err))

				return true
			}

			if prior.Timestamp() > currentStamp {
				prior.Release()

				return false
			}

			if prior.Timestamp() < windowStart || tickerSymbol(prior) != symbol {
				prior.Release()

				return true
			}

			if prior.Timestamp() == currentStamp && tickerSymbol(prior) == currentSymbol {
				seenCurrent = true
			}

			frames = append(frames, prior)

			if len(frames) > limit {
				frames[0].Release()
				frames = frames[1:]
			}

			return true
		})

		history = append(history, frames...)
	}

	if !seenCurrent {
		clone, err := current.Clone()
		if err != nil {
			signal.err = err

			return history
		}

		history = append(history, clone)
	}

	sort.SliceStable(history, func(left, right int) bool {
		return history[left].Timestamp() < history[right].Timestamp()
	})

	return history
}

func tickerHistoryPrefix(symbol string) []byte {
	return []byte("ticker/" + symbol + "/")
}

func tickerHistoryLowerBound(symbol string, timestamp int64) []byte {
	if timestamp <= 0 {
		return tickerHistoryPrefix(symbol)
	}

	return []byte("ticker/" + symbol + "/" + datura.FormatTimestamp(timestamp) + "/")
}

func tickerWindowStart(currentStamp int64, limit int, cadence float64) int64 {
	if currentStamp <= 0 || cadence <= 0 {
		return currentStamp
	}

	window := time.Duration(max(limit, 2)) * time.Duration(cadence*float64(time.Second))

	if window <= 0 {
		return currentStamp
	}

	return currentStamp - int64(window)
}

func symbolsWithCurrent(symbols []string, current string) []string {
	if slices.Contains(symbols, current) {
		return symbols
	}

	next := make([]string, 0, len(symbols)+1)
	next = append(next, symbols...)

	return append(next, current)
}

func tickerSymbol(artifact *datura.Artifact) string {
	return datura.Peek[string](artifact, "data", 0, "symbol")
}

func releaseArtifacts(artifacts []*datura.Artifact) {
	for _, artifact := range artifacts {
		artifact.Release()
	}
}

func measurementFromCohort(
	datapoint *datura.Artifact,
	symbol string,
	cohort *datura.Artifact,
) *datura.Artifact {
	shares := []dist.Share{
		{
			Key:      "herdScore",
			Category: logic.CategorySystemicHerd,
			Mass:     datura.Peek[float64](cohort, "output", "herdScore"),
		},
		{
			Key:      "alphaScore",
			Category: logic.CategoryDecoupledAlpha,
			Mass:     datura.Peek[float64](cohort, "output", "alphaScore"),
		},
		{
			Key:      "noiseScore",
			Category: logic.CategoryStochasticNoise,
			Mass:     datura.Peek[float64](cohort, "output", "noiseScore"),
		},
		{
			Key:      "stressScore",
			Category: logic.CategoryDivergentStress,
			Mass:     datura.Peek[float64](cohort, "output", "stressScore"),
		},
	}

	measurement := datura.Acquire("correlation", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCorrelation)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.MergeOutput("correlation", datura.Peek[float64](cohort, "output", "correlation"))
	measurement.MergeOutput("energy", datura.Peek[float64](cohort, "output", "energy"))
	measurement.MergeOutput("peakScore", datura.Peek[float64](cohort, "output", "peakScore"))

	if confidence := dist.Write(measurement, shares); confidence <= 0 {
		measurement.Release()
		return nil
	}

	return measurement
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return signal.err
}
