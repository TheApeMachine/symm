package numeric

/*
TransitionMatrix tracks category transitions with Dirichlet-smoothed counts and
scores surprisal via KL divergence against an observed distribution.
*/
type TransitionMatrix struct {
	counts       [][]float64
	lastCategory int
	numStates    int
	floor        float64
}

func NewTransitionMatrix(numStates int, alpha float64) *TransitionMatrix {
	counts := make([][]float64, numStates)

	for row := range counts {
		counts[row] = make([]float64, numStates)

		for column := range counts[row] {
			counts[row][column] = alpha
		}
	}

	return &TransitionMatrix{
		counts:       counts,
		lastCategory: 0,
		numStates:    numStates,
		floor:        1e-6,
	}
}

func (matrix *TransitionMatrix) Surprise(observed []float64) float64 {
	row := matrix.counts[matrix.lastCategory]
	rowSum := 0.0

	for _, count := range row {
		rowSum += count
	}

	return KLDivergence(observed, row, rowSum, matrix.floor)
}

func (matrix *TransitionMatrix) Update(stateIndex int) {
	matrix.counts[matrix.lastCategory][stateIndex] += 1.0
	matrix.lastCategory = stateIndex
}

func (matrix *TransitionMatrix) NumStates() int {
	return matrix.numStates
}

/*
PadObserved maps an N-category distribution into a numStates vector with a
leading none-state mass, then normalizes.
*/
func (matrix *TransitionMatrix) PadObserved(
	distribution []float64, leadingMass float64,
) []float64 {
	if leadingMass <= 0 {
		leadingMass = matrix.floor
	}

	padded := make([]float64, matrix.numStates)
	padded[0] = leadingMass

	for index, probability := range distribution {
		target := index + 1

		if target >= matrix.numStates {
			break
		}

		padded[target] = probability
	}

	sum := 0.0

	for _, probability := range padded {
		sum += probability
	}

	if sum <= 0 {
		return padded
	}

	for index := range padded {
		padded[index] /= sum
	}

	return padded
}

func (matrix *TransitionMatrix) Reset() error {
	alpha := matrix.counts[0][0]

	for row := range matrix.counts {
		for column := range matrix.counts[row] {
			matrix.counts[row][column] = alpha
		}
	}

	matrix.lastCategory = 0

	return nil
}

func (matrix *TransitionMatrix) Next(
	out float64, values ...float64,
) (float64, error) {
	_ = out

	if len(values) == 0 {
		return 0, nil
	}

	stateIndex := int(values[0])
	observed := values[1:]

	surprise := matrix.Surprise(observed)
	matrix.Update(stateIndex)

	return surprise, nil
}
