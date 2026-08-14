package resonance

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
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
		history.ledger.observe(item.horizon, item.forecast, actual)
		history.lastResolution = &taskResolution{
			horizon:  item.horizon,
			forecast: item.forecast,
			actual:   actual,
			error:    actual - item.forecast,
		}

		if !item.train {
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
			[]float64{actual},
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

	lean := forecast[0].Value
	history.issued[tick] = issuedTask{
		features:   coder.LatentState(),
		prediction: []float64{lean},
	}

	if signedDirection(lean) == 0 {
		history.pending[history.sequence+1] = append(
			history.pending[history.sequence+1],
			issuedHorizon{
				horizon:   1,
				forecast:  lean,
				mark:      mark,
				issueTick: tick,
				train:     true,
			},
		)

		return nil
	}

	for horizon := 1; horizon <= probeHorizon; horizon++ {
		if horizon > len(forecast) {
			continue
		}

		targetSequence := history.sequence + int64(horizon)
		history.pending[targetSequence] = append(
			history.pending[targetSequence],
			issuedHorizon{
				horizon:   horizon,
				forecast:  lean,
				mark:      mark,
				issueTick: tick,
				train:     horizon == 1,
			},
		)
	}

	return nil
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
