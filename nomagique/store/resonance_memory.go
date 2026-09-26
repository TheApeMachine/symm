package store

import (
	"gonum.org/v1/gonum/mat"
	"math"
)

/*
	resonanceMemory measures the lag-one covariance of successive complete input

vectors using sufficient statistics. Its reach ends where propagated correlation
falls below its measured sampling resolution, 1/sqrt(pair support).
*/
type resonanceMemory struct {
	previous                            []float64
	left, right                         []float64
	count                               int
	leftEnergy, rightEnergy, covariance float64
}

func (memory *resonanceMemory) observe(values []float64) int {
	if memory.previous == nil {
		memory.previous = append([]float64(nil), values...)

		if memory.left == nil {
			memory.left = make([]float64, len(values))
			memory.right = make([]float64, len(values))
		}
		return 1
	}
	memory.count++
	for index, current := range values {
		prior := memory.previous[index]
		leftDelta := prior - memory.left[index]
		rightDelta := current - memory.right[index]
		memory.left[index] += leftDelta / float64(memory.count)
		memory.right[index] += rightDelta / float64(memory.count)
		memory.leftEnergy += leftDelta * (prior - memory.left[index])
		memory.rightEnergy += rightDelta * (current - memory.right[index])
		memory.covariance += leftDelta * (current - memory.right[index])
		memory.previous[index] = current
	}
	if memory.count < 2 || memory.leftEnergy == 0 || memory.rightEnergy == 0 || memory.covariance == 0 {
		return 1
	}
	correlation := math.Abs(memory.covariance) / math.Sqrt(memory.leftEnergy*memory.rightEnergy)
	// A perfectly retained direction has no finite decay time; only observed
	// support limits the set of future offsets that may be tested.
	if correlation >= 1 {
		return memory.count
	}
	reach := int(math.Floor(math.Log(math.Sqrt(float64(memory.count))) / -math.Log(correlation)))
	return min(memory.count, max(1, reach))
}

/*
	extend adds the next causal task row only after existing rows have empirical

skill and the observed temporal covariance supports that further offset.
*/
func (manifold *resonanceManifold) extend(reach int) {
	if reach <= manifold.taskRows {
		return
	}
	last := manifold.taskRows - 1
	if !manifold.taskSkillReady[last] || manifold.taskSkill.AtVec(last) <= 1 {
		return
	}
	learner := manifold.taskLearners[last]
	if learner.observations <= learner.rank {
		return
	}
	rows := manifold.taskRows + 1
	weights := mat.NewDense(rows, manifold.readoutDim, nil)
	weights.Slice(0, manifold.taskRows, 0, manifold.readoutDim).(*mat.Dense).Copy(manifold.taskWeights)
	manifold.taskWeights = weights
	for _, field := range []**mat.VecDense{&manifold.taskBias, &manifold.taskVar, &manifold.taskScale, &manifold.taskPrecision, &manifold.taskModelLoss, &manifold.taskBaselineLoss, &manifold.taskSkill} {
		values := make([]float64, rows)
		copy(values, (*field).RawVector().Data)
		*field = mat.NewVecDense(rows, values)
	}
	manifold.taskLearners = append(manifold.taskLearners, newResonanceTask(manifold.readoutDim))
	manifold.taskSupport = append(manifold.taskSupport, 0)
	manifold.taskScaleReady = append(manifold.taskScaleReady, false)
	manifold.taskSkillReady = append(manifold.taskSkillReady, false)
	manifold.workspace.taskPred = mat.NewVecDense(rows, nil)
	manifold.taskRows = rows
}
