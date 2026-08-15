package resonance

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
)

type issuedHorizon struct {
	horizon   int
	forecast  float64
	mark      float64
	issueTick int64
	train     bool
}

func (history *sampleHistory) resolve(
	coder *learning.ResonanceManifold,
	mark float64,
) error {
	pending := history.pending[history.sequence]
	delete(history.pending, history.sequence)

	for _, item := range pending {
		if item.mark <= 0 {
			continue
		}

		actual := math.Log(mark / item.mark)
		call := signedDirection(item.forecast)
		target := signedDirection(actual)
		history.ledger.observe(item.horizon, call, actual)
		history.lastResolution = &taskResolution{
			horizon:  item.horizon,
			forecast: call,
			actual:   actual,
			error:    target - call,
		}

		if !item.train || target == 0 {
			if item.train {
				delete(history.issued, item.issueTick)
			}

			continue
		}

		issued, found := history.issued[item.issueTick]
		delete(history.issued, item.issueTick)

		if !found {
			continue
		}

		if err := coder.ObserveTask(
			issued.features,
			issued.prediction,
			[]float64{target},
		); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("resonance: resolve task failed [%s]", err.Error()),
				err,
			))
		}

		history.resolved++
	}

	return nil
}

func (history *sampleHistory) issue(
	coder *learning.ResonanceManifold,
	tick int64,
	mark float64,
	probeHorizon int,
	forecast []learning.RLSOutput,
) error {
	if mark <= 0 || len(forecast) == 0 {
		return nil
	}

	call := signedDirection(forecast[0].Value)
	history.issued[tick] = issuedTask{
		features:   coder.LatentState(),
		prediction: []float64{call},
	}

	if call == 0 {
		history.pending[history.sequence+1] = append(
			history.pending[history.sequence+1],
			issuedHorizon{
				horizon:   1,
				forecast:  call,
				mark:      mark,
				issueTick: tick,
				train:     true,
			},
		)

		return nil
	}

	cumulative := 0.0

	for horizon := 1; horizon <= probeHorizon; horizon++ {
		if horizon > len(forecast) {
			continue
		}

		cumulative += forecast[horizon-1].Value
		horizonCall := signedDirection(cumulative)
		targetSequence := history.sequence + int64(horizon)

		history.pending[targetSequence] = append(
			history.pending[targetSequence],
			issuedHorizon{
				horizon:   horizon,
				forecast:  horizonCall,
				mark:      mark,
				issueTick: tick,
				train:     horizon == 1,
			},
		)
	}

	return nil
}

func (history *sampleHistory) observeTickMove(mark float64) error {
	if history.moves == nil {
		history.moves = adaptive.NewAccumulator()
	}

	if history.lastTickMark > 0 && mark > 0 && mark != history.lastTickMark {
		deviation := math.Log(mark / history.lastTickMark)

		measured, err := history.moves.Measure(deviation * deviation)

		if err != nil {
			return err
		}

		history.moveStat = measured
	}

	history.lastTickMark = mark

	return nil
}

func (history *sampleHistory) moveScale() float64 {
	if history == nil || history.moveStat.Count == 0 {
		return 0
	}

	return math.Sqrt(history.moveStat.Value / float64(history.moveStat.Count))
}

func (history *sampleHistory) distinguishable(actual float64) bool {
	scale := history.moveScale()

	if !(scale > 0) {
		return math.Abs(actual) > 0
	}

	return math.Abs(actual) >= scale
}

func (history *sampleHistory) inFlight() bool {
	return history != nil && len(history.issued) > 0
}

func (history *sampleHistory) pruneTicks() {
	for len(history.ticks) > 1 {
		oldest := history.ticks[0]

		if _, waiting := history.issued[oldest]; waiting {
			return
		}

		delete(history.marks, oldest)
		history.ticks = history.ticks[1:]
	}
}
