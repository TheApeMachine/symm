package numeric

import (
	"fmt"
	"math"
	"time"
)

/*
TimeElasticMemory tracks a time-decayed baseline and returns sample/baseline
before each update. Alpha uses halflife with tau = halflife / ln(2).
*/
type TimeElasticMemory struct {
	halflife    time.Duration
	epsilon     float64
	vSlow       float64
	lastAt      time.Time
	initialized bool
}

func NewTimeElasticMemory(halflife time.Duration, epsilon float64) *TimeElasticMemory {
	if epsilon <= 0 {
		epsilon = 1e-6
	}

	return &TimeElasticMemory{
		halflife: halflife,
		epsilon:  epsilon,
	}
}

func (memory *TimeElasticMemory) Initialized() bool {
	return memory.initialized
}

func (memory *TimeElasticMemory) Update(eventAt time.Time, currentSample float64) (float64, error) {
	if currentSample < 0 {
		return 0, fmt.Errorf("numeric: TimeElasticMemory negative sample")
	}

	if memory.halflife <= 0 {
		return 0, fmt.Errorf("numeric: TimeElasticMemory halflife must be positive")
	}

	if eventAt.IsZero() {
		return 0, fmt.Errorf("numeric: TimeElasticMemory timestamp is required")
	}

	if !memory.initialized || memory.lastAt.IsZero() {
		memory.vSlow = currentSample
		memory.lastAt = eventAt
		memory.initialized = true

		return 1.0, nil
	}

	delta := eventAt.Sub(memory.lastAt)

	if delta < 0 {
		delta = 0
	}

	memory.lastAt = eventAt

	tau := float64(memory.halflife) / math.Ln2

	var alpha float64

	if tau > 0 && delta > 0 {
		alpha = 1.0 - math.Exp(-float64(delta)/tau)
	}

	if delta > 0 && tau <= 0 {
		alpha = 1.0
	}

	relative := currentSample / (memory.vSlow + memory.epsilon)

	memory.vSlow = (1.0-alpha)*memory.vSlow + alpha*currentSample

	return relative, nil
}
