package numeric

import (
	"fmt"
	"math"
)

func AssertFinite(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("numeric: %s is non-finite", name)
	}

	return nil
}

func AssertFiniteScores(prefix string, scores []float64) error {
	for index, score := range scores {
		if err := AssertFinite(fmt.Sprintf("%s.score[%d]", prefix, index), score); err != nil {
			return err
		}
	}

	return nil
}
